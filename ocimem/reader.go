package ocimem

import (
	"context"
	"fmt"

	"github.com/rogpeppe/ociregistry"
)

// This file implements the ociregistry.Reader methods.

func (r *Registry) GetBlob(ctx context.Context, repoName string, dig ociregistry.Digest) (ociregistry.BlobReader, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b := r.repo(repoName).blobs[dig]
	if b == nil {
		return nil, fmt.Errorf("no such blob")
	}
	return NewBytesReader(b.data, b.descriptor()), nil
}

func (r *Registry) GetManifest(ctx context.Context, repoName string, dig ociregistry.Digest) (ociregistry.BlobReader, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b := r.repo(repoName).manifests[dig]
	if b == nil {
		return nil, fmt.Errorf("no such manifest")
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
	desc, ok := r.repo(repoName).tags[tagName]
	if !ok {
		return ociregistry.Descriptor{}, fmt.Errorf("no such tag")
	}
	return desc, nil
}

func (r *Registry) ResolveBlob(ctx context.Context, repoName string, digest ociregistry.Digest) (ociregistry.Descriptor, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.repo(repoName).blobs[digest]
	if !ok {
		return ociregistry.Descriptor{}, fmt.Errorf("no such digest")
	}
	return b.descriptor(), nil
}

func (r *Registry) ResolveManifest(ctx context.Context, repoName string, digest ociregistry.Digest) (ociregistry.Descriptor, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.repo(repoName).manifests[digest]
	if !ok {
		return ociregistry.Descriptor{}, fmt.Errorf("no such digest")
	}
	return b.descriptor(), nil
}
