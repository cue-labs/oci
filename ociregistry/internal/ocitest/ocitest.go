// Package ocitest provides some helper types for writing ociregistry-related
// tests. It's designed to be used alongside the [qt package].
//
// [qt package]: https://pkg.go.dev/github.com/go-quicktest/qt
package ocitest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"testing"

	"github.com/go-quicktest/qt"
	"github.com/opencontainers/go-digest"

	"cuelabs.dev/go/oci/ociregistry"
)

type Registry struct {
	T *testing.T
	R ociregistry.Interface
}

func NewRegistry(t *testing.T, r ociregistry.Interface) Registry {
	return Registry{t, r}
}

// RegistryContent specifies the contents of a registry: a map from
// repository name to the contents of that repository.
type RegistryContent map[string]RepoContent

// RepoContent specifies the content of a repository.
// manifests and blobs are keyed by symbolic identifiers,
// not used inside the registry itself, but instead
// placeholders for the digest of the associated content.
//
// Digest strings inside manifests that are not valid digests
// will be replaced by the calculated digest of the manifest or
// blob with that identifier; the size and media type fields will also be
// filled in.
type RepoContent struct {
	// Manifests maps from manifest identifier to the contents of the manifest.
	// TODO support manifest lists too.
	Manifests map[string]ociregistry.Manifest
	// Blobs maps from blob identifer to the contents of the blob.
	Blobs map[string]string
	// Tags maps from tag name to manifest identifier.
	Tags map[string]string
}

// PushContent pushes all the content in rc to r.
func (r Registry) MustPushContent(rc RegistryContent) {
	for repo, repoc := range rc {
		err := pushRepoContent(r.R, repo, repoc)
		qt.Assert(r.T, qt.IsNil(err))
	}
}

func pushRepoContent(r ociregistry.Interface, repo string, repoc RepoContent) error {
	ctx := context.Background()
	// blobs maps blob name to the descriptor for that blob.
	blobs := make(map[string]ociregistry.Descriptor)
	for id, blob := range repoc.Blobs {
		blobs[id] = ociregistry.Descriptor{
			Digest:    digest.FromString(blob),
			Size:      int64(len(blob)),
			MediaType: "application/binary",
		}
	}
	manifests, manifestSeq, err := completedManifests(repoc, blobs)
	if err != nil {
		return err
	}
	// First push all the blobs:
	for id, content := range repoc.Blobs {
		_, err := r.PushBlob(ctx, repo, blobs[id], strings.NewReader(content))
		if err != nil {
			return fmt.Errorf("cannot push blob %q in repo %q", id, repo)
		}
	}
	// Then push the manifests that refer to the blobs.
	for _, mc := range manifestSeq {
		_, err := r.PushManifest(ctx, repo, "", mc.data, mc.desc.MediaType)
		if err != nil {
			return fmt.Errorf("cannot push manifest %q in repo %q", mc.id, repo)
		}
	}
	// Then push any tags.
	for tag, id := range repoc.Tags {
		mc, ok := manifests[id]
		if !ok {
			return fmt.Errorf("tag %q refers to unknown manifest id %q", tag, id)
		}
		_, err := r.PushManifest(ctx, repo, tag, mc.data, mc.desc.MediaType)
		if err != nil {
			return fmt.Errorf("cannot push tag %q in repo %q", id, repo)
		}
	}
	return nil
}

type manifestContent struct {
	id   string
	data []byte
	desc ociregistry.Descriptor
}

// completedManifests calculates the content of all the manifests and returns
// them all, keyed by id, and a partially ordered sequence suitable
// for pushing to a registry in bottom-up order.
func completedManifests(repoc RepoContent, blobs map[string]ociregistry.Descriptor) (map[string]manifestContent, []manifestContent, error) {
	manifests := make(map[string]manifestContent)
	manifestSeq := make([]manifestContent, 0, len(repoc.Manifests))
	// subject relationships can be arbitrarily deep, so continue iterating until
	// all the levels are completed. If at any point we can't make progress, we
	// know there's a problem and panic.
	for {
		madeProgress := false
		required := make(map[string]bool)
		for id, m := range repoc.Manifests {
			if _, ok := manifests[id]; ok {
				continue
			}
			m1 := m
			if m.Subject != nil {
				mc, ok := manifests[string(m.Subject.Digest)]
				if !ok {
					required[string(m.Subject.Digest)] = true
					continue
				}
				m.Subject = ref(*m.Subject)
				*m.Subject = mc.desc
				madeProgress = true
			}
			m1.Config = fillBlobDescriptor(m.Config, blobs)
			m1.Layers = make([]ociregistry.Descriptor, len(m.Layers))
			for i, desc := range m.Layers {
				m1.Layers[i] = fillBlobDescriptor(desc, blobs)
			}
			data, err := json.Marshal(m1)
			if err != nil {
				panic(err)
			}
			mc := manifestContent{
				id:   id,
				data: data,
				desc: ociregistry.Descriptor{
					Digest:    digest.FromBytes(data),
					Size:      int64(len(data)),
					MediaType: m.MediaType,
				},
			}
			manifests[id] = mc
			manifestSeq = append(manifestSeq, mc)
		}
		if len(required) == 0 {
			return manifests, manifestSeq, nil
		}
		if !madeProgress {
			return nil, nil, fmt.Errorf("no manifest found for ids %s", strings.Join(mapKeys(required), ", "))
		}
	}
}

func fillManifestDescriptors(m ociregistry.Manifest, blobs map[string]ociregistry.Descriptor) ociregistry.Manifest {
	m.Config = fillBlobDescriptor(m.Config, blobs)
	m.Layers = append([]ociregistry.Descriptor(nil), m.Layers...)
	for i, desc := range m.Layers {
		m.Layers[i] = fillBlobDescriptor(desc, blobs)
	}
	return m
}

func fillBlobDescriptor(d ociregistry.Descriptor, blobs map[string]ociregistry.Descriptor) ociregistry.Descriptor {
	blobDesc, ok := blobs[string(d.Digest)]
	if !ok {
		panic(fmt.Errorf("no blob found with id %q", d.Digest))
	}
	d.Digest = blobDesc.Digest
	d.Size = blobDesc.Size
	if d.MediaType == "" {
		d.MediaType = blobDesc.MediaType
	}
	return d
}

func (r Registry) MustPushBlob(repo string, data []byte) ociregistry.Descriptor {
	desc := ociregistry.Descriptor{
		Digest:    digest.FromBytes(data),
		Size:      int64(len(data)),
		MediaType: "application/octet-stream",
	}
	desc1, err := r.R.PushBlob(context.Background(), repo, desc, bytes.NewReader(data))
	qt.Assert(r.T, qt.IsNil(err))
	return desc1
}

func (r Registry) MustPushManifest(repo string, jsonObject any, tag string) ([]byte, ociregistry.Descriptor) {
	data, err := json.Marshal(jsonObject)
	qt.Assert(r.T, qt.IsNil(err))
	var mt struct {
		MediaType string `json:"mediaType,omitempty"`
	}
	err = json.Unmarshal(data, &mt)
	qt.Assert(r.T, qt.IsNil(err))
	qt.Assert(r.T, qt.Not(qt.Equals(mt.MediaType, "")))
	desc := ociregistry.Descriptor{
		Digest:    digest.FromBytes(data),
		Size:      int64(len(data)),
		MediaType: mt.MediaType,
	}
	desc1, err := r.R.PushManifest(context.Background(), repo, tag, data, mt.MediaType)
	qt.Assert(r.T, qt.IsNil(err))
	qt.Check(r.T, qt.Equals(desc1.Digest, desc.Digest))
	qt.Check(r.T, qt.Equals(desc1.Size, desc.Size))
	qt.Check(r.T, qt.Equals(desc1.MediaType, desc.MediaType))
	return data, desc1
}

func (r Registry) HasBlob(repo string, wantData []byte) qt.Checker {
	panic("TODO")
}
func (r Registry) HasManifest(repo string, wantData []byte, wantContentType string) qt.Checker {
	panic("TODO")
}
func (r Registry) Repo(repo string) Repo {
	panic("TODO")
}

type Repo struct {
	T    *testing.T
	Name string
	R    ociregistry.Interface
}

func (r Repo) MustPushBlob(data []byte) ociregistry.Descriptor {
	panic("TODO")
}
func (r Repo) MustPushManifest(jsonObject any, tag string) ([]byte, ociregistry.Descriptor) {
	panic("TODO")
}
func (r Repo) HasBlob(wantData []byte) qt.Checker {
	panic("TODO")
}
func (r Repo) HasManifest(wantData []byte, wantContentType string) qt.Checker {
	panic("TODO")
}

// HasContent returns a checker that checks r matches the expected
// data and has the expected content type. If wantMediaType is
// empty, application/octet-stream will be expected.
func HasContent(r ociregistry.BlobReader, wantData []byte, wantMediaType string) qt.Checker {
	if wantMediaType == "" {
		wantMediaType = "application/octet-stream"
	}
	return contentChecker{
		r:             r,
		wantData:      wantData,
		wantMediaType: wantMediaType,
	}
}

type contentChecker struct {
	r             ociregistry.BlobReader
	wantData      []byte
	wantMediaType string
}

func (c contentChecker) Args() []qt.Arg {
	return []qt.Arg{{
		Name:  "reader",
		Value: c.r,
	}, {
		Name:  "data",
		Value: c.wantData,
	}, {
		Name:  "mediaType",
		Value: c.wantMediaType,
	}}
}

func (c contentChecker) Check(note func(key string, value any)) error {
	desc := c.r.Descriptor()
	gotData, err := io.ReadAll(c.r)
	if err != nil {
		return qt.BadCheckf("error reading data: %v", err)
	}
	if got, want := desc.Size, int64(len(c.wantData)); got != want {
		note("actual data", gotData)
		return fmt.Errorf("mismatched content length (got %d want %d)", got, want)
	}
	if got, want := desc.Digest, digest.FromBytes(c.wantData); got != want {
		note("actual data", gotData)
		return fmt.Errorf("mismatched digest (got %v want %v)", got, want)
	}
	if !bytes.Equal(gotData, c.wantData) {
		note("actual data", gotData)
		return fmt.Errorf("mismatched content")
	}
	if got, want := desc.MediaType, c.wantMediaType; got != want {
		note("actual media type", desc.MediaType)
		return fmt.Errorf("media type mismatch")
	}
	return nil
}

func ref[T any](x T) *T {
	return &x
}

func mapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
