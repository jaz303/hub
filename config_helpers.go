package hub

import (
	"encoding/json"
	"fmt"
	"io"

	"nhooyr.io/websocket"
)

// FirstWins creates a policy that permits a single connection per unique client ID.
// Any additional connections with the same client ID will be immediately disconnected.
func FirstWins[ID comparable, IM any]() func(conn *Conn[ID, IM], roster *Roster[ID, IM]) ([]*Conn[ID, IM], error) {
	return func(conn *Conn[ID, IM], roster *Roster[ID, IM]) ([]*Conn[ID, IM], error) {
		if len(roster.ClientConnections(conn.ClientID())) > 0 {
			return nil, fmt.Errorf("connection with a matching client ID already exists")
		}
		return nil, nil
	}
}

// LastWins creates a policy that permits a single connection per unique client ID.
// Each additional connection with the same client ID will cause its predecessor to
// be disconnected.
func LastWins[ID comparable, IM any]() func(conn *Conn[ID, IM], roster *Roster[ID, IM]) ([]*Conn[ID, IM], error) {
	return func(conn *Conn[ID, IM], roster *Roster[ID, IM]) ([]*Conn[ID, IM], error) {
		return roster.ClientConnections(conn.ClientID()), nil
	}
}

// MultipleClientConnectionsAllowed creates a policy that permits multiple simultaneous
// connections per unique client ID.
func MultipleClientConnectionsAllowed[ID comparable, IM any]() func(conn *Conn[ID, IM], roster *Roster[ID, IM]) ([]*Conn[ID, IM], error) {
	return func(conn *Conn[ID, IM], roster *Roster[ID, IM]) ([]*Conn[ID, IM], error) {
		return nil, nil
	}
}

// Binary indicates that every outgoing message is encoded as binary
func Binary(v any) (websocket.MessageType, error) { return websocket.MessageBinary, nil }

// Text indicates that every outgoing message is encoded as text
func Text(v any) (websocket.MessageType, error) { return websocket.MessageText, nil }

// WriteJSON encodes each outgoing message using a *json.Encoder
func WriteJSON(dst io.Writer, msg any) error { return json.NewEncoder(dst).Encode(msg) }
