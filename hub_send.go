package hub

import "context"

// SendToConnection sends a message to the given connection.
// Returns an error if the server context is cancelled before the message
// could be enqueued.
func (s *Hub[ID, IM]) SendToConnection(conn *Conn[ID, IM], msg any) error {
	return s.SendToConnectionContext(context.Background(), conn, msg)
}

// SendToConnectionContext sends a message to the client connection.
// Returns an error if either the Hub context, or the given context, is
// cancelled before the message could be enqueued.
func (s *Hub[ID, IM]) SendToConnectionContext(ctx context.Context, conn *Conn[ID, IM], msg any) error {
	return sendContext2[any](ctx, s.context, s.outgoingInt, connectionOutgoingMessage[ID, IM]{
		Connection: conn,
		Msg:        msg,
	})
}

// SendToConnections sends a message to the given connections.
// Returns an error if the server context is cancelled before the message
// could be enqueued.
func (s *Hub[ID, IM]) SendToConnections(conns []*Conn[ID, IM], msg any) error {
	return s.SendToConnectionsContext(context.Background(), conns, msg)
}

// SendToConnectionsContext sends a message to the given connections.
// Returns an error if either the Hub context, or the given context, is
// cancelled before the message could be enqueued.
func (s *Hub[ID, IM]) SendToConnectionsContext(ctx context.Context, conns []*Conn[ID, IM], msg any) error {
	return sendContext2[any](ctx, s.context, s.outgoingInt, multiConnectionOutgoingMessage[ID, IM]{
		Connections: conns,
		Msg:         msg,
	})
}

// SendToClient sends a message to a client. If there are multiple
// active connections for the given client ID, the message will be relayed
// to each of them.
// Returns an error if the server context is cancelled before the message
// could be enqueued.
func (s *Hub[ID, IM]) SendToClient(clientID ID, msg any) error {
	return s.SendToClientContext(context.Background(), clientID, msg)
}

// SendToClientContext sends a message to a client. If there are multiple
// active connections for the given client ID, the message will be relayed
// to each of them.
// Returns an error if either the Hub context, or the given context, is
// cancelled before the message could be enqueued.
func (s *Hub[ID, IM]) SendToClientContext(ctx context.Context, clientID ID, msg any) error {
	return sendContext2[any](ctx, s.context, s.outgoingInt, clientOutgoingMessage[ID]{
		ClientID: clientID,
		Msg:      msg,
	})
}

// SendToClients sends a message to a client. If there are multiple
// active connections for the given client ID, the message will be relayed
// to each of them.
// Returns an error if the server context is cancelled before the message
// could be enqueued.
func (s *Hub[ID, IM]) SendToClients(clientIDs []ID, msg any) error {
	return s.SendToClientsContext(context.Background(), clientIDs, msg)
}

// SendToClientsContext sends a message to a client. If there are multiple
// active connections for the given client ID, the message will be relayed
// to each of them.
// Returns an error if either the Hub context, or the given context, is
// cancelled before the message could be enqueued.
func (s *Hub[ID, IM]) SendToClientsContext(ctx context.Context, clientIDs []ID, msg any) error {
	return sendContext2[any](ctx, s.context, s.outgoingInt, multiClientOutgoingMessage[ID]{
		ClientIDs: clientIDs,
		Msg:       msg,
	})
}
