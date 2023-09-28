// Copyright 2023 CUE Labs AG
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package ociunify unifies two OCI registries into one.
package ociunify

import (
	"context"
	"fmt"
	"io"

	"cuelabs.dev/go/oci/ociregistry"
)

type Options struct {
	ReadPolicy ReadPolicy
}

type ReadPolicy int

const (
	ReadSequential ReadPolicy = iota
	ReadConcurrent
)

// New returns a registry that unifies the contents from both
// the given registries. If there's a conflict, (for example a tag resolves
// to a different thing on both repositories), it returns an error
// for requests that specifically read the value, or omits the conflicting
// item for list requests.
//
// Writes write to both repositories. Reads of immutable data
// come from either.
func New(r0, r1 ociregistry.Interface, opts *Options) ociregistry.Interface {
	if opts == nil {
		opts = new(Options)
	}
	return unifier{
		r0:   r0,
		r1:   r1,
		opts: *opts,
	}
}

type unifier struct {
	r0, r1 ociregistry.Interface
	opts   Options
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

// both returns the results from calling f on both registries concurrently.
func both[T any](u unifier, f func(r ociregistry.Interface, i int) T) (T, T) {
	c0, c1 := make(chan T), make(chan T)
	go func() {
		c0 <- f(u.r0, 0)
	}()
	go func() {
		c1 <- f(u.r1, 1)
	}()
	return <-c0, <-c1
}

// runRead calls f concurrently on each registry.
// It returns the result from the first one that returns without error.
// This should not be used if the return value is affected by cancelling the context.
func runRead[T result[T]](ctx context.Context, u unifier, f func(ctx context.Context, r ociregistry.Interface, i int) T) T {
	r, cancel := runReadWithCancel(ctx, u, f)
	cancel()
	return r
}

// runReadWithCancel calls f concurrently on each registry.
// It returns the result from the first one that returns without error
// and a cancel function that should be called when the returned value is done with.
func runReadWithCancel[T result[T]](ctx context.Context, u unifier, f func(ctx context.Context, r ociregistry.Interface, i int) T) (T, func()) {
	switch u.opts.ReadPolicy {
	case ReadConcurrent:
		return runReadConcurrent(ctx, u, f)
	case ReadSequential:
		return runReadSequential(ctx, u, f), func() {}
	default:
		panic("unreachable")
	}
}

func runReadSequential[T result[T]](ctx context.Context, u unifier, f func(ctx context.Context, r ociregistry.Interface, i int) T) T {
	r := f(ctx, u.r0, 0)
	if err := r.error(); err == nil {
		return r
	}
	return f(ctx, u.r1, 1)
}

func runReadConcurrent[T result[T]](ctx context.Context, u unifier, f func(ctx context.Context, r ociregistry.Interface, i int) T) (T, func()) {
	done := make(chan struct{})
	defer close(done)
	type result struct {
		r      T
		cancel func()
	}
	c := make(chan result)
	sender := func(f func(context.Context, ociregistry.Interface, int) T, reg ociregistry.Interface, i int) {
		ctx, cancel := context.WithCancel(ctx)
		r := f(ctx, reg, i)
		select {
		case c <- result{r, cancel}:
		case <-done:
			r.close()
			cancel()
		}
	}
	go sender(f, u.r0, 0)
	go sender(f, u.r1, 1)
	select {
	case r := <-c:
		if r.r.error() == nil {
			return r.r, r.cancel
		}
		r.cancel()
	case <-ctx.Done():
		return (*new(T)).mkErr(ctx.Err()), func() {}
	}
	// The first result was a failure. Try for the second, which might work.
	select {
	case r := <-c:
		return r.r, r.cancel
	case <-ctx.Done():
		return (*new(T)).mkErr(ctx.Err()), func() {}
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
