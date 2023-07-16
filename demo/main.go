package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/jaz303/hub"
	"nhooyr.io/websocket"
)

// hub supports any comparable type as a client identifier.
// Here we'll use a custom string type (a simple string would also
// have been fine but this makes the code more understandable)
type username string

// Object representing the detailed client information associated
// with each connection. This can be as simple or complex as you
// like - it's not used by hub, only relayed to the outer
// program.
type user struct {
	Username username
}

// These are the messages that are exchanged over the WebSocket
// connection.
type chatMessage struct {
	SentAt  time.Time `json:"sentAt"`
	Sender  string    `json:"sender"`
	Message string    `json:"message"`
}

// Some type aliases to make code cleaner

type Config = hub.Config[username, *chatMessage]
type Roster = hub.Roster[username, *chatMessage]
type Conn = hub.Conn[username, *chatMessage]

func main() {

	// hub configuration object
	cfg := Config{
		// Maximum number of outoing messages that will be queued for each connection.
		// Currently, if the buffer is full, attempting to send another message will
		// stall the outer program.
		// Future versions of hub will support rate limiting and forced-disconnection
		// policies for slow clients.
		SendBufferSize: 32,

		// Authentication handler.
		//
		// On success, returns the client ID (username) and client info (user struct)
		// On failure, returns the WebSocket close status/reason.
		//
		// This is a dummy implementation that trusts the username supplied by the client.
		// In reality we'd probably perform some sort of password verification here.
		Authenticate: func(u uint64, c *websocket.Conn, r *http.Request) (username, any, websocket.StatusCode, string) {
			_, msgData, err := c.Read(r.Context())
			if err != nil {
				return "", nil, websocket.StatusInternalError, "auth-fail"
			}

			var authMsg = struct {
				Username username `json:"username"`
			}{}

			if err := json.Unmarshal(msgData, &authMsg); err != nil {
				return "", nil, websocket.StatusUnsupportedData, "auth-fail"
			}

			return authMsg.Username, &user{Username: authMsg.Username}, 0, ""
		},

		// Simple policy permitting a single connection per client
		// In the event of a duplicate client ID, the existing client
		// connection will be terminated.
		Accept: func(conn *Conn, roster *Roster) (bool, []*Conn) {
			return true, roster.ClientConnections(conn.ClientID())
		},

		// Incoming message decoder
		// Takes an incoming WebSocket message and returns an instance of *chatMessage.
		DecodeIncomingMessage: func(mt websocket.MessageType, b []byte) (*chatMessage, error) {
			if mt != websocket.MessageText {
				return nil, fmt.Errorf("received message of unexpected type %d", mt)
			}
			msg := chatMessage{}
			if err := json.Unmarshal(b, &msg); err != nil {
				return nil, err
			}
			return &msg, nil
		},

		// Outgoing message encoder
		// Takes an object representing an outgoing message and returns a WebSocket-
		// compatible encoding.
		EncodeOutoingMessage: func(a any) (websocket.MessageType, []byte, error) {
			jb, err := json.Marshal(a)
			if err != nil {
				return 0, nil, err
			}
			return websocket.MessageText, jb, nil
		},
	}

	// Create the hub and start it in the background
	srv := hub.New(context.Background(), &cfg)
	srv.Start()

	// Register a handler for incominbg socket connections, routed to hub's handler.
	http.HandleFunc("/socket", srv.HandleConnection)

	// Static page for test UI
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "demo/index.htm")
	})

	// Start the web server
	go http.ListenAndServe(":8080", http.DefaultServeMux)

	// Set of active connections
	activeConnections := make(map[*Conn]struct{})

	// Main loop
	for {
		select {

		// New connection
		case conn := <-srv.Connections():
			log.Printf("CONNECT: %s", conn)
			activeConnections[conn] = struct{}{}

		// Disconnection
		case conn := <-srv.Disconnections():
			log.Printf("DISCONNECT: %s", conn)
			delete(activeConnections, conn)

		// Incoming message received
		case inc := <-srv.Incoming():
			log.Printf("MSG(%s): %s", inc.Conn.ClientID(), inc.Msg.Message)
			inc.Msg.SentAt = inc.ReceivedAt
			inc.Msg.Sender = string(inc.Conn.ClientID())
			for c := range activeConnections {
				// Send message to every connection except sender.
				// In a more robust implementation we'd probably send to
				// the sender as well and include sequence numbers so the
				// everyone can see the same "true" order of messages.
				if c != inc.Conn {
					srv.SendToConnection(c, &inc.Msg)
				}
			}
		}
	}
}
