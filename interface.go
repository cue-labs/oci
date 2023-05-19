package ociregistry

import (
	"context"
	"io"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// TODO define a set of known errors:
// - not found
// - redirect

// Interface defines a generic interface to a single OCI registry.
// It does not support cross-registry operations: all methods are
// directed to the receiver only.
//
// TODO define known error types (not found, redirect, ... ?)
type Interface interface {
	Writer
	Reader
	Deleter
	Lister
}

type ReadWriter interface {
	Reader
	Writer
}

type (
	Digest     = digest.Digest
	Descriptor = ocispec.Descriptor
	Manifest   = ocispec.Manifest
)

type Reader interface {
	// GetBlob returns the content of the blob with the given digest.
	GetBlob(ctx context.Context, repo string, digest Digest) (BlobReader, error)

	// TODO
	// GetBlobFrom(ctx context.Context, repo string, digest Digest, startAt int64) (BlobReader, error)

	// GetManifest returns the contents of the manifest with the given digest.
	GetManifest(ctx context.Context, repo string, digest Digest) (BlobReader, error)

	// GetTag returns the contents of the manifest with the given tag.
	GetTag(ctx context.Context, repo string, tagName string) (BlobReader, error)

	// ResolveDigest returns the descriptor for a given blob.
	// Only the MediaType, Digest and Size fields will be filled out.
	ResolveBlob(ctx context.Context, repo string, digest Digest) (Descriptor, error)

	// ResolveManifest returns the descriptor for a given maniifest.
	// Only the MediaType, Digest and Size fields will be filled out.
	ResolveManifest(ctx context.Context, repo string, digest Digest) (Descriptor, error)

	// ResolveTag returns the descriptor for a given tag.
	// Only the MediaType, Digest and Size fields will be filled out.
	ResolveTag(ctx context.Context, repo string, tagName string) (Descriptor, error)
}

// Writer defines registry actions that write to blobs, manifests and tags.
type Writer interface {
	// PushBlob pushes a blob described by desc to the given repository, reading content from r.
	// Only the desc.Digest and desc.Size fields are used.
	// It returns desc with Digest set to the canonical digest for the blob.
	PushBlob(ctx context.Context, repo string, desc Descriptor, r io.Reader) (Descriptor, error)

	// PushBlobChunked starts to push a blob to the given repository.
	// The returned BlobWriter can be used to stream the upload and resume on temporary errors.
	// If id is non-zero, it should be the value returned from BlobWriter.ID
	// from a previous PushBlobChunked call and will be used to resume that blob
	// write.
	PushBlobChunked(ctx context.Context, repo string, id string) (BlobWriter, error)

	// MountBlob makes a blob with the given digest that's in fromRepo available
	// in toRepo.
	//
	// This avoids the need to pull content down from fromRepo only to push it to r.
	MountBlob(ctx context.Context, fromRepo, toRepo string, digest Digest) error

	// PushManifest pushes a manifest with the given media type and contents.
	// If tag is non-empty, the tag with that name will be pointed at the manifest.
	//
	// It returns a descriptor suitable for accessing the manfiest.
	PushManifest(ctx context.Context, repo string, tag string, contents []byte, mediaType string) (Descriptor, error)
}

// Deleter defines registry actions that delete objects from the registry.
type Deleter interface {
	// DeleteBlob deletes the blob with the given digest in the given repository.
	DeleteBlob(ctx context.Context, repo string, digest Digest) error

	// DeleteManifest deletes the manifest with the given digest in the given repository.
	DeleteManifest(ctx context.Context, repo string, digest Digest) error

	// DeleteTag deletes the manifest with the given tag in the given repository.
	// TODO does this delete the tag only, or the manifest too?
	DeleteTag(ctx context.Context, repo string, name string) error
}

// Lister defines registry operations that enumerate objects within the registry.
// TODO support resumption from a given point.
type Lister interface {
	// Repositories returns an iterator that can be used to iterate over all the repositories
	// in the registry.
	Repositories(ctx context.Context) Iter[string]

	// Tags returns an iterator that can be used to iterate over all the tags
	// in the given repository.
	Tags(ctx context.Context, repo string) Iter[string]

	// Referrers returns an iterator that can be used to iterate over all
	// the manifests that have the given digest as their Subject.
	// If artifactType is non-zero, the results will be restricted to
	// only manifests with that type.
	// TODO is it possible to ask for multiple artifact types?
	Referrers(ctx context.Context, repo string, digest Digest, artifactType string) Iter[Descriptor]
}

// BlobWriter provides a handle for inserting data into a blob store.
type BlobWriter interface {
	// Write writes more data to the blob. When resuming, the
	// caller must start writing data from Size bytes into the content.
	io.Writer

	// Closer closes the writer but does not abort. The blob write
	// can later be resumed.
	io.Closer

	// Size returns the number of bytes written to this blob.
	Size() int64

	// ID returns the opaque identifier for this writer. The returned value
	// can be passed to PushBlobChunked to resume the write.
	ID() string

	// Commit completes the blob writer process. The content is verified
	// against the provided digest. The returned digest may be different
	// to the original depending on the blob store, referred to as the canonical
	// descriptor.
	Commit(ctx context.Context, digest Digest) (Digest, error)

	// Cancel ends the blob write without storing any data and frees any
	// associated resources. Any data written thus far will be lost. Cancel
	// implementations should allow multiple calls even after a commit that
	// result in a no-op. This allows use of Cancel in a defer statement,
	// increasing the assurance that it is correctly called.
	Cancel(ctx context.Context) error
}

// BlobReader provides the contents of a given blob or manifest.
type BlobReader interface {
	io.ReadCloser
	// Descriptor returns the descriptor for the blob.
	Descriptor() Descriptor
}
