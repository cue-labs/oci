package ociclient

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"sync"

	"github.com/opencontainers/go-digest"

	"github.com/rogpeppe/ociregistry"
	"github.com/rogpeppe/ociregistry/internal/ocirequest"
)

// This file implements the ociregistry.Writer methods.

func (c *client) PushManifest(ctx context.Context, repo string, tag string, contents []byte, mediaType string) (ociregistry.Descriptor, error) {
	if mediaType == "" {
		return ociregistry.Descriptor{}, fmt.Errorf("PushManifest called with empty mediaType")
	}
	desc := ociregistry.Descriptor{
		Digest:    digest.FromBytes(contents),
		Size:      int64(len(contents)),
		MediaType: mediaType,
	}

	rreq := &ocirequest.Request{
		Kind: ocirequest.ReqManifestPut,
		Repo: repo,
		Tag:  tag,
	}
	method, u := rreq.Construct()
	req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(contents))
	if err != nil {
		return ociregistry.Descriptor{}, err
	}
	req.Header.Set("Content-Type", mediaType)
	req.ContentLength = desc.Size
	resp, err := c.do(req, http.StatusCreated)
	if err != nil {
		return ociregistry.Descriptor{}, err
	}
	resp.Body.Close()
	return desc, nil
}

func (c *client) MountBlob(ctx context.Context, fromRepo, toRepo string, dig ociregistry.Digest) error {
	rreq := &ocirequest.Request{
		Kind:     ocirequest.ReqBlobMount,
		Repo:     toRepo,
		FromRepo: fromRepo,
		Digest:   string(dig),
	}
	resp, err := c.doRequest(ctx, rreq, http.StatusCreated, http.StatusAccepted)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusAccepted {
		// Mount isn't supported and technically the upload session has begun,
		// but we aren't in a great position to be able to continue it, so let's just
		// return Unsupported.
		return fmt.Errorf("registry does not support mounts: %w", ociregistry.ErrUnsupported)
	}
	return nil
}

func (c *client) PushBlob(ctx context.Context, repo string, desc ociregistry.Descriptor, r io.Reader) (_ ociregistry.Descriptor, _err error) {
	rreq := &ocirequest.Request{
		Kind:   ocirequest.ReqBlobUploadBlob,
		Repo:   repo,
		Digest: string(desc.Digest),
	}
	method, u := rreq.Construct()
	req, err := http.NewRequestWithContext(ctx, method, u, r)
	if err != nil {
		return ociregistry.Descriptor{}, err
	}
	req.ContentLength = desc.Size
	// Note: even though we know a better content type here, the spec
	// says that we must always use application/octet-stream.
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := c.do(req, http.StatusCreated, http.StatusAccepted)
	if err != nil {
		return ociregistry.Descriptor{}, err
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusCreated {
		return desc, nil
	}
	location, err := locationFromResponse(resp)
	if err != nil {
		return ociregistry.Descriptor{}, err
	}

	// Monolithic push not supported (the response is Accepted, not Created).
	// Retry as a PUT request (the first request counts as a POST).

	// Note: we can't use ocirequest.Request here because that's
	// specific to the ociserver implementation in this case.
	req, err = http.NewRequestWithContext(ctx, "PUT", "", r)
	if err != nil {
		return ociregistry.Descriptor{}, err
	}
	req.URL = urlWithDigest(location, string(desc.Digest))
	req.ContentLength = desc.Size
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Range", rangeString(0, desc.Size))
	resp, err = c.do(req, http.StatusCreated)
	if err != nil {
		return ociregistry.Descriptor{}, err
	}
	defer closeOnError(&_err, resp.Body)
	resp.Body.Close()
	return desc, nil
}

// TODO is this a reasonable default? We have to
// weigh up in-memory cost vs round-trip overhead.
const defaultChunkSize = 64 * 1024

func (c *client) PushBlobChunked(ctx context.Context, repo string, id string, chunkSize int) (ociregistry.BlobWriter, error) {
	if id == "" {
		resp, err := c.doRequest(ctx, &ocirequest.Request{
			Kind: ocirequest.ReqBlobStartUpload,
			Repo: repo,
		}, http.StatusAccepted)
		if err != nil {
			return nil, err
		}
		resp.Body.Close()
		location, err := locationFromResponse(resp)
		if err != nil {
			return nil, err
		}
		return &blobWriter{
			ctx:       ctx,
			client:    c,
			chunkSize: chunkSizeFromResponse(resp, chunkSize),
			chunk:     make([]byte, 0, chunkSize),
			location:  location,
		}, nil
	}
	// Try to find what offset we're meant to be writing at
	// by doing a GET to the location.
	req, err := http.NewRequest("GET", id, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(req, http.StatusNoContent)
	if err != nil {
		return nil, fmt.Errorf("cannot recover chunk offset: %v", err)
	}
	location, err := locationFromResponse(resp)
	if err != nil {
		return nil, fmt.Errorf("cannot get location from response: %v", err)
	}
	rangeStr := resp.Header.Get("Range")
	p0, p1, ok := parseRange(rangeStr)
	if !ok {
		return nil, fmt.Errorf("invalid range %q in response", rangeStr)
	}
	if p0 != 0 {
		return nil, fmt.Errorf("range %q does not start with 0", rangeStr)
	}
	if chunkSize == 0 {
		chunkSize = defaultChunkSize
	}
	return &blobWriter{
		ctx:       ctx,
		client:    c,
		chunkSize: chunkSizeFromResponse(resp, chunkSize),
		chunk:     make([]byte, 0, chunkSize),
		size:      p1,
		flushed:   p1,
		location:  location,
	}, nil
}

type blobWriter struct {
	client    *client
	chunkSize int
	ctx       context.Context

	// mu guards the fields below it.
	mu              sync.Mutex
	closed          bool
	chunk           []byte
	chunkInProgress []byte
	closeErr        error

	// size holds the size of the entire upload as seen from the
	// client perspective. Each call to Write increases this immediately.
	size int64

	// flushed holds the size of the upload as flushed to the server.
	// Each successfully flushed chunk increases this.
	flushed  int64
	location *url.URL
	response chan doResult
}

type doResult struct {
	resp *http.Response
	err  error
}

func (w *blobWriter) Write(buf []byte) (_n int, _err error) {
	log.Printf("blobWriter.Write %q {", buf)
	defer func() {
		log.Printf("} -> %d, %v", _n, _err)
	}()
	w.mu.Lock()
	defer w.mu.Unlock()
	nwritten := 0
	for {
		if len(w.chunk) < cap(w.chunk) {
			// Copy as much as we can into the chunk buffer.
			n := copy(w.chunk[len(w.chunk):cap(w.chunk)], buf)
			w.chunk = w.chunk[:len(w.chunk)+n]
			buf = buf[n:]
			nwritten += n
		}
		if len(w.chunk) < cap(w.chunk) {
			// We need more data before we can send the request,
			// so let the user write it.
			w.size += int64(nwritten)
			return nwritten, nil
		}
		// The chunk buffer is full. First flush it.
		n, err := w.flush()
		if err != nil {
			nwritten += n
			w.size += int64(nwritten)
			return nwritten, err
		}
		nwritten += n
	}
}

// flushAll flushes all the data in the writer and waits for
// all chunk uploads to complete.
func (w *blobWriter) flushAll() (int, error) {
	n, err := w.flush()
	if err != nil {
		return n, err
	}
	n1, err := w.flush()
	n += n1
	return n, err
}

// flush waits for any existing chunk upload to complete
// and starts the next chunk upload if there's some data
// to write.
func (w *blobWriter) flush() (_n int, _err error) {
	log.Printf("blobWriter.flush (chunk %d, inprogress %d) {", len(w.chunk), len(w.chunkInProgress))
	defer func() {
		log.Printf("} -> %v, %v", _n, _err)
	}()
	nwritten := 0
	if w.response != nil {
		// An upload PATCH is still
		// in progress; we can't make any progress now, so wait
		// for the response to complete before uploading the next chunk.
		select {
		case resp := <-w.response:
			if resp.err != nil {
				return 0, resp.err
			}
			nwritten += len(w.chunkInProgress)
			location, err := locationFromResponse(resp.resp)
			if err != nil {
				return nwritten, fmt.Errorf("bad Location in response: %v", err)
			}
			// TODO is there something we could be doing with the Range header in the response?
			w.location = location
			w.response = nil
		case <-w.ctx.Done():
			return 0, fmt.Errorf("context cancelled while sending data: %v", w.ctx.Err())
		}
	}
	// We just got confirmation from the server that we wrote the chunk.
	w.flushed += int64(len(w.chunkInProgress))
	// Now swap the buffers and process with a new PATCH.
	w.chunk, w.chunkInProgress = w.chunkInProgress, w.chunk
	w.chunk = w.chunk[:0]
	if cap(w.chunk) == 0 {
		w.chunk = make([]byte, 0, w.chunkSize)
	}

	if len(w.chunkInProgress) == 0 {
		// Nothing more to write.
		return nwritten, nil
	}
	// Start a new PATCH request to send the data in w.chunkInProgress.
	// It'll send on w.response when done
	req, err := http.NewRequestWithContext(w.ctx, "PATCH", "", bytes.NewReader(w.chunkInProgress))
	if err != nil {
		return nwritten, fmt.Errorf("cannot make PATCH request: %v", err)
	}
	req.URL = w.location
	req.Header.Set("Content-Range", rangeString(w.flushed, w.flushed+int64(len(w.chunkInProgress))))
	w.response = make(chan doResult, 1)
	go func() {
		log.Printf("sending PATCH with data %q", w.chunkInProgress)
		resp, err := w.client.do(req, http.StatusAccepted)
		if err == nil {
			resp.Body.Close()
		}
		w.response <- doResult{
			resp: resp,
			err:  err,
		}
	}()
	return nwritten, nil
}

func (w *blobWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return w.closeErr
	}
	_, err := w.flushAll()
	w.closed = true
	w.closeErr = err
	return err
}

func (w *blobWriter) Size() int64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.size
}

func (w *blobWriter) ID() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.location.String()
}

func (w *blobWriter) Commit(digest ociregistry.Digest) (ociregistry.Digest, error) {
	log.Printf("blobWriter.Commit %q", digest)
	w.mu.Lock()
	defer w.mu.Unlock()
	_, err := w.flushAll()
	if err != nil {
		return "", fmt.Errorf("cannot flush data before commit: %v", err)
	}
	req, _ := http.NewRequestWithContext(w.ctx, "PUT", "", nil)
	req.URL = urlWithDigest(w.location, string(digest))
	if _, err := w.client.do(req, http.StatusCreated); err != nil {
		return "", err
	}
	return digest, nil
}

func (w *blobWriter) Cancel() error {
	return nil
}

// urlWithDigest returns u with the digest query parameter set, taking care not
// to disrupt the initial URL (thus avoiding the charge of "manually
// assembing the location; see [here].
//
// [here]: https://github.com/opencontainers/distribution-spec/blob/main/spec.md#post-then-put
func urlWithDigest(u0 *url.URL, digest string) *url.URL {
	u := *u0
	digest = url.QueryEscape(digest)
	switch {
	case u.ForceQuery:
		// The URL already ended in a "?" with no actual query parameters.
		u.RawQuery = "digest=" + digest
		u.ForceQuery = false
	case u.RawQuery != "":
		// There's already a query parameter present.
		u.RawQuery += "&digest=" + digest
	default:
		u.RawQuery = "digest=" + digest
	}
	return &u
}

// See https://github.com/opencontainers/distribution-spec/blob/main/spec.md#pushing-a-blob-in-chunks
func chunkSizeFromResponse(resp *http.Response, chunkSize int) int {
	minChunkSize, err := strconv.Atoi(resp.Header.Get("OCI-Chunk-Min-Length"))
	if err == nil && minChunkSize > chunkSize {
		return minChunkSize
	}
	return chunkSize
}
