package ocimem

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sync"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/rogpeppe/ociregistry"
)

type Registry struct {
	mu    sync.Mutex
	repos map[string]*repository
}

type repository struct {
	tags      map[string]ociregistry.Descriptor
	manifests map[ociregistry.Digest]*blob
	blobs     map[ociregistry.Digest]*blob
}

type blob struct {
	mediaType string
	data      []byte
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

func (r *Registry) PushManifest(ctx context.Context, repoName string, data []byte, desc ociregistry.Descriptor) (ociregistry.Descriptor, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	repo, err := r.makeRepo(repoName)
	if err != nil {
		return ociregistry.Descriptor{}, err
	}
	if err := CheckDescriptor(desc, data); err != nil {
		return ociregistry.Descriptor{}, fmt.Errorf("invalid descriptor: %v", err)
	}
	repo.manifests[desc.Digest] = &blob{desc.MediaType, data}
	// TODO check that all the layers in the manifest point to valid blobs.
	return desc, nil
}

func (r *Registry) Tag(ctx context.Context, repoName string, desc ociregistry.Descriptor, tag string, data []byte) error {
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

func (r *Registry) ResolveTag(ctx context.Context, repoName string, tagName string) (ociregistry.Descriptor, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	desc, ok := r.repo(repoName).tags[tagName]
	if !ok {
		return ociregistry.Descriptor{}, fmt.Errorf("no such tag")
	}
	return desc, nil
}

func (r *Registry) GetTag(ctx context.Context, repoName string, tagName string) (ociregistry.BlobReader, error) {
	desc, err := r.ResolveTag(ctx, repoName, tagName)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.GetManifest(ctx, repoName, desc.Digest)
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

var noRepo = new(repository)

func (r *Registry) repo(repoName string) *repository {
	if repo, ok := r.repos[repoName]; ok {
		return repo
	}
	return noRepo
}

func (r *Registry) makeRepo(repoName string) (*repository, error) {
	if !isValidRepoName(repoName) {
		return nil, fmt.Errorf("invalid repository name %q", repoName)
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

// CheckDescriptor checks that the given descriptor matches the given data or,
// if data is nil, that the descriptor looks sane.
func CheckDescriptor(desc ociregistry.Descriptor, data []byte) error {
	if data != nil {
		if digest.FromBytes(data) != desc.Digest {
			return fmt.Errorf("digest mismatch")
		}
		if desc.Size != int64(len(data)) {
			return fmt.Errorf("size mismatch")
		}
	} else {
		if desc.Size == 0 {
			return fmt.Errorf("zero sized content")
		}
	}
	if desc.MediaType == "" {
		return fmt.Errorf("no media type in descriptor")
	}
	return nil
}
