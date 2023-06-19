package ociunify

import (
	"context"

	"cuelabs.dev/go/oci/ociregistry"
)

// Deleter methods

// TODO all these methods should not raise an error if deleting succeeds in one
// registry but fails due to a not-found error in the other.

func (u unifier) DeleteBlob(ctx context.Context, repo string, digest ociregistry.Digest) error {
	return bothResults(both(u, func(r ociregistry.Interface, _ int) t1 {
		return mk1(r.DeleteBlob(ctx, repo, digest))
	})).err
}

func (u unifier) DeleteManifest(ctx context.Context, repo string, digest ociregistry.Digest) error {
	return bothResults(both(u, func(r ociregistry.Interface, _ int) t1 {
		return mk1(r.DeleteManifest(ctx, repo, digest))
	})).err
}

func (u unifier) DeleteTag(ctx context.Context, repo string, name string) error {
	return bothResults(both(u, func(r ociregistry.Interface, _ int) t1 {
		return mk1(r.DeleteTag(ctx, repo, name))
	})).err
}
