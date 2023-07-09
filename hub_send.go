package hub

import "context"

func (s *Hub[ID, IM]) SendToConnection(conn *Conn[ID, IM], msg any) error {
	return s.SendToConnectionContext(context.Background(), conn, msg)
}

func (s *Hub[ID, IM]) SendToConnectionContext(ctx context.Context, conn *Conn[ID, IM], msg any) error {
	return sendContext2[any](ctx, s.context, s.outgoingInt, connectionOutgoingMessage[ID, IM]{
		Connection: conn,
		Msg:        msg,
	})
}

func (s *Hub[ID, IM]) SendToConnections(conns []*Conn[ID, IM], msg any) error {
	return s.SendToConnectionsContext(context.Background(), conns, msg)
}

func (s *Hub[ID, IM]) SendToConnectionsContext(ctx context.Context, conns []*Conn[ID, IM], msg any) error {
	return sendContext2[any](ctx, s.context, s.outgoingInt, multiConnectionOutgoingMessage[ID, IM]{
		Connections: conns,
		Msg:         msg,
	})
}

func (s *Hub[ID, IM]) SendToClient(clientID ID, msg any) error {
	return s.SendToClientContext(context.Background(), clientID, msg)
}

func (s *Hub[ID, IM]) SendToClientContext(ctx context.Context, clientID ID, msg any) error {
	return sendContext2[any](ctx, s.context, s.outgoingInt, clientOutgoingMessage[ID]{
		ClientID: clientID,
		Msg:      msg,
	})
}

func (s *Hub[ID, IM]) SendToClients(clientIDs []ID, msg any) error {
	return s.SendToClientsContext(context.Background(), clientIDs, msg)
}

func (s *Hub[ID, IM]) SendToClientsContext(ctx context.Context, clientIDs []ID, msg any) error {
	return sendContext2[any](ctx, s.context, s.outgoingInt, multiClientOutgoingMessage[ID]{
		ClientIDs: clientIDs,
		Msg:       msg,
	})
}
