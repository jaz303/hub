package hub

import (
	"fmt"
)

func FirstWins[ID comparable, IM any]() func(conn *Conn[ID, IM], roster *Roster[ID, IM]) ([]*Conn[ID, IM], error) {
	return func(conn *Conn[ID, IM], roster *Roster[ID, IM]) ([]*Conn[ID, IM], error) {
		if len(roster.ClientConnections(conn.ClientID())) > 0 {
			return nil, fmt.Errorf("connection with a matching client ID already exists")
		}
		return nil, nil
	}
}

func LastWins[ID comparable, IM any]() func(conn *Conn[ID, IM], roster *Roster[ID, IM]) ([]*Conn[ID, IM], error) {
	return func(conn *Conn[ID, IM], roster *Roster[ID, IM]) ([]*Conn[ID, IM], error) {
		return roster.ClientConnections(conn.ClientID()), nil
	}
}

func MultipleClientConnectionsAllowed[ID comparable, IM any]() func(conn *Conn[ID, IM], roster *Roster[ID, IM]) ([]*Conn[ID, IM], error) {
	return func(conn *Conn[ID, IM], roster *Roster[ID, IM]) ([]*Conn[ID, IM], error) {
		return nil, nil
	}
}
