package ocimem

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/rogpeppe/ociregistry"

	"github.com/rogpeppe/misc/runtime/debug"
)

// This file implements the ociregistry.Writer methods.

func (r *Registry) PushBlob(ctx context.Context, repoName string, desc ociregistry.Descriptor, content io.Reader) (ociregistry.Descriptor, error) {
	data, err := io.ReadAll(content)
	if err != nil {
		return ociregistry.Descriptor{}, fmt.Errorf("cannot read content: %v", err)
	}
	if err := CheckDescriptor(desc, data); err != nil {
		return ociregistry.Descriptor{}, fmt.Errorf("invalid descriptor: %v", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	repo, err := r.makeRepo(repoName)
	if err != nil {
		return ociregistry.Descriptor{}, err
	}
	repo.blobs[desc.Digest] = &blob{mediaType: desc.MediaType, data: data}
	return desc, nil
}

func (r *Registry) PushBlobChunked(ctx context.Context, repoName string, resumeID string) (ociregistry.BlobWriter, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	repo, err := r.makeRepo(repoName)
	if err != nil {
		return nil, err
	}
	log.Printf("in PushBlobChunked; repo for %s is %p; callers %s", repoName, repo, debug.Callers(0, 20))
	if b := repo.uploads[resumeID]; b != nil {
		return b, nil
	}
	b := NewBuffer(func(b *Buffer) error {
		r.mu.Lock()
		defer r.mu.Unlock()
		desc, data, _ := b.GetBlob()
		repo.blobs[desc.Digest] = &blob{mediaType: desc.MediaType, data: data}
		log.Printf("%p commited blob; repo %s(%p); digest %s", r, repoName, repo, desc.Digest)
		return nil
	}, resumeID)
	repo.uploads[b.ID()] = b
	return b, nil
}

func (r *Registry) MountBlob(ctx context.Context, fromRepo, toRepo string, dig ociregistry.Digest) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	rto, err := r.makeRepo(toRepo)
	if err != nil {
		return err
	}
	b, err := r.blobForDigest(fromRepo, dig)
	if err != nil {
		return err
	}
	rto.blobs[dig] = b
	return nil
}

func (r *Registry) PushManifest(ctx context.Context, repoName string, tag string, data []byte, mediaType string) (ociregistry.Descriptor, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	repo, err := r.makeRepo(repoName)
	if err != nil {
		return ociregistry.Descriptor{}, err
	}
	if tag != "" && !isValidTag(tag) {
		return ociregistry.Descriptor{}, fmt.Errorf("invalid tag")
	}
	// make a copy of the data to avoid potential corruption.
	data = append([]byte(nil), data...)
	dig := digest.FromBytes(data)
	desc := ociregistry.Descriptor{
		Digest:    dig,
		MediaType: mediaType,
		Size:      int64(len(data)),
	}
	if err := CheckDescriptor(desc, data); err != nil {
		return ociregistry.Descriptor{}, fmt.Errorf("invalid descriptor: %v", err)
	}
	subject, err := r.checkManifest(repoName, desc.MediaType, data)
	if err != nil {
		return ociregistry.Descriptor{}, fmt.Errorf("invalid manifest: %v", err)
	}

	// TODO check that all the layers in the manifest point to valid blobs.
	repo.manifests[dig] = &blob{
		mediaType: mediaType,
		data:      data,
		subject:   subject,
	}
	if tag != "" {
		repo.tags[tag] = desc
	}
	return desc, nil
}

func (r *Registry) checkManifest(repoName string, mediaType string, data []byte) (ociregistry.Digest, error) {
	// TODO support other manifest types.
	if got, want := mediaType, ocispec.MediaTypeImageManifest; got != want {
		// TODO complain about non-manifest media types
		return "", nil
	}
	var m ociregistry.Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return "", err
	}
	repo, err := r.repo(repoName)
	if err != nil {
		return "", err
	}
	log.Printf("in checkManifest; repo for %q -> %p", repoName, repo)
	for i, layer := range m.Layers {
		if err := CheckDescriptor(layer, nil); err != nil {
			return "", fmt.Errorf("bad layer %d: %v", i, err)
		}
		if repo.blobs[layer.Digest] == nil {
			log.Printf("blob for layer not found in repo %p; all blobs: %v", repo, mapKeys(repo.blobs))
			return "", fmt.Errorf("%p blob for layer %d not found; repo %s; digest %s", r, i, repoName, layer.Digest)
		}
	}
	if err := CheckDescriptor(m.Config, nil); err != nil {
		return "", fmt.Errorf("bad config descriptor: %v", err)
	}
	if repo.blobs[m.Config.Digest] == nil {
		return "", fmt.Errorf("blob for config not found")
	}
	if m.Subject != nil {
		return m.Subject.Digest, nil
	}
	return "", nil
}

func mapKeys[K comparable, V any](m map[K]V) []K {
	ks := make([]K, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
