package ocimem

import (
	"context"

	"github.com/rogpeppe/ociregistry"
)

// This file implements the ociregistry.Reader methods.

func (r *Registry) GetBlob(ctx context.Context, repoName string, dig ociregistry.Digest) (ociregistry.BlobReader, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, err := r.blobForDigest(repoName, dig)
	if err != nil {
		return nil, err
	}
	return NewBytesReader(b.data, b.descriptor()), nil
}

func (r *Registry) GetManifest(ctx context.Context, repoName string, dig ociregistry.Digest) (ociregistry.BlobReader, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, err := r.manifestForDigest(repoName, dig)
	if err != nil {
		return nil, err
	}
	return NewBytesReader(b.data, b.descriptor()), nil
}

func (r *Registry) GetTag(ctx context.Context, repoName string, tagName string) (ociregistry.BlobReader, error) {
	desc, err := r.ResolveTag(ctx, repoName, tagName)
	if err != nil {
		return nil, err
	}
	return r.GetManifest(ctx, repoName, desc.Digest)
}

func (r *Registry) ResolveTag(ctx context.Context, repoName string, tagName string) (ociregistry.Descriptor, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	repo, err := r.repo(repoName)
	if err != nil {
		return ociregistry.Descriptor{}, err
	}
	desc, ok := repo.tags[tagName]
	if !ok {
		return ociregistry.Descriptor{}, ociregistry.ErrManifestUnknown
	}
	return desc, nil
}

func (r *Registry) ResolveBlob(ctx context.Context, repoName string, digest ociregistry.Digest) (ociregistry.Descriptor, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, err := r.blobForDigest(repoName, digest)
	if err != nil {
		return ociregistry.Descriptor{}, err
	}
	return b.descriptor(), nil
}

func (r *Registry) ResolveManifest(ctx context.Context, repoName string, digest ociregistry.Digest) (ociregistry.Descriptor, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, err := r.manifestForDigest(repoName, digest)
	if err != nil {
		return ociregistry.Descriptor{}, err
	}
	return b.descriptor(), nil
}
