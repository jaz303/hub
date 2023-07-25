package hub

func FirstWins[ID comparable, IM any]() func(conn *Conn[ID, IM], roster *Roster[ID, IM]) (bool, []*Conn[ID, IM]) {
	return func(conn *Conn[ID, IM], roster *Roster[ID, IM]) (bool, []*Conn[ID, IM]) {
		existing := roster.ClientConnections(conn.ClientID())
		return len(existing) == 0, nil
	}
}

func LastWins[ID comparable, IM any]() func(conn *Conn[ID, IM], roster *Roster[ID, IM]) (bool, []*Conn[ID, IM]) {
	return func(conn *Conn[ID, IM], roster *Roster[ID, IM]) (bool, []*Conn[ID, IM]) {
		return true, roster.ClientConnections(conn.ClientID())
	}
}

func MultipleClientConnectionsAllowed[ID comparable, IM any]() func(conn *Conn[ID, IM], roster *Roster[ID, IM]) (bool, []*Conn[ID, IM]) {
	return func(conn *Conn[ID, IM], roster *Roster[ID, IM]) (bool, []*Conn[ID, IM]) {
		return true, nil
	}
}
