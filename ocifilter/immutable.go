// Package ocifilter implements "filter" functions that wrap or combine ociregistry
// implementations in different ways.
package ocifilter

import (
	"context"
	"fmt"

	"github.com/opencontainers/go-digest"
	"go.cuelabs.dev/ociregistry"
)

// Immutable returns a registry wrap r but only allows content to be
// added but not changed once added: nothing can be deleted and tags
// can't be changed.
func Immutable(r ociregistry.Interface) ociregistry.Interface {
	return immutable{r}
}

type immutable struct {
	ociregistry.Interface
}

func (r immutable) PushManifest(ctx context.Context, repo string, tag string, contents []byte, mediaType string) (ociregistry.Descriptor, error) {
	if tag == "" {
		return r.Interface.PushManifest(ctx, repo, tag, contents, mediaType)
	}
	dig := digest.FromBytes(contents)

	if desc, err := r.ResolveTag(ctx, repo, tag); err == nil {
		if desc.Digest == dig {
			// We're trying to push exactly the same content. That's OK.
			return desc, nil
		}
		return ociregistry.Descriptor{}, fmt.Errorf("this store is immutable: %w", ociregistry.ErrDenied)
	}
	desc, err := r.Interface.PushManifest(ctx, repo, tag, contents, mediaType)
	if err != nil {
		return ociregistry.Descriptor{}, err
	}
	// We've pushed the tag but someone else might also have pushed it at the same time.
	// UNFORTUNATELY if there was a race, then there's a small window in time where
	// some client might have seen the tag change underfoot.
	desc, err = r.ResolveTag(ctx, repo, tag)
	if err != nil {
		return ociregistry.Descriptor{}, fmt.Errorf("cannot resolve tag that's just been pushed: %v", err)
	}
	if desc.Digest != dig {
		// We lost the race.
		return ociregistry.Descriptor{}, fmt.Errorf("this store is immutable: %w", ociregistry.ErrDenied)
	}
	return desc, nil
}

func (r immutable) DeleteBlob(ctx context.Context, repo string, digest ociregistry.Digest) error {
	return ociregistry.ErrDenied
}

func (r immutable) DeleteManifest(ctx context.Context, repo string, digest ociregistry.Digest) error {
	return ociregistry.ErrDenied
}

func (r immutable) DeleteTag(ctx context.Context, repo string, name string) error {
	return ociregistry.ErrDenied
}
