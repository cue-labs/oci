package ocimem

import (
	"encoding/json"
	"errors"
	"fmt"

	"cuelabs.dev/go/oci/ociregistry"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

type refKind int

const (
	kindSubjectManifest refKind = iota
	kindBlob
	kindManifest
)

type descInfo struct {
	name string
	kind refKind
	desc ociregistry.Descriptor
}

type descIter func(yield func(descInfo) bool)

// TODO support other manifest types.
var manifestIterators = map[string]func(data []byte) (descIter, error){
	ocispec.MediaTypeImageManifest: descIterForType(imageDescIter),
	ocispec.MediaTypeImageIndex:    descIterForType(indexDescIter),
}

var errUnknownManifestMediaTypeForIteration = errors.New("cannot determine references in unknown media type")

// manifestReferences returns an iterator that iterates over all
// direct references inside the given manifest described byx the
// given descriptor that holds the given data.
func manifestReferences(mediaType string, data []byte) (descIter, error) {
	dataIter := manifestIterators[mediaType]
	if dataIter == nil {
		// TODO provide a configuration option to disallow unknown manifest types.
		//return nil, fmt.Errorf("media type %q: %w", mediaType, errUnknownManifestMediaTypeForIteration)
		return func(func(descInfo) bool) {}, nil
	}
	return dataIter(data)
}

// repoTagIter returns an iterator that iterates through
// all the tags in the given repository.
func repoTagIter(r *repository) descIter {
	return func(yield func(descInfo) bool) {
		for tag, desc := range r.tags {
			if !yield(descInfo{
				name: tag,
				desc: desc,
				kind: kindManifest,
			}) {
				break
			}
		}
	}
}

func descIterForType[T any](newIter func(T) descIter) func(data []byte) (descIter, error) {
	return func(data []byte) (descIter, error) {
		var x T
		if err := json.Unmarshal(data, &x); err != nil {
			return nil, fmt.Errorf("cannot unmarshal into %T: %v", &x, err)
		}
		return newIter(x), nil
	}
}

func imageDescIter(m ociregistry.Manifest) descIter {
	return func(yield func(descInfo) bool) {
		for i, layer := range m.Layers {
			if !yield(descInfo{
				name: fmt.Sprintf("layers[%d]", i),
				desc: layer,
				kind: kindBlob,
			}) {
				return
			}
		}
		if !yield(descInfo{
			name: "config",
			desc: m.Config,
			kind: kindBlob,
		}) {
			return
		}
		if m.Subject != nil {
			if !yield(descInfo{
				name: "subject",
				kind: kindSubjectManifest,
				desc: *m.Subject,
			}) {
				return
			}
		}
	}
}

func indexDescIter(m ocispec.Index) descIter {
	return func(yield func(descInfo) bool) {
		for i, manifest := range m.Manifests {
			if !yield(descInfo{
				name: fmt.Sprintf("manifests[%d]", i),
				kind: kindManifest,
				desc: manifest,
			}) {
				return
			}
		}
		if m.Subject != nil {
			if !yield(descInfo{
				name: "subject",
				kind: kindSubjectManifest,
				desc: *m.Subject,
			}) {
				return
			}
		}
	}
}
