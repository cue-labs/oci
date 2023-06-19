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
	r0, r1 := both(
		func() t2[ociregistry.Descriptor] {
			return mk2(u.r0.PushManifest(ctx, repo, tag, contents, mediaType))
		},
		func() t2[ociregistry.Descriptor] {
			return mk2(u.r1.PushManifest(ctx, repo, tag, contents, mediaType))
		},
	)
	if (r0.err == nil) == (r1.err == nil) {
		return r0.get()
	}
	return ociregistry.Descriptor{}, fmt.Errorf("one push succeeded where the other failed (TODO better error)")
}

func (u unifier) PushBlobChunked(ctx context.Context, repo string, id string, chunkSize int) (ociregistry.BlobWriter, error) {
	var id0, id1 string
	if id != "" {
		var ids []string
		data, err := base64.RawURLEncoding.DecodeString(id)
		if err != nil {
			return nil, fmt.Errorf("malformed ID: %v", err)
		}
		if err := json.Unmarshal(data, &ids); err != nil {
			return nil, fmt.Errorf("malformed ID %q: %v", id, err)
		}
		if len(ids) != 2 {
			return nil, fmt.Errorf("malformed ID %q (expected two elements)", id)
		}
		id0, id1 = ids[0], ids[1]
	}
	r0, r1 := both(
		func() t2[ociregistry.BlobWriter] {
			return mk2(u.r0.PushBlobChunked(ctx, repo, id0, chunkSize))
		},
		func() t2[ociregistry.BlobWriter] {
			return mk2(u.r1.PushBlobChunked(ctx, repo, id1, chunkSize))
		},
	)
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
		w0:   w0,
		w1:   w1,
		size: size,
	}, nil
}

func (u unifier) MountBlob(ctx context.Context, fromRepo, toRepo string, digest ociregistry.Digest) (ociregistry.Descriptor, error) {
	return bothResults(both(
		func() t2[ociregistry.Descriptor] {
			return mk2(u.r0.MountBlob(ctx, fromRepo, toRepo, digest))
		},
		func() t2[ociregistry.Descriptor] {
			return mk2(u.r1.MountBlob(ctx, fromRepo, toRepo, digest))
		},
	)).get()
}

type unifiedBlobWriter struct {
	w0, w1 ociregistry.BlobWriter
	size   int64
}

func (w *unifiedBlobWriter) Write(buf []byte) (int, error) {
	r := bothResults(both(
		func() t2[int] {
			return mk2(w.w0.Write(buf))
		},
		func() t2[int] {
			return mk2(w.w1.Write(buf))
		},
	))
	if r.err != nil {
		return 0, r.err
	}
	w.size += int64(len(buf))
	return len(buf), nil
}

func (w *unifiedBlobWriter) Close() error {
	return bothResults(both(
		func() t1 {
			return mk1(w.w0.Close())
		},
		func() t1 {
			return mk1(w.w1.Close())
		},
	)).err
}

func (w *unifiedBlobWriter) Cancel() error {
	return bothResults(both(
		func() t1 {
			return mk1(w.w0.Cancel())
		},
		func() t1 {
			return mk1(w.w1.Cancel())
		},
	)).err
}

func (w *unifiedBlobWriter) Size() int64 {
	return w.size
}

func (w *unifiedBlobWriter) ID() string {
	data, _ := json.Marshal([]string{w.w0.ID(), w.w1.ID()})
	return base64.RawURLEncoding.EncodeToString(data)
}

func (w *unifiedBlobWriter) Commit(digest ociregistry.Digest) (ociregistry.Descriptor, error) {
	return bothResults(both(
		func() t2[ociregistry.Descriptor] {
			return mk2(w.w0.Commit(digest))
		},
		func() t2[ociregistry.Descriptor] {
			return mk2(w.w1.Commit(digest))
		},
	)).get()
}
