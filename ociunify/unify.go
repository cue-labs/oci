package ociunify

import (
	"context"
	"fmt"
	"io"

	"github.com/rogpeppe/ociregistry"
)

// New returns a registry that unifies the contents from both
// the given registries. If there's a conflict, (for example a tag resolves
// to a different thing on both repositories), it returns an error
// for requests that specifically read the value, or omits the conflicting
// item for list requests.
//
// Writes write to both repositories. Reads of immutable data
// come from either.
func New(r0, r1 ociregistry.Interface) ociregistry.Interface {
	return unifier{r0, r1, nil}
}

type unifier struct {
	r0, r1 ociregistry.Interface
	*ociregistry.Funcs
}

func bothResults[T result[T]](r0, r1 T) T {
	if r0.error() == nil && r1.error() == nil {
		return r0
	}
	var zero T
	if r0.error() != nil && r1.error() != nil {
		return zero.mkErr(fmt.Errorf("r0 and r1 failed: %w; %w", r0.error(), r1.error()))
	}
	if r0.error() != nil {
		return zero.mkErr(fmt.Errorf("r0 failed: %w", r0.error()))
	}
	return zero.mkErr(fmt.Errorf("r1 failed: %w", r1.error()))
}

type result[T any] interface {
	error() error
	close()
	mkErr(err error) T
}

// both returns the results from calling f0 and f1 concurrently.
func both[T any](f0, f1 func() T) (T, T) {
	c := make(chan T)
	go func() {
		c <- f0()
	}()
	go func() {
		c <- f1()
	}()
	return <-c, <-c
}

// race calls f0 and f1 concurrently. It returns the result from the first one that returns without error.
func race[T result[T]](ctx context.Context, f0, f1 func(ctx context.Context) T) T {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	c := make(chan T)
	sender := func(f func(ctx context.Context) T) {
		r := f(ctx)
		select {
		case c <- r:
		case <-ctx.Done():
			r.close()
		}
	}
	go sender(f0)
	go sender(f1)
	select {
	case r := <-c:
		if r.error() == nil {
			return r
		}
	case <-ctx.Done():
		return (*new(T)).mkErr(ctx.Err())
	}
	// The first result was a failure. Try for the second, which might work.
	select {
	case r := <-c:
		return r
	case <-ctx.Done():
		return (*new(T)).mkErr(ctx.Err())
	}
}

func mk1(err error) t1 {
	return t1{err}
}

type t1 struct {
	err error
}

func (t1) close() {}

func (t t1) error() error {
	return t.err
}

func (t1) mkErr(err error) t1 {
	return t1{err}
}

func mk2[T any](x T, err error) t2[T] {
	return t2[T]{x, err}
}

type t2[T any] struct {
	x   T
	err error
}

func (t t2[T]) close() {
	if closer, ok := any(t.x).(io.Closer); ok {
		closer.Close()
	}
}

func (t t2[T]) get() (T, error) {
	return t.x, t.err
}

func (t t2[T]) error() error {
	return t.err
}

func (t t2[T]) mkErr(err error) t2[T] {
	return t2[T]{*new(T), err}
}
