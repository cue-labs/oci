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

// NewRegistry returns a Registry instance that wraps r, providing
// convenience methods for pushing and checking content
// inside the given test instance.
//
// When a Must* method fails, it will fail using t.
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
	// TODO support manifest indexes too.
	Manifests map[string]ociregistry.Manifest

	// Blobs maps from blob identifer to the contents of the blob.
	Blobs map[string]string

	// Tags maps from tag name to manifest identifier.
	Tags map[string]string
}

// PushedRepoContent mirrors RepoContent but, instead
// of describing content that is to be pushed, describes the
// content that has been pushed.
type PushedRepoContent struct {
	// Manifests holds an entry for each manifest identifier
	// with the descriptor for that manifest.
	Manifests map[string]ociregistry.Descriptor

	// ManifestData holds the actually pushed data for each manifest.
	ManifestData map[string][]byte

	// Blobs holds an entry for each blob identifier
	// with the descriptor for that manifest.
	Blobs map[string]ociregistry.Descriptor
}

// PushContent pushes all the content in rc to r.
//
// It returns a map mapping repository name to the descriptors
// describing the content that has actually been pushed.
func PushContent(r ociregistry.Interface, rc RegistryContent) (map[string]PushedRepoContent, error) {
	regContent := make(map[string]PushedRepoContent)
	for repo, repoc := range rc {
		prc, err := PushRepoContent(r, repo, repoc)
		if err != nil {
			return nil, fmt.Errorf("cannot push content for repository %q: %v", repo, err)
		}
		regContent[repo] = prc
	}
	return regContent, nil
}

// PushRepoContent pushes the content for a single repository.
func PushRepoContent(r ociregistry.Interface, repo string, repoc RepoContent) (PushedRepoContent, error) {
	ctx := context.Background()
	prc := PushedRepoContent{
		Manifests:    make(map[string]ociregistry.Descriptor),
		ManifestData: make(map[string][]byte),
		Blobs:        make(map[string]ociregistry.Descriptor),
	}

	for id, blob := range repoc.Blobs {
		prc.Blobs[id] = ociregistry.Descriptor{
			Digest:    digest.FromString(blob),
			Size:      int64(len(blob)),
			MediaType: "application/binary",
		}
	}
	manifests, manifestSeq, err := completedManifests(repoc, prc.Blobs)
	if err != nil {
		return PushedRepoContent{}, err
	}
	for id, content := range manifests {
		prc.Manifests[id] = content.desc
		prc.ManifestData[id] = content.data
	}
	// First push all the blobs:
	for id, content := range repoc.Blobs {
		_, err := r.PushBlob(ctx, repo, prc.Blobs[id], strings.NewReader(content))
		if err != nil {
			return PushedRepoContent{}, fmt.Errorf("cannot push blob %q in repo %q: %v", id, repo, err)
		}
	}
	// Then push the manifests that refer to the blobs.
	for _, mc := range manifestSeq {
		_, err := r.PushManifest(ctx, repo, "", mc.data, mc.desc.MediaType)
		if err != nil {
			return PushedRepoContent{}, fmt.Errorf("cannot push manifest %q in repo %q: %v", mc.id, repo, err)
		}
	}
	// Then push any tags.
	for tag, id := range repoc.Tags {
		mc, ok := manifests[id]
		if !ok {
			return PushedRepoContent{}, fmt.Errorf("tag %q refers to unknown manifest id %q", tag, id)
		}
		_, err := r.PushManifest(ctx, repo, tag, mc.data, mc.desc.MediaType)
		if err != nil {
			return PushedRepoContent{}, fmt.Errorf("cannot push tag %q in repo %q: %v", id, repo, err)
		}
	}
	return prc, nil
}

// PushContent pushes all the content in rc to r.
//
// It returns a map mapping repository name to the descriptors
// describing the content that has actually been pushed.
func (r Registry) MustPushContent(rc RegistryContent) map[string]PushedRepoContent {
	prc, err := PushContent(r.R, rc)
	qt.Assert(r.T, qt.IsNil(err))
	return prc
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
	required := make(map[string]bool)
	for {
		madeProgress := false
		needMore := false
		need := func(digest ociregistry.Digest) {
			needMore = true
			if !required[string(digest)] {
				required[string(digest)] = true
				madeProgress = true
			}
		}
		for id, m := range repoc.Manifests {
			if _, ok := manifests[id]; ok {
				continue
			}
			m1 := m
			if m1.Subject != nil {
				mc, ok := manifests[string(m1.Subject.Digest)]
				if !ok {
					need(m1.Subject.Digest)
					continue
				}
				m1.Subject = ref(*m1.Subject)
				*m1.Subject = mc.desc
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
			madeProgress = true
			manifestSeq = append(manifestSeq, mc)
		}
		if !needMore {
			return manifests, manifestSeq, nil
		}
		if !madeProgress {
			for m := range required {
				if _, ok := manifests[m]; ok {
					delete(required, m)
				}
			}
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

type Repo struct {
	T    *testing.T
	Name string
	R    ociregistry.Interface
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
