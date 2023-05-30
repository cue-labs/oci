package ociunify

import (
	"context"

	"go.cuelabs.dev/ociregistry"
)

// Deleter methods

// TODO all these methods should not raise an error if deleting succeeds in one
// registry but fails due to a not-found error in the other.

func (u unifier) DeleteBlob(ctx context.Context, repo string, digest ociregistry.Digest) error {
	return bothResults(both(
		func() t1 {
			return mk1(u.r0.DeleteBlob(ctx, repo, digest))
		},
		func() t1 {
			return mk1(u.r1.DeleteBlob(ctx, repo, digest))
		},
	)).err
}

func (u unifier) DeleteManifest(ctx context.Context, repo string, digest ociregistry.Digest) error {
	return bothResults(both(
		func() t1 {
			return mk1(u.r0.DeleteManifest(ctx, repo, digest))
		},
		func() t1 {
			return mk1(u.r1.DeleteManifest(ctx, repo, digest))
		},
	)).err
}

func (u unifier) DeleteTag(ctx context.Context, repo string, name string) error {
	return bothResults(both(
		func() t1 {
			return mk1(u.r0.DeleteTag(ctx, repo, name))
		},
		func() t1 {
			return mk1(u.r1.DeleteTag(ctx, repo, name))
		},
	)).err
}
