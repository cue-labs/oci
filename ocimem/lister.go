package ocimem

import (
	"context"
	"fmt"

	"github.com/rogpeppe/ociregistry"
)

func (r *Registry) Repositories(ctx context.Context, startAt string) ociregistry.Iter[string] {
	return errIter[string]{fmt.Errorf("Repositories TODO")}
}

func (r *Registry) Tags(ctx context.Context, repo string, startAt string) ociregistry.Iter[string] {
	return errIter[string]{fmt.Errorf("Tags TODO")}
}

func (r *Registry) Referrers(ctx context.Context, repo string, digest ociregistry.Digest, artifactType string, startAt string) ociregistry.Iter[ociregistry.Descriptor] {
	return errIter[ociregistry.Descriptor]{fmt.Errorf("Referrers TODO")}
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
