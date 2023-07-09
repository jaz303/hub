package hub

import "context"

func (s *Hub[ID, CLI, IM]) SendToConnection(conn *Conn[ID, CLI, IM], msg any) error {
	return s.SendToConnectionContext(context.Background(), conn, msg)
}

func (s *Hub[ID, CLI, IM]) SendToConnectionContext(ctx context.Context, conn *Conn[ID, CLI, IM], msg any) error {
	select {
	case s.outgoingInt <- connectionOutgoingMessage[ID, CLI, IM]{
		Connection: conn,
		Msg:        msg,
	}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-s.context.Done():
		return s.context.Err()
	}
}

func (s *Hub[ID, CLI, IM]) SendToConnections(conns []*Conn[ID, CLI, IM], msg any) error {
	return s.SendToConnectionsContext(context.Background(), conns, msg)
}

func (s *Hub[ID, CLI, IM]) SendToConnectionsContext(ctx context.Context, conns []*Conn[ID, CLI, IM], msg any) error {
	select {
	case s.outgoingInt <- multiConnectionOutgoingMessage[ID, CLI, IM]{
		Connections: conns,
		Msg:         msg,
	}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-s.context.Done():
		return s.context.Err()
	}
}

func (s *Hub[ID, CLI, IM]) SendToClient(clientID ID, msg any) error {
	return s.SendToClientContext(context.Background(), clientID, msg)
}

func (s *Hub[ID, CLI, IM]) SendToClientContext(ctx context.Context, clientID ID, msg any) error {
	select {
	case s.outgoingInt <- clientOutgoingMessage[ID]{
		ClientID: clientID,
		Msg:      msg,
	}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-s.context.Done():
		return s.context.Err()
	}
}

func (s *Hub[ID, CLI, IM]) SendToClients(clientIDs []ID, msg any) error {
	return s.SendToClientsContext(context.Background(), clientIDs, msg)
}

func (s *Hub[ID, CLI, IM]) SendToClientsContext(ctx context.Context, clientIDs []ID, msg any) error {
	select {
	case s.outgoingInt <- multiClientOutgoingMessage[ID]{
		ClientIDs: clientIDs,
		Msg:       msg,
	}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-s.context.Done():
		return s.context.Err()
	}
}
