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

func (r *Registry) PushBlobChunked(ctx context.Context, repoName string, chunkSize int) (ociregistry.BlobWriter, error) {
	// TODO(mvdan): Why does the ocimem implementation allow a PATCH on an upload ID which doesn't exist?
	// The tests in ociserver make this assumption, so they break without this bit of code.
	//
	// Ideally they should start a new chunked upload to get a new ID, then use that for PATCH/PUT.
	// Alternatively, add a new method to ocimem outside of the interface to start a chunked upload with a predefined ID.
	// Either way, this case should be an error, per the spec.
	return r.PushBlobChunkedResume(ctx, repoName, "", 0, chunkSize)
}

func (r *Registry) PushBlobChunkedResume(ctx context.Context, repoName, id string, offset int64, chunkSize int) (ociregistry.BlobWriter, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	repo, err := r.makeRepo(repoName)
	if err != nil {
		return nil, err
	}
	b := repo.uploads[id]
	if b == nil {
		b = NewBuffer(func(b *Buffer) error {
			r.mu.Lock()
			defer r.mu.Unlock()
			desc, data, _ := b.GetBlob()
			repo.blobs[desc.Digest] = &blob{mediaType: desc.MediaType, data: data}
			return nil
		}, id)
		repo.uploads[b.ID()] = b
	}
	b.checkStartOffset = offset
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
	ocispec.MediaTypeImageManifest: manifestChecker(imageDescIter),
	ocispec.MediaTypeImageIndex:    manifestChecker(indexDescIter),
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

type descInfo struct {
	name  string
	desc  ocispec.Descriptor
	check digestCheck
}

// manifestChecker returns a function that can be used to check manifests
// that are JSON-unmarshaled into type T. The descIter function is
// used to iterate over the descriptors inside T: it will be called with a yield
// function that is called back to provide each descriptor, a name of the descriptor
// and what check is appropriate for applying to that descriptor.
func manifestChecker[T any](descIter func(T) func(yield func(descInfo) bool)) func(repo *repository, data []byte) (digest.Digest, error) {
	return func(repo *repository, data []byte) (_ digest.Digest, retErr error) {
		var x T
		if err := json.Unmarshal(data, &x); err != nil {
			return "", fmt.Errorf("cannot unmarshal into %T: %v", &x, err)
		}
		iter := descIter(x)
		var subject digest.Digest
		iter(func(info descInfo) bool {
			if info.name == "subject" {
				subject = info.desc.Digest
			}
			if err := CheckDescriptor(info.desc, nil); err != nil {
				retErr = fmt.Errorf("bad descriptor in %s: %v", info.name, err)
				return false
			}
			switch info.check {
			case blobMustExist:
				if repo.blobs[info.desc.Digest] == nil {
					retErr = fmt.Errorf("blob for %s not found", info.name)
					return false
				}
			case manifestMustExist:
				if repo.manifests[info.desc.Digest] == nil {
					retErr = fmt.Errorf("manifest for %s not found", info.name)
					return false
				}
			}
			return true
		})
		return subject, retErr
	}
}

func imageDescIter(m ociregistry.Manifest) func(yield func(descInfo) bool) {
	return func(yield func(descInfo) bool) {
		for i, layer := range m.Layers {
			if !yield(descInfo{
				name:  fmt.Sprintf("layers[%d]", i),
				desc:  layer,
				check: blobMustExist,
			}) {
				return
			}
		}
		if !yield(descInfo{
			name:  "config",
			desc:  m.Config,
			check: blobMustExist,
		}) {
			return
		}
		if m.Subject != nil {
			if !yield(descInfo{
				name:  "subject",
				desc:  *m.Subject,
				check: noCheck,
			}) {
				return
			}
		}
	}
}

func indexDescIter(m ocispec.Index) func(yield func(descInfo) bool) {
	return func(yield func(descInfo) bool) {
		for i, manifest := range m.Manifests {
			if !yield(descInfo{
				name:  fmt.Sprintf("manifests[%d]", i),
				desc:  manifest,
				check: manifestMustExist,
			}) {
				return
			}
		}
		if m.Subject != nil {
			if !yield(descInfo{
				name:  "subject",
				desc:  *m.Subject,
				check: noCheck,
			}) {
				return
			}
		}
	}
}
