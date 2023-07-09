package hub

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"nhooyr.io/websocket"
)

func NullLogger(msg string, args ...any) {}

type Config[ID comparable, IM any] struct {
	AcceptOptions *websocket.AcceptOptions

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
	Authenticate func(uint64, *websocket.Conn, *http.Request) (ID, any, websocket.StatusCode, string)

	// Callback to determine if a new, authenticated connection should be
	// accepted for registration. Use this function to implement connection
	// policies for multiple instances of a single client ID, based on
	// inspection of the provided Roster.
	//
	// First return value indicates whether conn should be registered with
	// the server. Second return value is a list of pre-existing connections
	// that should be cancelled.
	//
	// This function is called from the server's main loop so should complete
	// quickly. The provided Roster instance must not be stored elsewhere as it
	// is not threadsafe.
	Accept func(conn *Conn[ID, IM], roster *Roster[ID, IM]) (bool, []*Conn[ID, IM])

	// Decode an incoming message into an instance of IM
	DecodeIncomingMessage func(websocket.MessageType, []byte) (IM, error)

	// Encode an outgoing message
	EncodeOutoingMessage func(any) (websocket.MessageType, []byte, error)

	// Logger - defaults to log.Printf(). Use wss.NullLogger to silence output.
	Logger func(string, ...any)
}

type Conn[ID comparable, IM any] struct {
	valid        bool
	context      context.Context
	cancel       context.CancelFunc
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

type IncomingMessage[ID comparable, IM any] struct {
	ReceivedAt time.Time
	Conn       *Conn[ID, IM]
	Msg        IM
}
