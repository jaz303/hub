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
	getCloseStatus func(cause int, err error) CloseStatus
	sendBufferSize int
	pingInterval   time.Duration
	authenticate   func(context.Context, *websocket.Conn, *http.Request) (ID, any, CloseStatus)
	accept         func(conn *Conn[ID, IM], roster *Roster[ID, IM]) ([]*Conn[ID, IM], error)
	decodeMessage  func(websocket.MessageType, []byte) (IM, error)
	encodeMessage  func(any) (websocket.MessageType, []byte, error)
	printf         func(string, ...any)

	nextConnectionID  uint64
	registrations     chan registration[ID, IM]
	closedConnections chan *Conn[ID, IM]
	incomingInt       chan IncomingMessage[ID, IM]
	outgoingInt       chan any

	connections    chan *Conn[ID, IM]
	disconnections chan *Conn[ID, IM]
	incoming       chan IncomingMessage[ID, IM]

	roster *Roster[ID, IM]
}

type registration[ID comparable, IM any] struct {
	Conn   *Conn[ID, IM]
	Accept chan error
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

// New creates a new Hub with the given config, and bound to the given context.
func New[ID comparable, IM any](ctx context.Context, cfg *Config[ID, IM]) *Hub[ID, IM] {
	acceptOptions := cfg.AcceptOptions
	if acceptOptions == nil {
		acceptOptions = defaultAcceptOptions
	}

	getCloseStatus := cfg.GetCloseStatus
	if cfg.GetCloseStatus == nil {
		getCloseStatus = DefaultCloseStatus
	}

	sbs := cfg.SendBufferSize
	if sbs <= 0 {
		sbs = defaultSendBufferSize
	}

	pi := cfg.PingInterval
	if pi < 0 {
		pi = 0
	}

	var logger = cfg.Logger
	if logger == nil {
		logger = log.Printf
	}

	srv := &Hub[ID, IM]{
		context:        ctx,
		acceptOptions:  acceptOptions,
		getCloseStatus: getCloseStatus,
		sendBufferSize: sbs,
		pingInterval:   pi,
		authenticate:   cfg.Authenticate,
		accept:         cfg.Accept,
		decodeMessage:  cfg.DecodeIncomingMessage,
		encodeMessage:  cfg.EncodeOutoingMessage,
		printf:         logger,

		nextConnectionID:  0,
		registrations:     make(chan registration[ID, IM]),
		closedConnections: make(chan *Conn[ID, IM]),
		incomingInt:       make(chan IncomingMessage[ID, IM]),
		outgoingInt:       make(chan any),

		connections:    make(chan *Conn[ID, IM]),
		disconnections: make(chan *Conn[ID, IM]),
		incoming:       make(chan IncomingMessage[ID, IM]),

		roster: NewRoster[ID, IM](),
	}
	return srv
}

// Start starts the Hub in the background; the Hub will continue running
// until the context that was passed to New is cancelled.
//
// Once started, programs should read continuously from the Connections(),
// Disconnections(), and Incoming() channels in order to prevent stalling.
// Additionally, Start should be called before starting the HTTP server that
// delegates connections to the Hub.
func (s *Hub[ID, IM]) Start() { go s.run() }

// Connections returns a channel that will receive each new Hub connection
func (s *Hub[ID, IM]) Connections() <-chan *Conn[ID, IM] { return s.connections }

// Connections returns a channel that will receive each Hub disconnection
func (s *Hub[ID, IM]) Disconnections() <-chan *Conn[ID, IM] { return s.disconnections }

// Incoming returns a channel that will receive messages received from the Hub's connections
func (s *Hub[ID, IM]) Incoming() <-chan IncomingMessage[ID, IM] { return s.incoming }

// HandleConnection is an http.HandlerFunc; integrations should route their
// WebSocket endpoint to this method.
func (s *Hub[ID, IM]) HandleConnection(w http.ResponseWriter, r *http.Request) {
	ws, err := websocket.Accept(w, r, s.acceptOptions)
	if err != nil {
		return
	}

	clientID, client, closeInfo := s.authenticate(s.context, ws, r)
	if s.context.Err() != nil {
		cs := s.getCloseStatus(HubShuttingDown, nil)
		ws.Close(cs.StatusCode, cs.Reason)
	} else if closeInfo.StatusCode > 0 {
		ws.Close(closeInfo.StatusCode, closeInfo.Reason)
		return
	}

	conn := s.makeConnection(r, clientID, client, ws)

	conn.wg.Add(1)
	go func() {
		s.writePump(conn)
		conn.wg.Done()
		conn.cancel()
	}()

	defer func() {
		conn.cancel()
		conn.wg.Wait()
		cs := conn.getCloseStatus()
		if cs.StatusCode == 0 {
			cs = s.getCloseStatus(0, nil)
		}
		conn.sock.Close(cs.StatusCode, cs.Reason)
	}()

	replyCh := make(chan error)
	if err := sendContext(s.context, s.registrations, registration[ID, IM]{
		Conn:   conn,
		Accept: replyCh,
	}); err != nil {
		s.printf("Server context cancelled while sending %s registration, cancelling")
		conn.trySetCloseStatus(s.getCloseStatus(HubShuttingDown, nil))
		return
	}

	if acceptErr, recvErr := recvContext(s.context, replyCh); recvErr != nil {
		s.printf("Server context cancelled while waiting for %s registration result, cancelling", conn)
		conn.trySetCloseStatus(s.getCloseStatus(HubShuttingDown, nil))
		return
	} else if acceptErr != nil {
		s.printf("New %s rejected by server, cancelling", conn)
		conn.trySetCloseStatus(s.getCloseStatus(AcceptPolicyDenied, acceptErr))
		return
	}

	// read messages until the connection's context is cancelled or an error occurs
	s.readPump(conn)
	sendContext(s.context, s.closedConnections, conn)
}

func (s *Hub[ID, IM]) makeConnection(r *http.Request, clientID ID, clientInfo any, sock *websocket.Conn) *Conn[ID, IM] {
	ctx, cancel := context.WithCancel(r.Context())

	return &Conn[ID, IM]{
		valid:        true,
		context:      ctx,
		cancel:       cancel,
		closeStatus:  make(chan CloseStatus, 1),
		wg:           sync.WaitGroup{},
		connectionID: atomic.AddUint64(&s.nextConnectionID, 1),
		clientID:     clientID,
		client:       clientInfo,
		sock:         sock,
		outgoing:     make(chan any, s.sendBufferSize),
	}
}

func (s *Hub[ID, IM]) writePump(conn *Conn[ID, IM]) {
	s.printf("Starting write pump for %s", conn)
	defer s.printf("Exiting write pump for %s", conn)

	var pingTimer *time.Timer
	var nextPing <-chan time.Time
	if s.pingInterval > 0 {
		pingTimer = time.NewTimer(s.pingInterval)
		nextPing = pingTimer.C
		defer func() {
			if !pingTimer.Stop() {
				<-pingTimer.C
			}
		}()
	}

	for {
		select {

		case og := <-conn.outgoing:
			if encType, encData, err := s.encodeMessage(og); err != nil {
				s.printf("Encode message failed for %s: %s", conn, err)
				conn.trySetCloseStatus(s.getCloseStatus(EncodeOutgoingMessageFailed, err))
				return
			} else if err := conn.sock.Write(conn.context, encType, encData); err != nil {
				s.printf("Write message to socket failed for %s: %s", conn, err)
				conn.trySetCloseStatus(s.getCloseStatus(WriteOutgoingMessageFailed, err))
				return
			}

		case <-nextPing:
			if err := conn.sock.Ping(conn.context); err != nil {
				s.printf("Ping failed for %s: %s", conn, err)
				conn.trySetCloseStatus(s.getCloseStatus(PingFailed, err))
				return
			}
			pingTimer.Reset(s.pingInterval)
			nextPing = pingTimer.C

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
			s.printf("Read pump for %s exited due to context cancellation", conn)
			return
		} else if err != nil {
			s.printf("Failed to read message from %s: %s", conn, err)
			conn.trySetCloseStatus(s.getCloseStatus(ReadIncomingMessageFailed, err))
			return
		} else if decoded, err := s.decodeMessage(msgType, msgData); err != nil {
			s.printf("Failed to decode message from %s: %s", conn, err)
			conn.trySetCloseStatus(s.getCloseStatus(DecodeIncomingMessageFailed, err))
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
			// Check that we can accept the connection - a connection may be denied
			// based on policy. e.g. max one connection per unique client ID. The
			// policy can also specify a list of connections that should be terminated,
			// for example if the policy says max 1 connection per user but latest wins.
			removals, err := s.accept(reg.Conn, s.roster)

			// Cancel connection for each terminated connection.
			// This will send a disconnection notification for each removed connection
			// to the application. It's important this happens before we send the
			// connection notification so that the application does not observe
			// a state inconsistent with its connection policy.
			for _, conn := range removals {
				conn.trySetCloseStatus(s.getCloseStatus(ClosedByAcceptPolicy, nil))
				s.cancelConnection(conn)
			}

			// If connection allowed, add to roster
			if err == nil {
				s.roster.Add(reg.Conn)
			}

			// Notify the connection loop of the result of the acceptance check - this
			// will cause it to start its read/write pumps
			reg.Accept <- err

			// If connection allowed, notify app of new connection
			if err == nil {
				if err := sendContext(s.context, s.connections, reg.Conn); err != nil {
					return
				}
			}

		case conn := <-s.closedConnections:
			s.cancelConnection(conn)

		case msg := <-s.incomingInt:
			if err := sendContext(s.context, s.incoming, msg); err != nil {
				return
			}

		case og := <-s.outgoingInt:
			s.printf("PICKED UP AN OUTGOING MESSAGE IN THE HUB")
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

	s.printf("SENDING MESSAGE TO CONNECTION")

	select {
	case conn.outgoing <- msg:
		s.printf("SENT OK")
	case <-s.context.Done():
		s.printf("SEND FAILED DUE TO CONTEXT CANCELLATION")
	}
}

func (s *Hub[ID, IM]) cancelAllConnections() {
	for _, conn := range s.roster.connections {
		s.cancelConnection(conn)
	}
}

func (s *Hub[ID, IM]) cancelConnection(conn *Conn[ID, IM]) {
	if !conn.valid {
		return
	}

	conn.valid = false
	conn.cancel()
	s.roster.Remove(conn)

	if err := sendContext(s.context, s.disconnections, conn); err != nil {
		return
	}
}

func (s *Hub[ID, IM]) isConnectionValid(conn *Conn[ID, IM]) bool {
	return conn.valid
}
