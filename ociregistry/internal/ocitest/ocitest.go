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
		return fmt.Errorf("mismatched digest (got %d want %d)", got, want)
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
