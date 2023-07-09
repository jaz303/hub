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

type username string

type user struct {
	Username username
}

type Config = hub.Config[username, *chatMessage]
type Roster = hub.Roster[username, *chatMessage]
type Conn = hub.Conn[username, *chatMessage]

type chatMessage struct {
	SentAt  time.Time `json:"sentAt"`
	Sender  string    `json:"sender"`
	Message string    `json:"message"`
}

func main() {
	cfg := Config{
		SendBufferSize: 32,

		// Dummy authentication handler that trusts the username
		// supplied by the client. In reality we'd probably want
		// to perform some sort of password checking here.
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

		EncodeOutoingMessage: func(a any) (websocket.MessageType, []byte, error) {
			jb, err := json.Marshal(a)
			if err != nil {
				return 0, nil, err
			}
			return websocket.MessageText, jb, nil
		},
	}

	srv := hub.New(context.Background(), &cfg)
	srv.Start()

	http.HandleFunc("/socket", srv.HandleConnection)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "demo/index.htm")
	})

	go http.ListenAndServe(":8080", http.DefaultServeMux)

	conns := make(map[*Conn]struct{})

	for {
		select {
		case conn := <-srv.Connections():
			log.Printf("CONNECT: %s", conn)
			conns[conn] = struct{}{}
		case conn := <-srv.Disconnections():
			log.Printf("DISCONNECT: %s", conn)
			delete(conns, conn)
		case inc := <-srv.Incoming():
			log.Printf("MSG(%s): %s", inc.Conn.ClientID(), inc.Msg.Message)
			inc.Msg.SentAt = inc.ReceivedAt
			inc.Msg.Sender = string(inc.Conn.ClientID())
			for c := range conns {
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
