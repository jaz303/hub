package hub

import (
	"context"
	"errors"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"nhooyr.io/websocket"
)

var defaultAcceptOptions = &websocket.AcceptOptions{}

const (
	defaultSendBufferSize = 16
)

type Hub[ID comparable, IM any] struct {
	context        context.Context
	acceptOptions  *websocket.AcceptOptions
	sendBufferSize int
	authenticate   func(uint64, *websocket.Conn, *http.Request) (ID, any, websocket.StatusCode, string)
	accept         func(conn *Conn[ID, IM], roster *Roster[ID, IM]) (bool, []*Conn[ID, IM])
	decodeMessage  func(websocket.MessageType, []byte) (IM, error)
	encodeMessage  func(any) (websocket.MessageType, []byte, error)
	printf         func(string, ...any)

	nextConnectionID uint64
	registrations    chan registration[ID, IM]
	unregistrations  chan *Conn[ID, IM]
	incomingInt      chan IncomingMessage[ID, IM]
	outgoingInt      chan any

	connections    chan *Conn[ID, IM]
	disconnections chan *Conn[ID, IM]
	incoming       chan IncomingMessage[ID, IM]

	roster *Roster[ID, IM]
}

type registration[ID comparable, IM any] struct {
	Conn   *Conn[ID, IM]
	Accept chan bool
}

type connectionOutgoingMessage[ID comparable, IM any] struct {
	Connection *Conn[ID, IM]
	Msg        any
}

type multiConnectionOutgoingMessage[ID comparable, IM any] struct {
	Connections []*Conn[ID, IM]
	Msg         any
}

type clientOutgoingMessage[ID comparable] struct {
	ClientID ID
	Msg      any
}

type multiClientOutgoingMessage[ID comparable] struct {
	ClientIDs []ID
	Msg       any
}

func New[ID comparable, IM any](ctx context.Context, cfg *Config[ID, IM]) *Hub[ID, IM] {
	acceptOptions := cfg.AcceptOptions
	if acceptOptions == nil {
		acceptOptions = defaultAcceptOptions
	}

	sbs := cfg.SendBufferSize
	if sbs <= 0 {
		sbs = defaultSendBufferSize
	}

	var logger = cfg.Logger
	if logger == nil {
		logger = log.Printf
	}

	srv := &Hub[ID, IM]{
		context:        ctx,
		acceptOptions:  acceptOptions,
		sendBufferSize: sbs,
		authenticate:   cfg.Authenticate,
		accept:         cfg.Accept,
		decodeMessage:  cfg.DecodeIncomingMessage,
		encodeMessage:  cfg.EncodeOutoingMessage,
		printf:         logger,

		nextConnectionID: 0,
		registrations:    make(chan registration[ID, IM]),
		unregistrations:  make(chan *Conn[ID, IM]),
		incomingInt:      make(chan IncomingMessage[ID, IM]),
		outgoingInt:      make(chan any),

		connections:    make(chan *Conn[ID, IM]),
		disconnections: make(chan *Conn[ID, IM]),
		incoming:       make(chan IncomingMessage[ID, IM]),

		roster: NewRoster[ID, IM](),
	}
	return srv
}

func (s *Hub[ID, IM]) Start()                                   { go s.run() }
func (s *Hub[ID, IM]) Connections() <-chan *Conn[ID, IM]        { return s.connections }
func (s *Hub[ID, IM]) Disconnections() <-chan *Conn[ID, IM]     { return s.disconnections }
func (s *Hub[ID, IM]) Incoming() <-chan IncomingMessage[ID, IM] { return s.incoming }

func (s *Hub[ID, IM]) HandleConnection(w http.ResponseWriter, r *http.Request) {
	ws, err := websocket.Accept(w, r, s.acceptOptions)
	if err != nil {
		return
	}

	cid := atomic.AddUint64(&s.nextConnectionID, 1)

	clientID, client, closeStatus, closeReason := s.authenticate(cid, ws, r)
	if closeStatus > 0 {
		ws.Close(closeStatus, closeReason)
		return
	}

	var wg sync.WaitGroup
	defer wg.Wait()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	conn := &Conn[ID, IM]{
		valid:        true,
		context:      ctx,
		cancel:       cancel,
		connectionID: cid,
		clientID:     clientID,
		client:       client,
		sock:         ws,
		outgoing:     make(chan any, s.sendBufferSize),
	}

	// Start the write pump
	wg.Add(1)
	go func() {
		s.writePump(conn)
		wg.Done()
		cancel()
	}()

	// Inform the main loop of the new connection
	replyCh := make(chan bool, 1)

	if err := sendContext(s.context, s.registrations, registration[ID, IM]{
		Conn:   conn,
		Accept: replyCh,
	}); err != nil {
		s.printf("Server context cancelled while sending %s registration, cancelling")
		return
	}

	if res, err := recvContext(s.context, replyCh); err != nil {
		s.printf("Server context cancelled while waiting for %s registration result, cancelling", conn)
		return
	} else if !res {
		s.printf("New %s rejected by server, cancelling", conn)
		return
	}

	// Read messages until the connection's context is cancelled
	s.readPump(conn)

	sendContext(s.context, s.unregistrations, conn)
}

func (s *Hub[ID, IM]) writePump(conn *Conn[ID, IM]) {
	s.printf("Starting write pump for %s", conn)
	defer s.printf("Exiting write pump for %s", conn)

	for {
		select {

		case og := <-conn.outgoing:
			if encType, encData, err := s.encodeMessage(og); err != nil {
				s.printf("Encode message failed for %s: %s", conn, err)
				return
			} else if err := conn.sock.Write(conn.context, encType, encData); err != nil {
				s.printf("Write message to socket failed for %s: %s", conn, err)
				return
			}

		case <-conn.context.Done():
			return

		}
	}
}

func (s *Hub[ID, IM]) readPump(conn *Conn[ID, IM]) {
	s.printf("Starting read pump for %s", conn)
	defer s.printf("Exiting read pump for %s", conn)

	for {
		msgType, msgData, err := conn.sock.Read(conn.context)
		if errors.Is(err, context.Canceled) {
			return // external cancellation
		} else if err != nil {
			s.printf("Failed to read message from %s: %s", conn, err)
			return
		} else if decoded, err := s.decodeMessage(msgType, msgData); err != nil {
			s.printf("Failed to decode message from %s: %s", conn, err)
			return
		} else {
			sendContext(s.context, s.incomingInt, IncomingMessage[ID, IM]{
				ReceivedAt: time.Now(),
				Conn:       conn,
				Msg:        decoded,
			})
		}
	}
}

func (s *Hub[ID, IM]) run() {
	defer s.cancelAllConnections()

	for {
		select {

		case reg := <-s.registrations:
			ok, removals := s.accept(reg.Conn, s.roster)
			for _, conn := range removals {
				s.cancelConnection(conn)
			}

			if ok {
				s.roster.Add(reg.Conn)
			}

			reg.Accept <- ok

			if err := sendContext(s.context, s.connections, reg.Conn); err != nil {
				return
			}

		case conn := <-s.unregistrations:
			s.roster.Remove(conn)
			if err := sendContext(s.context, s.disconnections, conn); err != nil {
				return
			}

		case msg := <-s.incomingInt:
			if err := sendContext(s.context, s.incoming, msg); err != nil {
				return
			}

		case og := <-s.outgoingInt:
			s.sendOutgoingMessage(og)

		case <-s.context.Done():
			return

		}
	}
}

func (s *Hub[ID, IM]) sendOutgoingMessage(og any) {
	switch msg := og.(type) {
	case connectionOutgoingMessage[ID, IM]:
		s.sendToConnection(msg.Connection, msg.Msg)
	case multiConnectionOutgoingMessage[ID, IM]:
		for _, conn := range msg.Connections {
			s.sendToConnection(conn, msg.Msg)
		}
	case clientOutgoingMessage[ID]:
		s.sendToClient(msg.ClientID, msg.Msg)
	case multiClientOutgoingMessage[ID]:
		for _, id := range msg.ClientIDs {
			s.sendToClient(id, msg.Msg)
		}
	}
}

func (s *Hub[ID, IM]) sendToClient(client ID, msg any) {
	for _, conn := range s.roster.clients[client] {
		s.sendToConnection(conn, msg)
	}
}

func (s *Hub[ID, IM]) sendToConnection(conn *Conn[ID, IM], msg any) {
	if !s.isConnectionValid(conn) {
		return
	}

	select {
	case conn.outgoing <- msg:
	case <-s.context.Done():
	}
}

func (s *Hub[ID, IM]) cancelAllConnections() {

}

func (s *Hub[ID, IM]) cancelConnection(conn *Conn[ID, IM]) {
	conn.cancel()
	conn.valid = false
	s.roster.Remove(conn)
}

func (s *Hub[ID, IM]) isConnectionValid(conn *Conn[ID, IM]) bool {
	return conn.valid
}