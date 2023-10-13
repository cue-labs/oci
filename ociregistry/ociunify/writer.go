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

package ociunify

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"

	"cuelabs.dev/go/oci/ociregistry"
)

func (u unifier) PushBlob(ctx context.Context, repo string, desc ociregistry.Descriptor, r io.Reader) (ociregistry.Descriptor, error) {
	resultc := make(chan t2[ociregistry.Descriptor])
	onePush := func(ri ociregistry.Interface, r *io.PipeReader) {
		desc, err := ri.PushBlob(ctx, repo, desc, r)
		r.CloseWithError(err)
		resultc <- t2[ociregistry.Descriptor]{desc, err}
	}
	pr0, pw0 := io.Pipe()
	pr1, pw1 := io.Pipe()
	go onePush(u.r0, pr0)
	go onePush(u.r1, pr1)
	go func() {
		_, err := io.Copy(io.MultiWriter(pw0, pw1), r)
		pw0.CloseWithError(err)
		pw1.CloseWithError(err)
	}()
	r0 := <-resultc
	r1 := <-resultc
	if (r0.err == nil) == (r1.err == nil) {
		return r0.get()
	}
	return ociregistry.Descriptor{}, fmt.Errorf("one push succeeded where the other failed (TODO better error)")
}

func (u unifier) PushManifest(ctx context.Context, repo string, tag string, contents []byte, mediaType string) (ociregistry.Descriptor, error) {
	r0, r1 := both(u, func(r ociregistry.Interface, _ int) t2[ociregistry.Descriptor] {
		return mk2(r.PushManifest(ctx, repo, tag, contents, mediaType))
	})
	if (r0.err == nil) == (r1.err == nil) {
		return r0.get()
	}
	return ociregistry.Descriptor{}, fmt.Errorf("one push succeeded where the other failed (TODO better error)")
}

func (u unifier) PushBlobChunked(ctx context.Context, repo string, chunkSize int) (ociregistry.BlobWriter, error) {
	r0, r1 := both(u, func(r ociregistry.Interface, i int) t2[ociregistry.BlobWriter] {
		return mk2(r.PushBlobChunked(ctx, repo, chunkSize))
	})
	if r0.err != nil || r1.err != nil {
		r0.close()
		r1.close()
		return nil, bothResults(r0, r1).err
	}
	w0, w1 := r0.x, r1.x
	size := w0.Size() // assumed to agree with w1.Size
	return &unifiedBlobWriter{
		w:    [2]ociregistry.BlobWriter{w0, w1},
		size: size,
	}, nil
}

func (u unifier) PushBlobChunkedResume(ctx context.Context, repo, id string, offset int64, chunkSize int) (ociregistry.BlobWriter, error) {
	data, err := base64.RawURLEncoding.DecodeString(id)
	if err != nil {
		return nil, fmt.Errorf("malformed ID: %v", err)
	}
	var ids []string
	if err := json.Unmarshal(data, &ids); err != nil {
		return nil, fmt.Errorf("malformed ID %q: %v", id, err)
	}
	if len(ids) != 2 {
		return nil, fmt.Errorf("malformed ID %q (expected two elements)", id)
	}
	r0, r1 := both(u, func(r ociregistry.Interface, i int) t2[ociregistry.BlobWriter] {
		return mk2(r.PushBlobChunkedResume(ctx, repo, ids[i], offset, chunkSize))
	})
	if r0.err != nil || r1.err != nil {
		r0.close()
		r1.close()
		return nil, bothResults(r0, r1).err
	}
	w0, w1 := r0.x, r1.x
	size := w0.Size()
	if w1.Size() != size {
		r0.close()
		r1.close()
		return nil, fmt.Errorf("registries do not agree on upload size; please start upload again")
	}
	return &unifiedBlobWriter{
		w:    [2]ociregistry.BlobWriter{w0, w1},
		size: size,
	}, nil
}

func (u unifier) MountBlob(ctx context.Context, fromRepo, toRepo string, digest ociregistry.Digest) (ociregistry.Descriptor, error) {
	return bothResults(both(u,
		func(r ociregistry.Interface, _ int) t2[ociregistry.Descriptor] {
			return mk2(r.MountBlob(ctx, fromRepo, toRepo, digest))
		},
	)).get()
}

type unifiedBlobWriter struct {
	u    unifier
	w    [2]ociregistry.BlobWriter
	size int64
}

func (w *unifiedBlobWriter) Write(buf []byte) (int, error) {
	r := bothResults(both(w.u, func(_ ociregistry.Interface, i int) t2[int] {
		return mk2(w.w[i].Write(buf))
	}))
	if r.err != nil {
		return 0, r.err
	}
	w.size += int64(len(buf))
	return len(buf), nil
}

func (w *unifiedBlobWriter) Close() error {
	return bothResults(both(w.u, func(_ ociregistry.Interface, i int) t1 {
		return mk1(w.w[i].Close())
	})).err
}

func (w *unifiedBlobWriter) Cancel() error {
	return bothResults(both(w.u, func(_ ociregistry.Interface, i int) t1 {
		return mk1(w.w[i].Cancel())
	})).err
}

func (w *unifiedBlobWriter) Size() int64 {
	return w.size
}

func (w *unifiedBlobWriter) ChunkSize() int {
	// ChunkSize can be derived from the server's required minimum, so take the maximum between both.
	// ChunkSize is usually a cheap method, so there's no need to call both concurrently.
	// TODO(mvdan): replace with max when we can assume Go 1.21
	s1, s2 := w.w[0].ChunkSize(), w.w[1].ChunkSize()
	if s2 > s1 {
		return s2
	}
	return s1
}

func (w *unifiedBlobWriter) ID() string {
	data, _ := json.Marshal([]string{w.w[0].ID(), w.w[1].ID()})
	return base64.RawURLEncoding.EncodeToString(data)
}

func (w *unifiedBlobWriter) Commit(digest ociregistry.Digest) (ociregistry.Descriptor, error) {
	return bothResults(both(w.u, func(_ ociregistry.Interface, i int) t2[ociregistry.Descriptor] {
		return mk2(w.w[i].Commit(digest))
	})).get()
}
