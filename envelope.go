package hub

const (
	envBroadcast = 1 + iota
	envSingleConnection
	envMultiConnection
	envSingleClient
	envMultiClient
)

type Envelope struct {
	targetType int
	target     any
	msg        any
}
