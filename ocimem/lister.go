package ocimem

import (
	"context"
	"sort"

	"github.com/rogpeppe/ociregistry"
)

func (r *Registry) Repositories(ctx context.Context) ociregistry.Iter[string] {
	r.mu.Lock()
	defer r.mu.Unlock()
	return mapKeysIter(r.repos, stringLess)
}

func (r *Registry) Tags(ctx context.Context, repoName string) ociregistry.Iter[string] {
	r.mu.Lock()
	defer r.mu.Unlock()
	repo, err := r.repo(repoName)
	if err != nil {
		return errIter[string]{err}
	}
	return mapKeysIter(repo.tags, stringLess)
}

func (r *Registry) Referrers(ctx context.Context, repoName string, digest ociregistry.Digest, artifactType string) ociregistry.Iter[ociregistry.Descriptor] {
	r.mu.Lock()
	defer r.mu.Unlock()
	repo, err := r.repo(repoName)
	if err != nil {
		return errIter[ociregistry.Descriptor]{err}
	}
	var referrers []ociregistry.Descriptor
	for _, b := range repo.blobs {
		if b.subject != digest {
			continue
		}
		// TODO filter by artifact type
		referrers = append(referrers, b.descriptor())
	}
	sort.Slice(referrers, func(i, j int) bool {
		return descriptorLess(referrers[i], referrers[j])
	})
	return ociregistry.SliceIter(referrers)
}

type errIter[T any] struct {
	err error
}

func (it errIter[T]) Close() {}

func (it errIter[T]) Next() (T, bool) {
	return *new(T), false
}

func (it errIter[T]) Error() error {
	return it.err
}

func mapKeysIter[K comparable, V any](m map[K]V, less func(K, K) bool) ociregistry.Iter[K] {
	ks := make([]K, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Slice(ks, func(i, j int) bool {
		return less(ks[i], ks[j])
	})
	return ociregistry.SliceIter(ks)
}

func stringLess(s1, s2 string) bool {
	return s1 < s2
}

func descriptorLess(d1, d2 ociregistry.Descriptor) bool {
		return d1.Digest < d2.Digest
	}