package ociunify

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/rogpeppe/ociregistry"
)

func (u unifier) Repositories(ctx context.Context) ociregistry.Iter[string] {
	r0, r1 := both(
		func() ociregistry.Iter[string] {
			return u.r0.Repositories(ctx)
		},
		func() ociregistry.Iter[string] {
			return u.r1.Repositories(ctx)
		},
	)
	return mergeIter(r0, r1, strings.Compare)
}

func (u unifier) Tags(ctx context.Context, repo string) ociregistry.Iter[string] {
	r0, r1 := both(
		func() ociregistry.Iter[string] {
			return u.r0.Tags(ctx, repo)
		},
		func() ociregistry.Iter[string] {
			return u.r1.Tags(ctx, repo)
		},
	)
	return mergeIter(r0, r1, strings.Compare)
}

func (u unifier) Referrers(ctx context.Context, repo string, digest ociregistry.Digest, artifactType string) ociregistry.Iter[ociregistry.Descriptor] {
	r0, r1 := both(
		func() ociregistry.Iter[ociregistry.Descriptor] {
			return u.r0.Referrers(ctx, repo, digest, artifactType)
		},
		func() ociregistry.Iter[ociregistry.Descriptor] {
			return u.r1.Referrers(ctx, repo, digest, artifactType)
		},
	)
	return mergeIter(r0, r1, compareDescriptor)
}

func compareDescriptor(d0, d1 ociregistry.Descriptor) int {
	return strings.Compare(string(d0.Digest), string(d1.Digest))
}

func mergeIter[T any](it0, it1 ociregistry.Iter[T], cmp func(T, T) int) ociregistry.Iter[T] {
	// TODO streaming merge sort
	xs0, err0 := ociregistry.All(it0)
	xs1, err1 := ociregistry.All(it1)
	if err0 != nil || err1 != nil {
		notFound0 := errors.Is(err0, ociregistry.ErrNameUnknown)
		notFound1 := errors.Is(err1, ociregistry.ErrNameUnknown)
		if notFound0 && notFound1 {
			return ociregistry.ErrorIter[T](err0)
		}
		if notFound0 {
			err0 = nil
		}
		if notFound1 {
			err1 = nil
		}
	}
	var xs []T
	if len(xs0)+len(xs1) > 0 {
		xs = make([]T, len(xs0)+len(xs1))
		copy(xs, xs0)
		copy(xs[len(xs0):], xs1)
		sort.Slice(xs, func(i, j int) bool {
			return cmp(xs[i], xs[j]) < 0
		})
		j := 0
		for i := 1; i < len(xs); i++ {
			if cmp(xs[i], xs[j]) != 0 {
				j++
				xs[j] = xs[i]
			}
		}
		xs = xs[:j+1]
	}
	it := ociregistry.SliceIter(xs)
	err := err0
	if err == nil {
		err = err1
	}
	if err == nil {
		return it
	}
	return errIter[T]{
		Iter: it,
		err:  err,
	}
}

type errIter[T any] struct {
	ociregistry.Iter[T]
	err error
}

func (it errIter[T]) Error() error {
	return it.err
}
