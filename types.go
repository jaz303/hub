package hub

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"nhooyr.io/websocket"
)

// NullLogger discards its input
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

// Config defines a Hub's configuration.
type Config[ID comparable, IM any] struct {
	AcceptOptions *websocket.AcceptOptions

	// Callback that returns a status code and reason based on the given
	// cause of closure.
	GetCloseStatus func(cause int, err error) CloseStatus

	// Maximum number of outgoing messages that can be queued for
	// any connection. Currently this must be set; an unbounded
	// queue is not supported. This may change in the future.
	SendBufferSize int

	// Authentication callback; this function should use the provided
	// websocket and HTTP request to authenticate the client, returning
	// the user's credentials on success, and a non-zero close status/reason
	// on failure.
	//
	// The first argument is the server/hub context; the callback should abort
	// if this context is cancelled before completion. In this situation it
	// is not necessary to return a meaningful close status as one will be
	// selected by the hub.
	//
	// If the HTTP request's context is cancelled it is the responsibility of
	// the callback to provide a meaningful close status.
	Authenticate func(context.Context, *websocket.Conn, *http.Request) (ID, any, CloseStatus)

	// Callback to determine if a new, authenticated connection should be
	// accepted for registration. Use this function to implement connection
	// policies for multiple instances of a single client ID, based on
	// inspection of the provided Roster.
	//
	// First return value is a list of pre-existing connections
	// that should be cancelled - this can be used, for example, to terminate
	// any existing connections with a matching client ID.
	//
	// Second return value indicates whether conn should be registered with
	// the server; nil indicates that the connection is accepted, non-nil
	// that it is rejected. The default close status generator uses the
	// error's string representation as the socket close reason.
	//
	// This function is called from the server's main loop so should complete
	// quickly. The provided Roster instance is valid for this invocation only
	// and must not be retained for use elsewhere as it is not threadsafe.
	Accept func(conn *Conn[ID, IM], roster *Roster[ID, IM]) ([]*Conn[ID, IM], error)

	// Decode an incoming message into an instance of IM
	DecodeIncomingMessage func(websocket.MessageType, []byte) (IM, error)

	// Encode an outgoing message
	EncodeOutoingMessage func(any) (websocket.MessageType, []byte, error)

	// Logger - defaults to log.Printf(). Use hub.NullLogger to silence output.
	Logger func(string, ...any)
}

// DefaultCloseStatus provides a basic default mapping of cause
// to close status.
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

// CloseStatus represents a WebSocket close status code and reason
type CloseStatus struct {
	StatusCode websocket.StatusCode
	Reason     string
}

// MakeCloseStatus is a helper function for creating a CloseStatus
func MakeCloseStatus(sc websocket.StatusCode, r string) CloseStatus {
	return CloseStatus{
		StatusCode: sc,
		Reason:     r,
	}
}

// Conn represents a connection to the Hub
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

// Context() returns the context associated with this connection. The context
// is valid until the connection terminates, at which point it will be cancelled.
func (c *Conn[ID, IM]) Context() context.Context { return c.context }

// ConnectionID returns the connection's unique ID
func (c *Conn[ID, IM]) ConnectionID() uint64 { return c.connectionID }

// ClientID returns the client ID associated with this connection.
// Depending on the Hub's multiple-client policy, multiple connections
// with the same client ID may be allowed.
func (c *Conn[ID, IM]) ClientID() ID { return c.clientID }

// ClientInfo returns the client info associated with the connection,
// as returned by the Authenticate callback. This value is not used
// by Hub in any way.
func (c *Conn[ID, IM]) ClientInfo() any { return c.client }

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

// IncomingMessage represents a message received from a Hub connection.
type IncomingMessage[ID comparable, IM any] struct {
	ReceivedAt time.Time     // Time message was received
	Conn       *Conn[ID, IM] // Connection from which message was received
	Msg        IM            // Decoded message
}
