// Package ocimem provides a simple in-memory implementation of
// an OCI registry.
package ocimem

import (
	"fmt"
	"regexp"
	"sync"

	"cuelabs.dev/go/oci/ociregistry"
	"github.com/opencontainers/go-digest"
)

var _ ociregistry.Interface = (*Registry)(nil)

type Registry struct {
	*ociregistry.Funcs
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

func New() *Registry {
	return &Registry{}
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
	if !isValidRepoName(repoName) {
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

var (
	tagPattern      = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9._-]{0,127}$`)
	repoNamePattern = regexp.MustCompile(`^[a-z0-9]+([._-][a-z0-9]+)*(/[a-z0-9]+([._-][a-z0-9]+)*)*$`)
)

func isValidRepoName(repoName string) bool {
	return repoNamePattern.MatchString(repoName)
}

func isValidTag(tag string) bool {
	return tagPattern.MatchString(tag)
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
