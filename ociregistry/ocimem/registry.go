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

// Package ocimem provides a simple in-memory implementation of
// an OCI registry.
package ocimem

import (
	"fmt"
	"sync"

	"cuelabs.dev/go/oci/ociregistry"
	"github.com/opencontainers/go-digest"
)

var _ ociregistry.Interface = (*Registry)(nil)

type Registry struct {
	*ociregistry.Funcs
	cfg   Config
	mu    sync.Mutex
	repos map[string]*repository
}

type repository struct {
	tags      map[string]ociregistry.Descriptor
	manifests map[ociregistry.Digest]*blob
	blobs     map[ociregistry.Digest]*blob
	uploads   map[string]*Buffer
}

type blob struct {
	mediaType string
	data      []byte
	subject   digest.Digest
}

func (b *blob) descriptor() ociregistry.Descriptor {
	return ociregistry.Descriptor{
		MediaType: b.mediaType,
		Size:      int64(len(b.data)),
		Digest:    digest.FromBytes(b.data),
	}
}

// TODO (breaking API change) rename NewWithConfig to New
// so we don't have two very similar entry points.

// New is like NewWithConfig(nil).
func New() *Registry {
	return NewWithConfig(nil)
}

// NewWithConfig returns a new in-memory [ociregistry.Interface]
// implementation using the given configuration. If
// cfg is nil, it's treated the same as a pointer to the zero [Config] value.
func NewWithConfig(cfg0 *Config) *Registry {
	var cfg Config
	if cfg0 != nil {
		cfg = *cfg0
	}
	return &Registry{
		cfg: cfg,
	}
}

// Config holds configuration for the registry.
type Config struct {
	// ImmutableTags specifies that tags in the registry cannot
	// be changed. Specifically the following restrictions are enforced:
	// - no removal of tags from a manifest
	// - no pushing of a tag if that tag already exists with a different
	// digest or media type.
	// - no deletion of directly tagged manifests
	// - no deletion of any blob or manifest that a tagged manifest
	// refers to (TODO: not implemented yet)
	ImmutableTags bool
}

func (r *Registry) repo(repoName string) (*repository, error) {
	if repo, ok := r.repos[repoName]; ok {
		return repo, nil
	}
	return nil, ociregistry.ErrNameUnknown
}

func (r *Registry) manifestForDigest(repoName string, dig ociregistry.Digest) (*blob, error) {
	repo, err := r.repo(repoName)
	if err != nil {
		return nil, err
	}
	b := repo.manifests[dig]
	if b == nil {
		return nil, ociregistry.ErrManifestUnknown
	}
	return b, nil
}

func (r *Registry) blobForDigest(repoName string, dig ociregistry.Digest) (*blob, error) {
	repo, err := r.repo(repoName)
	if err != nil {
		return nil, err
	}
	b := repo.blobs[dig]
	if b == nil {
		return nil, ociregistry.ErrBlobUnknown
	}
	return b, nil
}

func (r *Registry) makeRepo(repoName string) (*repository, error) {
	if !ociregistry.IsValidRepoName(repoName) {
		return nil, ociregistry.ErrNameInvalid
	}
	if r.repos == nil {
		r.repos = make(map[string]*repository)
	}
	if repo := r.repos[repoName]; repo != nil {
		return repo, nil
	}
	repo := &repository{
		tags:      make(map[string]ociregistry.Descriptor),
		manifests: make(map[digest.Digest]*blob),
		blobs:     make(map[digest.Digest]*blob),
		uploads:   make(map[string]*Buffer),
	}
	r.repos[repoName] = repo
	return repo, nil
}

// SHA256("")
const emptyHash = "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// CheckDescriptor checks that the given descriptor matches the given data or,
// if data is nil, that the descriptor looks sane.
func CheckDescriptor(desc ociregistry.Descriptor, data []byte) error {
	if err := desc.Digest.Validate(); err != nil {
		return fmt.Errorf("invalid digest: %v", err)
	}
	if data != nil {
		if digest.FromBytes(data) != desc.Digest {
			return fmt.Errorf("digest mismatch")
		}
		if desc.Size != int64(len(data)) {
			return fmt.Errorf("size mismatch")
		}
	} else {
		if desc.Size == 0 && desc.Digest != emptyHash {
			return fmt.Errorf("zero sized content with mismatching digest")
		}
	}
	if desc.MediaType == "" {
		return fmt.Errorf("no media type in descriptor")
	}
	return nil
}
