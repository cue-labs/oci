// Package ocifunc provides an implementation of ociregistry.Interface
// that uses user-provided functions for all its methods.
package ocifunc

import (
	"context"
	"fmt"
	"io"

	"github.com/rogpeppe/ociregistry"
)

// Funcs holds one function field for every corresponding method of [ociregistry.Interface].
// Use [New] to turn this into an actual implementation of that method.
type Funcs struct {
	GetBlob         func(ctx context.Context, repo string, digest ociregistry.Digest) (ociregistry.BlobReader, error)
	GetManifest     func(ctx context.Context, repo string, digest ociregistry.Digest) (ociregistry.BlobReader, error)
	GetTag          func(ctx context.Context, repo string, tagName string) (ociregistry.BlobReader, error)
	ResolveBlob     func(ctx context.Context, repo string, digest ociregistry.Digest) (ociregistry.Descriptor, error)
	ResolveManifest func(ctx context.Context, repo string, digest ociregistry.Digest) (ociregistry.Descriptor, error)
	ResolveTag      func(ctx context.Context, repo string, tagName string) (ociregistry.Descriptor, error)
	PushBlob        func(ctx context.Context, repo string, desc ociregistry.Descriptor, r io.Reader) (ociregistry.Descriptor, error)
	PushBlobChunked func(ctx context.Context, repo string, id string) (ociregistry.BlobWriter, error)
	MountBlob       func(ctx context.Context, fromRepo, toRepo string, digest ociregistry.Digest) error
	PushManifest    func(ctx context.Context, repo string, tag string, contents []byte, mediaType string) (ociregistry.Descriptor, error)
	DeleteBlob      func(ctx context.Context, repo string, digest ociregistry.Digest) error
	DeleteManifest  func(ctx context.Context, repo string, digest ociregistry.Digest) error
	DeleteTag       func(ctx context.Context, repo string, name string) error
	Repositories    func(ctx context.Context) ociregistry.Iter[string]
	Tags            func(ctx context.Context, repo string) ociregistry.Iter[string]
	Referrers       func(ctx context.Context, repo string, digest ociregistry.Digest, artifactType string) ociregistry.Iter[ociregistry.Descriptor]
}

// New returns an implementation of ociregistry.Interface that uses the functions
// in f for its methods. When a function is nil, the corresponding method will return
// an [ociregistry.ErrUnsupported] error. For nil functions that return an iterator,
// the corresponding method will return an iterator that returns no items and
// returns ErrUnsupported from its Err method.
func New(f Funcs) ociregistry.Interface {
	return funcs{f}
}

type funcs struct {
	f Funcs
}

func (f funcs) GetBlob(ctx context.Context, repo string, digest ociregistry.Digest) (ociregistry.BlobReader, error) {
	if f.GetBlob != nil {
		return f.f.GetBlob(ctx, repo, digest)
	}
	return nil, fmt.Errorf("GetBlob: %w", ociregistry.ErrUnsupported)
}

func (f funcs) GetManifest(ctx context.Context, repo string, digest ociregistry.Digest) (ociregistry.BlobReader, error) {
	if f.GetManifest != nil {
		return f.f.GetManifest(ctx, repo, digest)
	}
	return nil, fmt.Errorf("GetManifest: %w", ociregistry.ErrUnsupported)
}

func (f funcs) GetTag(ctx context.Context, repo string, tagName string) (ociregistry.BlobReader, error) {
	if f.GetTag != nil {
		return f.f.GetTag(ctx, repo, tagName)
	}
	return nil, fmt.Errorf("GetTag: %w", ociregistry.ErrUnsupported)
}

func (f funcs) ResolveBlob(ctx context.Context, repo string, digest ociregistry.Digest) (ociregistry.Descriptor, error) {
	if f.ResolveBlob != nil {
		return f.f.ResolveBlob(ctx, repo, digest)
	}
	return ociregistry.Descriptor{}, fmt.Errorf("ResolveBlob: %w", ociregistry.ErrUnsupported)
}

func (f funcs) ResolveManifest(ctx context.Context, repo string, digest ociregistry.Digest) (ociregistry.Descriptor, error) {
	if f.ResolveManifest != nil {
		return f.f.ResolveManifest(ctx, repo, digest)
	}
	return ociregistry.Descriptor{}, fmt.Errorf("ResolveManifest: %w", ociregistry.ErrUnsupported)
}

func (f funcs) ResolveTag(ctx context.Context, repo string, tagName string) (ociregistry.Descriptor, error) {
	if f.ResolveTag != nil {
		return f.f.ResolveTag(ctx, repo, tagName)
	}
	return ociregistry.Descriptor{}, fmt.Errorf("ResolveTag: %w", ociregistry.ErrUnsupported)
}

func (f funcs) PushBlob(ctx context.Context, repo string, desc ociregistry.Descriptor, r io.Reader) (ociregistry.Descriptor, error) {
	if f.PushBlob != nil {
		return f.f.PushBlob(ctx, repo, desc, r)
	}
	return ociregistry.Descriptor{}, fmt.Errorf("PushBlob: %w", ociregistry.ErrUnsupported)
}

func (f funcs) PushBlobChunked(ctx context.Context, repo string, id string) (ociregistry.BlobWriter, error) {
	if f.PushBlobChunked != nil {
		return f.f.PushBlobChunked(ctx, repo, id)
	}
	return nil, fmt.Errorf("PushBlobChunked: %w", ociregistry.ErrUnsupported)
}

func (f funcs) MountBlob(ctx context.Context, fromRepo, toRepo string, digest ociregistry.Digest) error {
	if f.MountBlob != nil {
		return f.f.MountBlob(ctx, fromRepo, toRepo, digest)
	}
	return fmt.Errorf("MountBlob: %w", ociregistry.ErrUnsupported)
}

func (f funcs) PushManifest(ctx context.Context, repo string, tag string, contents []byte, mediaType string) (ociregistry.Descriptor, error) {
	if f.PushManifest != nil {
		return f.f.PushManifest(ctx, repo, tag, contents, mediaType)
	}
	return ociregistry.Descriptor{}, fmt.Errorf("PushManifest: %w", ociregistry.ErrUnsupported)
}

func (f funcs) DeleteBlob(ctx context.Context, repo string, digest ociregistry.Digest) error {
	if f.DeleteBlob != nil {
		return f.f.DeleteBlob(ctx, repo, digest)
	}
	return fmt.Errorf("DeleteBlob: %w", ociregistry.ErrUnsupported)
}

func (f funcs) DeleteManifest(ctx context.Context, repo string, digest ociregistry.Digest) error {
	if f.DeleteManifest != nil {
		return f.f.DeleteManifest(ctx, repo, digest)
	}
	return fmt.Errorf("DeleteManifest: %w", ociregistry.ErrUnsupported)
}

func (f funcs) DeleteTag(ctx context.Context, repo string, name string) error {
	if f.DeleteTag != nil {
		return f.f.DeleteTag(ctx, repo, name)
	}
	return fmt.Errorf("DeleteTag: %w", ociregistry.ErrUnsupported)
}

func (f funcs) Repositories(ctx context.Context) ociregistry.Iter[string] {
	if f.Repositories != nil {
		return f.f.Repositories(ctx)
	}
	return ErrIter[string]{fmt.Errorf("Repositories: %w", ociregistry.ErrUnsupported)}
}

func (f funcs) Tags(ctx context.Context, repo string) ociregistry.Iter[string] {
	if f.Tags != nil {
		return f.f.Tags(ctx, repo)
	}
	return ErrIter[string]{fmt.Errorf("Tags: %w", ociregistry.ErrUnsupported)}
}

func (f funcs) Referrers(ctx context.Context, repo string, digest ociregistry.Digest, artifactType string) ociregistry.Iter[ociregistry.Descriptor] {
	if f.Referrers != nil {
		return f.f.Referrers(ctx, repo, digest, artifactType)
	}
	return ErrIter[ociregistry.Descriptor]{fmt.Errorf("Referrers: %w", ociregistry.ErrUnsupported)}
}
