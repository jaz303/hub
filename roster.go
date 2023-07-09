package hub

type Roster[ID comparable, IM any] struct {
	clients     map[ID][]*Conn[ID, IM]
	connections map[uint64]*Conn[ID, IM]
}

func NewRoster[ID comparable, IM any]() *Roster[ID, IM] {
	return &Roster[ID, IM]{
		clients:     make(map[ID][]*Conn[ID, IM]),
		connections: make(map[uint64]*Conn[ID, IM]),
	}
}

func (r *Roster[ID, IM]) ClientConnections(id ID) []*Conn[ID, IM] {
	lst := r.clients[id]
	out := make([]*Conn[ID, IM], len(lst))
	copy(out, lst)
	return out
}

func (r *Roster[ID, IM]) Add(conn *Conn[ID, IM]) {
	clients := r.clients[conn.clientID]
	clients = append(clients, conn)
	r.clients[conn.clientID] = clients

	r.connections[conn.connectionID] = conn
}

func (r *Roster[ID, IM]) Remove(conn *Conn[ID, IM]) {
	clients := removeFirstMatch(r.clients[conn.clientID], conn)
	if len(clients) == 0 {
		delete(r.clients, conn.clientID)
	} else {
		r.clients[conn.clientID] = clients
	}

	delete(r.connections, conn.connectionID)
}
