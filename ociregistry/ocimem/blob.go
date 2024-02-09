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

package ocimem

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"sync"

	"github.com/opencontainers/go-digest"

	"cuelabs.dev/go/oci/ociregistry"
)

// NewBytesReader returns an implementation of ociregistry.BlobReader
// that returns the given bytes. The returned reader will return desc from its
// Descriptor method.
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
type Buffer struct {
	commit           func(b *Buffer) error
	mu               sync.Mutex
	buf              []byte
	checkStartOffset int64
	uuid             string
	committed        bool
	desc             ociregistry.Descriptor
	commitErr        error
}

// NewBuffer returns a buffer that calls commit with the
// when [Buffer.Commit] is invoked successfully.
// /
// It's OK to call methods concurrently on a buffer.
func NewBuffer(commit func(b *Buffer) error, uuid string) *Buffer {
	if uuid == "" {
		uuid = newUUID()
	}
	return &Buffer{
		commit: commit,
		uuid:   uuid,
	}
}

func (b *Buffer) Cancel() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.commitErr = fmt.Errorf("upload canceled")
	return nil
}

func (b *Buffer) Close() error {
	return nil
}

func (b *Buffer) Size() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return int64(len(b.buf))
}

func (b *Buffer) ChunkSize() int {
	return 8 * 1024 // 8KiB; not really important
}

// GetBlob returns any committed data and is descriptor. It returns an error
// if the data hasn't been committed or there was an error doing so.
func (b *Buffer) GetBlob() (ociregistry.Descriptor, []byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
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
	b.mu.Lock()
	defer b.mu.Unlock()
	if offset := b.checkStartOffset; offset != -1 {
		// Can't call Buffer.Size, since we are already holding the mutex.
		if int64(len(b.buf)) != offset {
			return 0, fmt.Errorf("invalid offset %d in resumed upload (actual offset %d): %w", offset, len(b.buf), ociregistry.ErrRangeInvalid)
		}
		// Only check on the first write, since it's the start offset.
		b.checkStartOffset = -1
	}
	b.buf = append(b.buf, data...)
	return len(data), nil
}

func newUUID() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return fmt.Sprintf("%x", buf)
}

// ID implements [ociregistry.BlobWriter.ID] by returning a randomly
// allocated hex UUID.
func (b *Buffer) ID() string {
	return b.uuid
}

// Commit implements [ociregistry.BlobWriter.Commit] by checking
// that everything looks OK and calling the commit function if so.
func (b *Buffer) Commit(dig ociregistry.Digest) (_ ociregistry.Descriptor, err error) {
	if err := b.checkCommit(dig); err != nil {
		return ociregistry.Descriptor{}, err
	}
	// Note: we're careful to call this function outside of the mutex so
	// that it can call locked Buffer methods OK.
	if err := b.commit(b); err != nil {
		b.mu.Lock()
		defer b.mu.Unlock()

		b.commitErr = err
		return ociregistry.Descriptor{}, err
	}
	return ociregistry.Descriptor{
		MediaType: "application/octet-stream",
		Size:      int64(len(b.buf)),
		Digest:    dig,
	}, nil
}

func (b *Buffer) checkCommit(dig ociregistry.Digest) (err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.commitErr != nil {
		return b.commitErr
	}
	defer func() {
		if err != nil {
			b.commitErr = err
		}
	}()
	if digest.FromBytes(b.buf) != dig {
		return fmt.Errorf("digest mismatch (sha256(%q) != %s): %w", b.buf, dig, ociregistry.ErrDigestInvalid)
	}
	b.desc = ociregistry.Descriptor{
		MediaType: "application/octet-stream",
		Digest:    dig,
		Size:      int64(len(b.buf)),
	}
	b.committed = true
	return nil
}
