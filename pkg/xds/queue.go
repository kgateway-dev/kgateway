package xds

import (
	"context"
)

type AsyncQueue[T any] interface {
	// Dequeue will pop the first available item off of the queue.
	// This function will block until there is an item available, or the context is canceled.
	// If the context is canceled this function will return ctx.Err().
	Dequeue(ctx context.Context) (T, error)
	// Enqueue will push an item onto the queue. If no space is available on the queue,
	// this function will return immediately and drop the item.
	Enqueue(T)
	// Next will return a channel so that this can be used in a select statement.
	Next() <-chan T
}

func NewAsyncQueue[T any]() AsyncQueue[T] {
	return &asyncQueue[T]{
		queue: make(chan T, 1),
	}
}

type asyncQueue[T any] struct {
	// The queue of items to be processed
	queue chan T
}

func (a *asyncQueue[T]) Dequeue(ctx context.Context) (T, error) {
	var t T
	select {
	case t = <-a.queue:
		return t, nil
	case <-ctx.Done():
		return t, ctx.Err()
	}
}

func (a *asyncQueue[T]) Enqueue(t T) {

	select {
	case <-a.queue: // If an item is already in the queue, drop it
	default: // If not continue
	}

	select {
	case a.queue <- t: // add an item to the queue
	default: // Should never happen as we pre-emptively drained above.
	}
}

func (a *asyncQueue[T]) Next() <-chan T {
	return a.queue
}
