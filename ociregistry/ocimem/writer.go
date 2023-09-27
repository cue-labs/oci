package ocimem

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"cuelabs.dev/go/oci/ociregistry"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
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

func (r *Registry) PushBlobChunked(ctx context.Context, repoName string, resumeID string, chunkSize int) (ociregistry.BlobWriter, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	repo, err := r.makeRepo(repoName)
	if err != nil {
		return nil, err
	}
	if b := repo.uploads[resumeID]; b != nil {
		return b, nil
	}
	b := NewBuffer(func(b *Buffer) error {
		r.mu.Lock()
		defer r.mu.Unlock()
		desc, data, _ := b.GetBlob()
		repo.blobs[desc.Digest] = &blob{mediaType: desc.MediaType, data: data}
		return nil
	}, resumeID)
	repo.uploads[b.ID()] = b
	return b, nil
}

func (r *Registry) MountBlob(ctx context.Context, fromRepo, toRepo string, dig ociregistry.Digest) (ociregistry.Descriptor, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rto, err := r.makeRepo(toRepo)
	if err != nil {
		return ociregistry.Descriptor{}, err
	}
	b, err := r.blobForDigest(fromRepo, dig)
	if err != nil {
		return ociregistry.Descriptor{}, err
	}
	rto.blobs[dig] = b
	return b.descriptor(), nil
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

// TODO support other manifest types.
var manifestCheckers = map[string]func(repo *repository, data []byte) (digest.Digest, error){
	ocispec.MediaTypeImageManifest: checkManifest(imageDescIter),
	ocispec.MediaTypeImageIndex:    checkManifest(indexDescIter),
}

func (r *Registry) checkManifest(repoName string, mediaType string, data []byte) (ociregistry.Digest, error) {
	repo, err := r.repo(repoName)
	if err != nil {
		return "", err
	}
	checker, ok := manifestCheckers[mediaType]
	if !ok {
		// TODO complain about non-manifest media types
		return "", nil
	}
	return checker(repo, data)
}

type digestCheck int

const (
	noCheck digestCheck = iota
	blobMustExist
	manifestMustExist
)

func checkManifest[T any](descIter func(T) func(func(string, ocispec.Descriptor, digestCheck) bool)) func(repo *repository, data []byte) (digest.Digest, error) {
	return func(repo *repository, data []byte) (_ digest.Digest, retErr error) {
		var x T
		if err := json.Unmarshal(data, &x); err != nil {
			return "", fmt.Errorf("cannot unmarshal into %T: %v", &x, err)
		}
		iter := descIter(x)
		var subject digest.Digest
		iter(func(about string, desc ocispec.Descriptor, check digestCheck) bool {
			if about == "subject" {
				subject = desc.Digest
			}
			if err := CheckDescriptor(desc, nil); err != nil {
				retErr = fmt.Errorf("bad descriptor in %s: %v", about, err)
				return false
			}
			switch check {
			case blobMustExist:
				if repo.blobs[desc.Digest] == nil {
					retErr = fmt.Errorf("blob for %s not found", about)
					return false
				}
			case manifestMustExist:
				if repo.manifests[desc.Digest] == nil {
					retErr = fmt.Errorf("manifest for %s not found", about)
					return false
				}
			}
			return true
		})
		return subject, retErr
	}
}

func imageDescIter(m ociregistry.Manifest) func(func(string, ocispec.Descriptor, digestCheck) bool) {
	return func(yield func(string, ocispec.Descriptor, digestCheck) bool) {
		for i, layer := range m.Layers {
			if !yield(fmt.Sprintf("layers[%d]", i), layer, blobMustExist) {
				return
			}
		}
		if !yield("config", m.Config, blobMustExist) {
			return
		}
		if m.Subject != nil {
			if !yield("subject", *m.Subject, noCheck) {
				return
			}
		}
	}
}

func indexDescIter(m ocispec.Index) func(func(string, ocispec.Descriptor, digestCheck) bool) {
	return func(yield func(string, ocispec.Descriptor, digestCheck) bool) {
		for i, manifest := range m.Manifests {
			if !yield(fmt.Sprintf("manifests[%d]", i), manifest, manifestMustExist) {
				return
			}
		}
		if m.Subject != nil {
			if !yield("subject", *m.Subject, noCheck) {
				return
			}
		}
	}
}
