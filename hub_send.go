package hub

// Broadcast enqueus a message to be sent to all connections.
// Returns true on success, false if the message could not be enqueued.
func (s *Hub[ID, IM]) Broadcast(msg any) bool {
	return s.sendQueue.Put(Envelope{
		targetType: envBroadcast,
		msg:        msg,
	})
}

// SendToConnection enqueues a message for sending to a single connection.
// Returns true on success, false if the message could not be enqueued.
func (s *Hub[ID, IM]) SendToConnection(conn *Conn[ID, IM], msg any) bool {
	return s.sendQueue.Put(Envelope{
		targetType: envSingleConnection,
		target:     conn,
		msg:        msg,
	})
}

// SendToConnections enqueues a message for sending to a multiple connections.
// Returns true on success, false if the message could not be enqueued.
func (s *Hub[ID, IM]) SendToConnections(conns []*Conn[ID, IM], msg any) bool {
	return s.sendQueue.Put(Envelope{
		targetType: envMultiConnection,
		target:     conns,
		msg:        msg,
	})
}

// SendToClient enqueues a message for sending to a single client.
// If there are multiple active connections for the given client ID,
// the message will be relayed to each of them.
// Returns true on success, false if the message could not be enqueued.
func (s *Hub[ID, IM]) SendToClient(clientID ID, msg any) bool {
	return s.sendQueue.Put(Envelope{
		targetType: envSingleClient,
		target:     clientID,
		msg:        msg,
	})
}

// SendToClients enqueues a message for sending to multiple clients.
// If there are multiple active connections for any given client ID,
// the message will be relayed to each of them.
// Returns true on success, false if the message could not be enqueued.
func (s *Hub[ID, IM]) SendToClients(clientIDs []ID, msg any) bool {
	return s.sendQueue.Put(Envelope{
		targetType: envMultiClient,
		target:     clientIDs,
		msg:        msg,
	})
}
