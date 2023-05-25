package ociregistry

import (
	"context"
	"fmt"
	"io"
)

// Funcs implements Interface by calling its member functions: there's one field
// for every corresponding method of [Interface].
//
// When a function is nil, the corresponding method will return
// an [ErrUnsupported] error. For nil functions that return an iterator,
// the corresponding method will return an iterator that returns no items and
// returns ErrUnsupported from its Err method.
//
// If Funcs is nil itself, all methods will behave as if the corresponding field was nil,
// so (*ociregistry.Funcs)(nil) is a useful placeholder to implement Interface.
//
// If you're writing your own implementation of Funcs, you'll need to embed a *Funcs
// value to get an implementation of the private method. This means that it will
// be possible to add members to Interface in the future without breaking compatibility.
type Funcs struct {
	GetBlob_         func(ctx context.Context, repo string, digest Digest) (BlobReader, error)
	GetManifest_     func(ctx context.Context, repo string, digest Digest) (BlobReader, error)
	GetTag_          func(ctx context.Context, repo string, tagName string) (BlobReader, error)
	ResolveBlob_     func(ctx context.Context, repo string, digest Digest) (Descriptor, error)
	ResolveManifest_ func(ctx context.Context, repo string, digest Digest) (Descriptor, error)
	ResolveTag_      func(ctx context.Context, repo string, tagName string) (Descriptor, error)
	PushBlob_        func(ctx context.Context, repo string, desc Descriptor, r io.Reader) (Descriptor, error)
	PushBlobChunked_ func(ctx context.Context, repo string, id string, chunkSize int) (BlobWriter, error)
	MountBlob_       func(ctx context.Context, fromRepo, toRepo string, digest Digest) error
	PushManifest_    func(ctx context.Context, repo string, tag string, contents []byte, mediaType string) (Descriptor, error)
	DeleteBlob_      func(ctx context.Context, repo string, digest Digest) error
	DeleteManifest_  func(ctx context.Context, repo string, digest Digest) error
	DeleteTag_       func(ctx context.Context, repo string, name string) error
	Repositories_    func(ctx context.Context) Iter[string]
	Tags_            func(ctx context.Context, repo string) Iter[string]
	Referrers_       func(ctx context.Context, repo string, digest Digest, artifactType string) Iter[Descriptor]
}

// This blesses Funcs as the canonical Interface implementation.
func (*Funcs) private() {}

type funcs struct {
	f Funcs
}

func (f *funcs) GetBlob(ctx context.Context, repo string, digest Digest) (BlobReader, error) {
	if f != nil && f.f.GetBlob_ != nil {
		return f.f.GetBlob_(ctx, repo, digest)
	}
	return nil, fmt.Errorf("GetBlob: %w", ErrUnsupported)
}

func (f *funcs) GetManifest(ctx context.Context, repo string, digest Digest) (BlobReader, error) {
	if f != nil && f.f.GetManifest_ != nil {
		return f.f.GetManifest_(ctx, repo, digest)
	}
	return nil, fmt.Errorf("GetManifest: %w", ErrUnsupported)
}

func (f *funcs) GetTag(ctx context.Context, repo string, tagName string) (BlobReader, error) {
	if f != nil && f.f.GetTag_ != nil {
		return f.f.GetTag_(ctx, repo, tagName)
	}
	return nil, fmt.Errorf("GetTag: %w", ErrUnsupported)
}

func (f *funcs) ResolveBlob(ctx context.Context, repo string, digest Digest) (Descriptor, error) {
	if f != nil && f.f.ResolveBlob_ != nil {
		return f.f.ResolveBlob_(ctx, repo, digest)
	}
	return Descriptor{}, fmt.Errorf("ResolveBlob: %w", ErrUnsupported)
}

func (f *funcs) ResolveManifest(ctx context.Context, repo string, digest Digest) (Descriptor, error) {
	if f != nil && f.f.ResolveManifest_ != nil {
		return f.f.ResolveManifest_(ctx, repo, digest)
	}
	return Descriptor{}, fmt.Errorf("ResolveManifest: %w", ErrUnsupported)
}

func (f *funcs) ResolveTag(ctx context.Context, repo string, tagName string) (Descriptor, error) {
	if f != nil && f.f.ResolveTag_ != nil {
		return f.f.ResolveTag_(ctx, repo, tagName)
	}
	return Descriptor{}, fmt.Errorf("ResolveTag: %w", ErrUnsupported)
}

func (f *funcs) PushBlob(ctx context.Context, repo string, desc Descriptor, r io.Reader) (Descriptor, error) {
	if f != nil && f.f.PushBlob_ != nil {
		return f.f.PushBlob_(ctx, repo, desc, r)
	}
	return Descriptor{}, fmt.Errorf("PushBlob: %w", ErrUnsupported)
}

func (f *funcs) PushBlobChunked(ctx context.Context, repo string, id string, chunkSize int) (BlobWriter, error) {
	if f != nil && f.f.PushBlobChunked_ != nil {
		return f.f.PushBlobChunked_(ctx, repo, id, chunkSize)
	}
	return nil, fmt.Errorf("PushBlobChunked: %w", ErrUnsupported)
}

func (f *funcs) MountBlob(ctx context.Context, fromRepo, toRepo string, digest Digest) error {
	if f != nil && f.f.MountBlob_ != nil {
		return f.f.MountBlob_(ctx, fromRepo, toRepo, digest)
	}
	return fmt.Errorf("MountBlob: %w", ErrUnsupported)
}

func (f *funcs) PushManifest(ctx context.Context, repo string, tag string, contents []byte, mediaType string) (Descriptor, error) {
	if f != nil && f.f.PushManifest_ != nil {
		return f.f.PushManifest_(ctx, repo, tag, contents, mediaType)
	}
	return Descriptor{}, fmt.Errorf("PushManifest: %w", ErrUnsupported)
}

func (f *funcs) DeleteBlob(ctx context.Context, repo string, digest Digest) error {
	if f != nil && f.f.DeleteBlob_ != nil {
		return f.f.DeleteBlob_(ctx, repo, digest)
	}
	return fmt.Errorf("DeleteBlob: %w", ErrUnsupported)
}

func (f *funcs) DeleteManifest(ctx context.Context, repo string, digest Digest) error {
	if f != nil && f.f.DeleteManifest_ != nil {
		return f.f.DeleteManifest_(ctx, repo, digest)
	}
	return fmt.Errorf("DeleteManifest: %w", ErrUnsupported)
}

func (f *funcs) DeleteTag(ctx context.Context, repo string, name string) error {
	if f != nil && f.f.DeleteTag_ != nil {
		return f.f.DeleteTag_(ctx, repo, name)
	}
	return fmt.Errorf("DeleteTag: %w", ErrUnsupported)
}

func (f *funcs) Repositories(ctx context.Context) Iter[string] {
	if f != nil && f.f.Repositories_ != nil {
		return f.f.Repositories_(ctx)
	}
	return ErrorIter[string](fmt.Errorf("Repositories: %w", ErrUnsupported))
}

func (f *funcs) Tags(ctx context.Context, repo string) Iter[string] {
	if f != nil && f.f.Tags_ != nil {
		return f.f.Tags_(ctx, repo)
	}
	return ErrorIter[string](fmt.Errorf("Tags: %w", ErrUnsupported))
}

func (f *funcs) Referrers(ctx context.Context, repo string, digest Digest, artifactType string) Iter[Descriptor] {
	if f != nil && f.f.Referrers_ != nil {
		return f.f.Referrers_(ctx, repo, digest, artifactType)
	}
	return ErrorIter[Descriptor](fmt.Errorf("Referrers: %w", ErrUnsupported))
}
