package hub

import (
	"context"
	"reflect"
)

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

func orDefault[T any](v T, defaultValue T) T {
	ref := reflect.ValueOf(v)
	if !ref.IsValid() || ref.IsZero() {
		return defaultValue
	}
	return v
}

func orDefaultLazy[T any](v T, defaultValue func() T) T {
	ref := reflect.ValueOf(v)
	if !ref.IsValid() || ref.IsZero() {
		return defaultValue()
	}
	return v
}
