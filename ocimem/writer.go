package ocimem

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/rogpeppe/ociregistry"
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
	repo.blobs[desc.Digest] = &blob{desc.MediaType, data}
	return desc, nil
}

func (r *Registry) PushBlobChunked(ctx context.Context, repoName string, resumeID string) (ociregistry.BlobWriter, error) {
	if resumeID != "" {
		return nil, fmt.Errorf("TODO support resuming")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	repo, err := r.makeRepo(repoName)
	if err != nil {
		return nil, err
	}
	return NewBuffer(func(b *Buffer) error {
		r.mu.Lock()
		defer r.mu.Unlock()
		desc, data, _ := b.GetBlob()
		repo.blobs[desc.Digest] = &blob{desc.MediaType, data}
		return nil
	}), nil
}

func (r *Registry) MountBlob(ctx context.Context, fromRepo, toRepo string, dig ociregistry.Digest) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	rto, err := r.makeRepo(toRepo)
	if err != nil {
		return err
	}
	b := r.repo(fromRepo).blobs[dig]
	if b == nil {
		return fmt.Errorf("no such blob")
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
	if mediaType == "" {
		return ociregistry.Descriptor{}, fmt.Errorf("empty media type")
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

	// TODO check that all the layers in the manifest point to valid blobs.
	repo.manifests[dig] = &blob{mediaType, data}
	if tag != "" {
		repo.tags[tag] = desc
	}
	return desc, nil
}

func (r *Registry) PushTag(ctx context.Context, repoName string, desc ociregistry.Descriptor, tag string, data []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !isValidTag(tag) {
		return fmt.Errorf("invalid tag")
	}
	repo := r.repo(repoName)
	var b *blob
	if data == nil {
		b = repo.manifests[desc.Digest]
		if b == nil {
			return fmt.Errorf("no manifest data present")
		}
	} else {
		b = &blob{desc.MediaType, data}
	}
	if err := CheckDescriptor(desc, b.data); err != nil {
		return fmt.Errorf("invalid descriptor: %v", err)
	}
	if err := r.checkManifest(repoName, desc.MediaType, b.data); err != nil {
		return fmt.Errorf("invalid manifest: %v", err)
	}
	if data != nil {
		repo.manifests[desc.Digest] = b
	}
	// Note: we make a new descriptor here because the actual API does
	// not upload the full descriptor when tagging.
	repo.tags[tag] = ociregistry.Descriptor{
		Digest:    desc.Digest,
		MediaType: desc.MediaType,
		Size:      desc.Size,
	}
	return nil

}

func (r *Registry) checkManifest(repoName string, mediaType string, data []byte) error {
	// TODO support other manifest types.
	if got, want := mediaType, ocispec.MediaTypeImageManifest; got != want {
		return fmt.Errorf("unexpected media type; got %q want %q", got, want)
	}
	var m ociregistry.Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	repo := r.repo(repoName)
	for i, layer := range m.Layers {
		if err := CheckDescriptor(layer, nil); err != nil {
			return fmt.Errorf("bad layer %d: %v", i, err)
		}
		if repo.blobs[layer.Digest] == nil {
			return fmt.Errorf("blob for layer %d not found", i)
		}
	}
	if err := CheckDescriptor(m.Config, nil); err != nil {
		return fmt.Errorf("bad config descriptor: %v", err)
	}
	if repo.blobs[m.Config.Digest] == nil {
		return fmt.Errorf("blob for config not found")
	}
	return nil
}
