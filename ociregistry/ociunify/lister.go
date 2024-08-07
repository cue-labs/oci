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

package ociunify

import (
	"context"
	"errors"
	"slices"
	"strings"

	"cuelabs.dev/go/oci/ociregistry"
)

func (u unifier) Repositories(ctx context.Context, startAfter string) ociregistry.Seq[string] {
	r0, r1 := both(u, func(r ociregistry.Interface, _ int) ociregistry.Seq[string] {
		return r.Repositories(ctx, startAfter)
	})
	return mergeIter(r0, r1, strings.Compare)
}

func (u unifier) Tags(ctx context.Context, repo, startAfter string) ociregistry.Seq[string] {
	r0, r1 := both(u, func(r ociregistry.Interface, _ int) ociregistry.Seq[string] {
		return r.Tags(ctx, repo, startAfter)
	})
	return mergeIter(r0, r1, strings.Compare)
}

func (u unifier) Referrers(ctx context.Context, repo string, digest ociregistry.Digest, artifactType string) ociregistry.Seq[ociregistry.Descriptor] {
	r0, r1 := both(u, func(r ociregistry.Interface, _ int) ociregistry.Seq[ociregistry.Descriptor] {
		return r.Referrers(ctx, repo, digest, artifactType)
	})
	return mergeIter(r0, r1, compareDescriptor)
}

func compareDescriptor(d0, d1 ociregistry.Descriptor) int {
	return strings.Compare(string(d0.Digest), string(d1.Digest))
}

func mergeIter[T any](it0, it1 ociregistry.Seq[T], cmp func(T, T) int) ociregistry.Seq[T] {
	// TODO streaming merge sort
	xs0, err0 := ociregistry.All(it0)
	xs1, err1 := ociregistry.All(it1)
	if err0 != nil || err1 != nil {
		notFound0 := errors.Is(err0, ociregistry.ErrNameUnknown)
		notFound1 := errors.Is(err1, ociregistry.ErrNameUnknown)
		if notFound0 && notFound1 {
			return ociregistry.ErrorSeq[T](err0)
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
		xs = slices.Concat(xs0, xs1)
		slices.SortFunc(xs, cmp)
		xs = slices.CompactFunc(xs, func(t1, t2 T) bool {
			return cmp(t1, t2) == 0
		})
	}
	err := err0
	if err == nil {
		err = err1
	}
	if err == nil {
		return ociregistry.SliceSeq(xs)
	}
	return func(yield func(T, error) bool) {
		for _, x := range xs {
			if !yield(x, nil) {
				return
			}
		}
		yield(*new(T), err)
	}
}
