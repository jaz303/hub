package hub

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"nhooyr.io/websocket"
)

func NullLogger(msg string, args ...any) {}

const (
	EncodeOutgoingMessageFailed = 1 + iota
	WriteOutgoingMessageFailed
	DecodeIncomingMessageFailed
	ReadIncomingMessageFailed
	AcceptPolicyDenied
	ClosedByAcceptPolicy
	HubShuttingDown
)

type Config[ID comparable, IM any] struct {
	AcceptOptions *websocket.AcceptOptions

	GetCloseStatus func(cause int, err error) CloseStatus

	// Maximum number of outgoing messages that can be queued for
	// any connection. Currently this must be set; an unbounded
	// queue is not supported. This may change in the future.
	SendBufferSize int

	// Authentication callback; this function should use the provided
	// websocket and HTTP request to authenticate the client, returning
	// the user's credentials on success, and a non-zero close status/reason
	// on failure.
	// The callback must abort if the request's Context is cancelled
	// before completion.
	Authenticate func(context.Context, *websocket.Conn, *http.Request) (ID, any, CloseStatus)

	// Callback to determine if a new, authenticated connection should be
	// accepted for registration. Use this function to implement connection
	// policies for multiple instances of a single client ID, based on
	// inspection of the provided Roster.
	//
	// First return value indicates whether conn should be registered with
	// the server; a status code of zero means the connection is accepted,
	// and any other value causes the connection to be closed with the given
	// status code.
	//
	// Second return value is a list of pre-existing connections
	// that should be cancelled.
	//
	// This function is called from the server's main loop so should complete
	// quickly. The provided Roster instance must not be stored elsewhere as it
	// is not threadsafe.
	Accept func(conn *Conn[ID, IM], roster *Roster[ID, IM]) ([]*Conn[ID, IM], error)

	// Decode an incoming message into an instance of IM
	DecodeIncomingMessage func(websocket.MessageType, []byte) (IM, error)

	// Encode an outgoing message
	EncodeOutoingMessage func(any) (websocket.MessageType, []byte, error)

	// Logger - defaults to log.Printf(). Use hub.NullLogger to silence output.
	Logger func(string, ...any)
}

func DefaultCloseStatus(cause int, err error) CloseStatus {
	switch cause {
	case EncodeOutgoingMessageFailed,
		WriteOutgoingMessageFailed,
		DecodeIncomingMessageFailed,
		ReadIncomingMessageFailed:
		return MakeCloseStatus(websocket.StatusInternalError, "internal error")
	case AcceptPolicyDenied:
		return MakeCloseStatus(websocket.StatusPolicyViolation, err.Error())
	case HubShuttingDown:
		return MakeCloseStatus(websocket.StatusGoingAway, "hub shutting down")
	}

	return MakeCloseStatus(websocket.StatusNormalClosure, "")
}

type CloseStatus struct {
	StatusCode websocket.StatusCode
	Reason     string
}

func MakeCloseStatus(sc websocket.StatusCode, r string) CloseStatus {
	return CloseStatus{
		StatusCode: sc,
		Reason:     r,
	}
}

type Conn[ID comparable, IM any] struct {
	// true if connection is valid
	// This field is not threadsafe - only read/update from main hub goroutine
	valid bool

	// Connection context - derived from initial incoming request context.
	// The connection will run until this context is cancelled; cancellation
	// can happen anywhere
	context context.Context
	cancel  context.CancelFunc

	//
	closeStatus chan CloseStatus

	wg sync.WaitGroup

	connectionID uint64
	clientID     ID
	client       any
	sock         *websocket.Conn
	outgoing     chan any
}

func (c *Conn[ID, IM]) String() string {
	return fmt.Sprintf("connection[id=%d, client=%v]", c.connectionID, c.clientID)
}

func (c *Conn[ID, IM]) ConnectionID() uint64 { return c.connectionID }
func (c *Conn[ID, IM]) ClientID() ID         { return c.clientID }
func (c *Conn[ID, IM]) ClientInfo() any      { return c.client }

func (c *Conn[ID, IM]) trySetCloseStatus(cs CloseStatus) bool {
	select {
	case c.closeStatus <- cs:
		return true
	default:
		return false
	}
}

func (c *Conn[ID, IM]) getCloseStatus() CloseStatus {
	select {
	case cs := <-c.closeStatus:
		return cs
	default:
		return CloseStatus{}
	}
}

type IncomingMessage[ID comparable, IM any] struct {
	ReceivedAt time.Time
	Conn       *Conn[ID, IM]
	Msg        IM
}
