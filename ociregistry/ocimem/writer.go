// Copyright 2023 CUE Labs AG
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
	for i, layer := range m.Layers {
		if err := CheckDescriptor(layer, nil); err != nil {
			return "", fmt.Errorf("bad layer %d: %v", i, err)
		}
		if repo.blobs[layer.Digest] == nil {
			return "", fmt.Errorf("blob for layer %d not found; repo %s; digest %s", i, repoName, layer.Digest)
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
