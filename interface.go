package ociregistry

import (
	"context"
	"io"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// Interface defines a generic interface to a single OCI registry.
// It does not support cross-registry operations: all methods are
// directed to the receiver only.
type Interface interface {
	Writer
	Reader
	Deleter
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
	GetBlob(ctx context.Context, repo string, digest Digest) (BlobReader, error)
	GetManifest(ctx context.Context, repo string, digest Digest) (BlobReader, error)
	GetTag(ctx context.Context, repo string, tagName string) (BlobReader, error)
}

type Writer interface {
	PushBlob(ctx context.Context, repo string, r io.Reader, desc Descriptor) (Descriptor, error)
	PushBlobChunked(ctx context.Context, repo string, resumeID string) (BlobWriter, error)
	PushManifest(ctx context.Context, repo string, data []byte, desc Descriptor) (Descriptor, error)
	MountBlob(ctx context.Context, repo string, fromRepo string, digest Digest) error
	Tag(ctx context.Context, repo string, digest Digest, tag string) error
}

type Deleter interface {
	DeleteBlob(ctx context.Context, repo string, digest Digest) error
	DeleteManifest(ctx context.Context, repo string, digest Digest) error
	DeleteTag(ctx context.Context, repo string, name string) error
}

type Lister interface {
	Repositories(ctx context.Context) Iter[string]
	Tags(ctx context.Context, repo string) Iter[string]
	Referrers(ctx context.Context, repo string, digest Digest, artifactType string) Iter[Descriptor]
}

// BlobWriter provides a handle for inserting data into a blob store.
type BlobWriter interface {
	io.WriteCloser
	io.ReaderFrom

	// Size returns the number of bytes written to this blob.
	Size() int64

	// ID returns the identifier for this writer. The returned value
	// can be passed to PushBlobChunked to resume the write.
	ID() string

	// Commit completes the blob writer process. The content is verified
	// against the provided provisional descriptor, which may result in an
	// error. Depending on the implementation, written data may be validated
	// against the provisional descriptor fields. If MediaType is not present,
	// the implementation may reject the commit or assign "application/octet-
	// stream" to the blob. The returned descriptor may have a different
	// digest depending on the blob store, referred to as the canonical
	// descriptor.
	Commit(ctx context.Context, provisional Descriptor) (canonical Descriptor, err error)

	// Cancel ends the blob write without storing any data and frees any
	// associated resources. Any data written thus far will be lost. Cancel
	// implementations should allow multiple calls even after a commit that
	// result in a no-op. This allows use of Cancel in a defer statement,
	// increasing the assurance that it is correctly called.
	Cancel(ctx context.Context) error
}

// BlobReader provides the contents of a given blob or manifest.
type BlobReader interface {
	Descriptor() Descriptor
	Open() io.ReadCloser
	OpenRange(p0, p1 int64) io.ReadCloser
}
