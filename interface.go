// Package ociregistry provides an abstraction that represents the
// capabilities provided by an OCI registry.
//
// See the [OCI distribution specification] for more information on OCI registries.
//
// Packages within this module provide the capability to translate to and
// from the HTTP protocol documented in that specification:
// - [github.com/rogpeppe/ociregistry/ociclient] provides an [Interface] value
// that acts as an HTTP client.
// - [github.com/rogpeppe/ociregistry/ociserver] provides an HTTP server
// that serves the distribution protocol by making calls to an arbitrary
// [Interface] value.
//
// When used together in a stack, the above two packages can be used
// to provide a simple proxy server.
//
// The [github.com/rogpeppe/ociregistry/ocimem] package provides a trivial
// in-memory implementation of the interface.
//
// Other packages provide some utilities that manipulate [Interface] values:
// - [github.com/rogpeppe/ociregistry/ocifilter] provides functionality for exposing
// modified or restricted views onto a registry.
// - [github.com/rogpeppe/ociregistry/ociunify] can combine two registries into one
// unified view across both.
//
// # Notes on [Interface]
//
// In general, the caller cannot assume that the implementation of a given [Interface] value
// is present on the network. For example, [github.com/rogpeppe/ociregistry/ocimem]
// doesn't know about the network at all. But there are times when an implementation
// might want to provide information about the location of blobs or manifests so
// that a client can go direct if it wishes. That is, a proxy might not wish
// to ship all the traffic for all blobs through itself, but instead redirect clients
// to talk to some other location on the internet.
//
// When an [Interface] implementation wishes to provide that information, it
// can do so by setting the `URLs` field on the descriptor that it returns for
// a given blob or manifest. Although it is not mandatory for a caller to use
// this, some callers (specifically the ociserver package) can use this information
// to redirect clients appropriately.
//
// [OCI distribution specification]: https://github.com/opencontainers/distribution-spec/blob/main/spec.md
package ociregistry

import (
	"context"
	"io"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// TODO here's an idea:
// // WithBlobs returns a registry that can use blobs for its blob storage
// // but r for everything else. It will not work if r checks that
// // pushed manifests refer to blobs within its own blob storage.
// func WithBlobs(r ociregistry.Interface, blobs ociregistry.Blobs) ociregistry.Interface

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
	private()
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
	// Errors:
	// - ErrNameUnknown when the repository is not present.
	// - ErrBlobUnknown when the blob is not present in the repository.
	GetBlob(ctx context.Context, repo string, digest Digest) (BlobReader, error)

	// TODO
	// GetBlobFrom(ctx context.Context, repo string, digest Digest, startAt int64) (BlobReader, error)

	// GetManifest returns the contents of the manifest with the given digest.
	// Errors:
	// - ErrNameUnknown when the repository is not present.
	// - ErrManifestUnknown when the blob is not present in the repository.
	GetManifest(ctx context.Context, repo string, digest Digest) (BlobReader, error)

	// GetTag returns the contents of the manifest with the given tag.
	// Errors:
	// - ErrNameUnknown when the repository is not present.
	// - ErrManifestUnknown when the tag is not present in the repository.
	GetTag(ctx context.Context, repo string, tagName string) (BlobReader, error)

	// ResolveDigest returns the descriptor for a given blob.
	// Only the MediaType, Digest and Size fields will be filled out.
	// Errors:
	// - ErrNameUnknown when the repository is not present.
	// - ErrBlobUnknown when the blob is not present in the repository.
	ResolveBlob(ctx context.Context, repo string, digest Digest) (Descriptor, error)

	// ResolveManifest returns the descriptor for a given maniifest.
	// Only the MediaType, Digest and Size fields will be filled out.
	// Errors:
	// - ErrNameUnknown when the repository is not present.
	// - ErrManifestUnknown when the blob is not present in the repository.
	ResolveManifest(ctx context.Context, repo string, digest Digest) (Descriptor, error)

	// ResolveTag returns the descriptor for a given tag.
	// Only the MediaType, Digest and Size fields will be filled out.
	// Errors:
	// - ErrNameUnknown when the repository is not present.
	// - ErrManifestUnknown when the blob is not present in the repository.
	ResolveTag(ctx context.Context, repo string, tagName string) (Descriptor, error)
}

// Writer defines registry actions that write to blobs, manifests and tags.
type Writer interface {
	// PushBlob pushes a blob described by desc to the given repository, reading content from r.
	// Only the desc.Digest and desc.Size fields are used.
	// It returns desc with Digest set to the canonical digest for the blob.
	// Errors:
	// - ErrNameUnknown when the repository is not present.
	// - ErrNameInvalid when the repository name is not valid.
	// - ErrDigestInvalid when desc.Digest does not match the content.
	// - ErrSizeInvalid when desc.Size does not match the content length.
	PushBlob(ctx context.Context, repo string, desc Descriptor, r io.Reader) (Descriptor, error)

	// PushBlobChunked starts to push a blob to the given repository.
	// The returned BlobWriter can be used to stream the upload and resume on temporary errors.
	// If id is non-zero, it should be the value returned from BlobWriter.ID
	// from a previous PushBlobChunked call and will be used to resume that blob
	// write.
	//
	// The chunkSize parameter provides a hint for the chunk size to use
	// when writing to the registry. If it's zero, a suitable default will be chosen.
	// It might be larger if the underlying registry requires that.
	//
	// The context remains active as long as the BlobWriter is around: if it's
	// cancelled, it should cause any blocked BlobWriter operations to terminate.
	PushBlobChunked(ctx context.Context, repo string, id string, chunkSize int) (BlobWriter, error)

	// MountBlob makes a blob with the given digest that's in fromRepo available
	// in toRepo and returns its canonical descriptor.
	//
	// This avoids the need to pull content down from fromRepo only to push it to r.
	//
	// TODO the mount endpoint doesn't return the size of the content,
	// so to return a correctly populated descriptor, a client will need to make
	// an extra HTTP call to find that out. For now, we'll just say that
	// the descriptor returned from MountBlob might have a zero Size.
	//
	// Errors:
	//	ErrUnsupported (when the repository does not support mounts).
	MountBlob(ctx context.Context, fromRepo, toRepo string, digest Digest) (Descriptor, error)

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
	// It is only valid before Write has been called or after Close has
	// been called.
	ID() string

	// Commit completes the blob writer process. The content is verified
	// against the provided digest, and a canonical descriptor for it is returned.
	Commit(digest Digest) (Descriptor, error)

	// Cancel ends the blob write without storing any data and frees any
	// associated resources. Any data written thus far will be lost. Cancel
	// implementations should allow multiple calls even after a commit that
	// result in a no-op. This allows use of Cancel in a defer statement,
	// increasing the assurance that it is correctly called.
	Cancel() error
}

// BlobReader provides the contents of a given blob or manifest.
type BlobReader interface {
	io.ReadCloser
	// Descriptor returns the descriptor for the blob.
	Descriptor() Descriptor
}
