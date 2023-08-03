package hub

// Roster is an index of all active connections to a Hub.
// Rosters are not threadsafe and must only be used by integrating
// programs in the callbacks to which they are provided.
type Roster[ID comparable, IM any] struct {
	clients     map[ID][]*Conn[ID, IM]
	connections map[uint64]*Conn[ID, IM]
}

// NewRoster creates a new Roster.
func NewRoster[ID comparable, IM any]() *Roster[ID, IM] {
	return &Roster[ID, IM]{
		clients:     make(map[ID][]*Conn[ID, IM]),
		connections: make(map[uint64]*Conn[ID, IM]),
	}
}

// ClientConnections returns a slice containing all connections associated
// with a given client ID
func (r *Roster[ID, IM]) ClientConnections(id ID) []*Conn[ID, IM] {
	lst := r.clients[id]
	out := make([]*Conn[ID, IM], len(lst))
	copy(out, lst)
	return out
}

// Add adds a connection to the Roster
func (r *Roster[ID, IM]) Add(conn *Conn[ID, IM]) {
	clients := r.clients[conn.clientID]
	clients = append(clients, conn)
	r.clients[conn.clientID] = clients

	r.connections[conn.connectionID] = conn
}

// Remove removes a connection from the Roster
func (r *Roster[ID, IM]) Remove(conn *Conn[ID, IM]) {
	clients := removeFirstMatch(r.clients[conn.clientID], conn)
	if len(clients) == 0 {
		delete(r.clients, conn.clientID)
	} else {
		r.clients[conn.clientID] = clients
	}

	delete(r.connections, conn.connectionID)
}
