package hub

import "context"

const (
	defaultBufferedQueueSize = 1024
)

type SendQueue interface {
	Envelopes() <-chan Envelope
	Put(env Envelope) bool
}

type RunnableSendQueue interface {
	SendQueue
	Run(ctx context.Context)
}

type BufferedChannelQueue struct {
	buffer chan Envelope
}

func NewBufferedChannelQueue(bufferSize int) *BufferedChannelQueue {
	return &BufferedChannelQueue{
		buffer: make(chan Envelope, bufferSize),
	}
}

var _ SendQueue = &BufferedChannelQueue{}

func (q *BufferedChannelQueue) Envelopes() <-chan Envelope {
	return q.buffer
}

func (q *BufferedChannelQueue) Put(env Envelope) bool {
	select {
	case q.buffer <- env:
		return true
	default:
		return false
	}
}
