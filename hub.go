package hub

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"nhooyr.io/websocket"
)

var defaultAcceptOptions = &websocket.AcceptOptions{}

const (
	defaultTag            = "hub"
	defaultSendBufferSize = 16
)

type Hub[ID comparable, IM any] struct {
	context                 context.Context
	tag                     string
	acceptOptions           *websocket.AcceptOptions
	getCloseStatus          func(cause int, err error) CloseStatus
	sendQueue               SendQueue
	perConnectionBufferSize int
	pingInterval            time.Duration
	authenticate            func(context.Context, *websocket.Conn, *http.Request) (ID, any, CloseStatus)
	accept                  func(conn *Conn[ID, IM], roster *Roster[ID, IM]) ([]*Conn[ID, IM], error)
	readMessage             func(websocket.MessageType, io.Reader) (IM, error)
	outgoingMessageType     func(any) (websocket.MessageType, error)
	writeOutgoingMessage    func(io.Writer, any) error
	logFn                   func(...any)

	nextConnectionID  uint64
	registrations     chan registration[ID, IM]
	closedConnections chan *Conn[ID, IM]
	incomingInt       chan IncomingMessage[ID, IM]

	connections    chan *Conn[ID, IM]
	disconnections chan *Conn[ID, IM]
	incoming       chan IncomingMessage[ID, IM]

	roster *Roster[ID, IM]
}

type registration[ID comparable, IM any] struct {
	Conn   *Conn[ID, IM]
	Accept chan error
}

// New creates a new Hub with the given config, and bound to the given context.
func New[ID comparable, IM any](ctx context.Context, cfg *Config[ID, IM]) *Hub[ID, IM] {
	srv := &Hub[ID, IM]{
		context:        ctx,
		tag:            fmt.Sprintf("[hub=%s]", orDefault(cfg.Tag, defaultTag)),
		acceptOptions:  orDefault(cfg.AcceptOptions, defaultAcceptOptions),
		getCloseStatus: orDefault(cfg.GetCloseStatus, DefaultCloseStatus),
		sendQueue: orDefaultLazy(cfg.SendQueue, func() SendQueue {
			return NewBufferedChannelQueue(orDefault(cfg.SendQueueSize, defaultBufferedQueueSize))
		}),
		perConnectionBufferSize: orDefault(cfg.PerConnectionSendBufferSize, defaultSendBufferSize),
		pingInterval:            orDefault(cfg.PingInterval, 0),
		authenticate:            cfg.Authenticate,
		accept:                  cfg.Accept,
		readMessage:             cfg.ReadIncomingMessage,
		outgoingMessageType:     orDefault(cfg.OutgoingMessageType, Text),
		writeOutgoingMessage:    orDefault(cfg.WriteOutgoingMessage, WriteJSON),
		logFn:                   orDefault(cfg.Logger, log.Println),

		nextConnectionID:  0,
		registrations:     make(chan registration[ID, IM]),
		closedConnections: make(chan *Conn[ID, IM]),
		incomingInt:       make(chan IncomingMessage[ID, IM]),

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
		defer func() {
			conn.cancel()
			conn.wg.Done()
		}()
		s.writePump(conn)
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
		outgoing:     make(chan any, s.perConnectionBufferSize),
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
			closeStatus, err := s.writeOneMessage(conn, og)
			if err != nil {
				conn.trySetCloseStatus(s.getCloseStatus(closeStatus, err))
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

func (s *Hub[ID, IM]) writeOneMessage(conn *Conn[ID, IM], msg any) (int, error) {
	msgType, err := s.outgoingMessageType(msg)
	if err != nil {
		s.printf("Failed to get message type for outgoing message on %s: %s", conn, err)
		return EncodeOutgoingMessageFailed, err
	}

	writer, err := conn.sock.Writer(conn.context, msgType)
	if err != nil {
		s.printf("Failed to acquire writer for outgoing message on %s: %s", conn, err)
		return WriteOutgoingMessageFailed, err
	}

	defer writer.Close()

	if err := s.writeOutgoingMessage(writer, msg); err != nil {
		s.printf("Write message to socket failed on %s: %s", conn, err)
		return WriteOutgoingMessageFailed, err
	}

	return 0, nil
}

func (s *Hub[ID, IM]) readPump(conn *Conn[ID, IM]) {
	s.printf("Starting read pump for %s", conn)
	defer s.printf("Exiting read pump for %s", conn)

	for {
		msgType, reader, err := conn.sock.Reader(conn.context)
		if errors.Is(err, context.Canceled) {
			s.printf("Read pump for %s exited due to context cancellation", conn)
			return
		} else if err != nil {
			s.printf("Failed to acquire message reader for %s: %s", conn, err)
			conn.trySetCloseStatus(s.getCloseStatus(ReadIncomingMessageFailed, err))
			return
		}

		decoded, err := s.readMessage(msgType, reader)
		skip := errors.Is(err, ErrSkipMessage)

		if err != nil && !skip {
			s.printf("Failed to decode message from %s: %s", conn, err)
			conn.trySetCloseStatus(s.getCloseStatus(DecodeIncomingMessageFailed, err))
			return
		}

		// There's no guarantee that decodeMessage() read the entire message so
		// drain the rest of the input.
		if _, err := io.Copy(io.Discard, reader); err != nil {
			s.printf("Failed to discard message tail from %s: %s", conn, err)
			conn.trySetCloseStatus(s.getCloseStatus(ReadIncomingMessageFailed, err))
			return
		}

		if !skip {
			sendContext(s.context, s.incomingInt, IncomingMessage[ID, IM]{
				ReceivedAt: time.Now(),
				Conn:       conn,
				Msg:        decoded,
			})
		}
	}
}

func (s *Hub[ID, IM]) run() {
	if runnable, ok := s.sendQueue.(RunnableSendQueue); ok {
		go runnable.Run(s.context)
	}

	defer s.cancelAllConnections()
	for s.context.Err() == nil {
		s.tick()
	}
}

func (s *Hub[ID, IM]) tick() {
	defer func() {
		if ex := recover(); ex != nil {
			s.printf("hub tick recovered from error: %+v", ex)
		}
	}()

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

	case env := <-s.sendQueue.Envelopes():
		s.dispatchEnvelope(env)

	case <-s.context.Done():
		return

	}
}

func (s *Hub[ID, IM]) dispatchEnvelope(env Envelope) {
	switch env.targetType {
	case envBroadcast:
		for _, conn := range s.roster.connections {
			s.sendToConnection(conn, env.msg)
		}
	case envSingleConnection:
		s.sendToConnection(env.target.(*Conn[ID, IM]), env.msg)
	case envMultiConnection:
		for _, conn := range env.target.([]*Conn[ID, IM]) {
			s.sendToConnection(conn, env.msg)
		}
	case envSingleClient:
		s.sendToClient(env.target.(ID), env.msg)
	case envMultiClient:
		for _, id := range env.target.([]ID) {
			s.sendToClient(id, env.msg)
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

func (s *Hub[ID, IM]) printf(msg string, args ...any) {
	s.logFn(s.tag, fmt.Sprintf(msg, args...))
}
