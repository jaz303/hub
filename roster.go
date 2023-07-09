package hub

type Roster[ID comparable, CLI any, IM any] struct {
	clients     map[ID][]*Conn[ID, CLI, IM]
	connections map[uint64]*Conn[ID, CLI, IM]
}

func NewRoster[ID comparable, CLI any, IM any]() *Roster[ID, CLI, IM] {
	return &Roster[ID, CLI, IM]{
		clients:     make(map[ID][]*Conn[ID, CLI, IM]),
		connections: make(map[uint64]*Conn[ID, CLI, IM]),
	}
}

func (r *Roster[ID, CLI, IM]) ClientConnections(id ID) []*Conn[ID, CLI, IM] {
	lst := r.clients[id]
	out := make([]*Conn[ID, CLI, IM], len(lst))
	copy(out, lst)
	return out
}

func (r *Roster[ID, CLI, IM]) Add(conn *Conn[ID, CLI, IM]) {
	clients := r.clients[conn.clientID]
	clients = append(clients, conn)
	r.clients[conn.clientID] = clients

	r.connections[conn.connectionID] = conn
}

func (r *Roster[ID, CLI, IM]) Remove(conn *Conn[ID, CLI, IM]) {
	clients := removeFirstMatch(r.clients[conn.clientID], conn)
	if len(clients) == 0 {
		delete(r.clients, conn.clientID)
	} else {
		r.clients[conn.clientID] = clients
	}

	delete(r.connections, conn.connectionID)
}
