package hub

import "context"

func recvContext[E any](ctx context.Context, src <-chan E) (E, error) {
	var zero E
	select {
	case <-ctx.Done():
		return zero, ctx.Err()
	case v := <-src:
		return v, nil
	}
}

func sendContext[E any](ctx context.Context, dst chan<- E, val E) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case dst <- val:
		return nil
	}
}

func sendContext2[E any](ctx1 context.Context, ctx2 context.Context, dst chan<- E, val E) error {
	select {
	case <-ctx1.Done():
		return ctx1.Err()
	case <-ctx2.Done():
		return ctx2.Err()
	case dst <- val:
		return nil
	}
}

func removeFirstMatch[E comparable](slice []E, val E) []E {
	last := len(slice) - 1
	for i := range slice {
		if slice[i] == val {
			slice[i], slice[last] = slice[last], slice[i]
			return slice[0:last]
		}
	}
	return slice
}
