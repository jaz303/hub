package hub

const (
	envBroadcast = 1 + iota
	envSingleConnection
	envMultiConnection
	envSingleClient
	envMultiClient
)

// Envelope represents an outgoing message along with its destination(s)
type Envelope struct {
	targetType int
	target     any
	msg        any
}
