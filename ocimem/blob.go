package ocimem

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"

	"github.com/opencontainers/go-digest"

	"github.com/rogpeppe/ociregistry"
)

// NewBytesReader returns an implementation of ociregistry.BlobReader
// that returns the given bytes. It fills in desc.Digest and desc.Size.
func NewBytesReader(data []byte, desc ociregistry.Descriptor) ociregistry.BlobReader {
	r := &bytesReader{
		desc: desc,
	}
	r.r.Reset(data)
	return r
}

type bytesReader struct {
	r    bytes.Reader
	desc ociregistry.Descriptor
}

func (r *bytesReader) Close() error {
	return nil
}

// Descriptor implements [ociregistry.BlobReader.Descriptor].
func (r *bytesReader) Descriptor() ociregistry.Descriptor {
	return r.desc
}

func (r *bytesReader) Read(data []byte) (int, error) {
	return r.r.Read(data)
}

// Buffer holds an in-memory implementation of ociregistry.BlobWriter.
// The zero value is good to use.
type Buffer struct {
	commit    func(b *Buffer) error
	buf       []byte
	uuid      string
	committed bool
	desc      ociregistry.Descriptor
	commitErr error
}

// NewBuffer returns a buffer that calls commit with the
// returned buffer when [Buffer.Commit] is invoked successfully.
func NewBuffer(commit func(b *Buffer) error) *Buffer {
	return &Buffer{
		commit: commit,
	}
}

func (b *Buffer) Cancel(ctx context.Context) error {
	b.commitErr = fmt.Errorf("upload canceled")
	return nil
}

func (b *Buffer) Close() error {
	return nil
}

func (b *Buffer) Size() int64 {
	return int64(len(b.buf))
}

// GetBlob returns any committed data and is descriptor. It returns an error
// if the data hasn't been committed or there was an error doing so.
func (b *Buffer) GetBlob() (ociregistry.Descriptor, []byte, error) {
	if !b.committed {
		return ociregistry.Descriptor{}, nil, fmt.Errorf("blob not committed")
	}
	if b.commitErr != nil {
		return ociregistry.Descriptor{}, nil, b.commitErr
	}
	return b.desc, b.buf, nil
}

// Write implements io.Writer by writing some data to the blob.
func (b *Buffer) Write(data []byte) (int, error) {
	b.buf = append(b.buf, data...)
	return len(data), nil
}

// ID implements [ociregistry.BlobWriter.ID] by returning a randomly
// allocated hex UUID.
func (b *Buffer) ID() string {
	if b.uuid == "" {
		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err != nil {
			panic(err)
		}
		b.uuid = fmt.Sprintf("%x", buf)
	}
	return b.uuid
}

// Commit implements [ociregistry.BlobWriter.Commit].
func (b *Buffer) Commit(ctx context.Context, dig ociregistry.Digest) (_ ociregistry.Digest, err error) {
	if b.commitErr != nil {
		return "", b.commitErr
	}
	defer func() {
		if err != nil {
			b.commitErr = err
		}
	}()
	if digest.FromBytes(b.buf) != dig {
		return "", fmt.Errorf("digest mismatch")
	}
	b.desc = ociregistry.Descriptor{
		MediaType: "application/octet-stream",
		Digest:    dig,
		Size:      b.Size(),
	}
	b.committed = true
	return dig, nil
}
