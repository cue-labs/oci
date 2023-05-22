package ocirequest

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/opencontainers/go-digest"

	"github.com/rogpeppe/ociregistry"
)

// ParseError represents an error that can happen when parsing.
// The Err field holds one of the possible error values below.
type ParseError struct {
	Err error
}

func (e *ParseError) Error() string {
	return e.Err.Error()
}

func (e *ParseError) Unwrap() error {
	return e.Err
}

var (
	ErrNotFound          = errors.New("page not found")
	ErrBadlyFormedDigest = errors.New("badly formed digest")
	ErrMethodNotAllowed  = errors.New("method not allowed")
	ErrBadRequest        = errors.New("bad request")
)

func badAPIUseError(f string, a ...any) error {
	return ociregistry.NewError(fmt.Sprintf(f, a...), ociregistry.ErrUnsupported.Code(), nil)
}

type Request struct {
	Kind Kind

	// Repo holds the repository name. Valid for all request kinds
	// except ReqCatalogList and ReqPing.
	Repo string

	// Digest holds the digest being used in the request.
	// Valid for:
	//	ReqBlobMount
	//	ReqBlobUploadBlob
	//	ReqBlobGet
	//	ReqBlobHead
	//	ReqBlobDelete
	//	ReqBlobCompleteUpload
	//	ReqReferrersList
	//
	// Valid for these manifest requests when they're referring to a digest
	// rather than a tag:
	//	ReqManifestGet
	//	ReqManifestHead
	//	ReqManifestPut
	//	ReqManifestDelete
	Digest string

	// Tag holds the tag being used in the request. Valid for
	// these manifest requests when they're referring to a tag:
	//	ReqManifestGet
	//	ReqManifestHead
	//	ReqManifestPut
	//	ReqManifestDelete
	Tag string

	// FromRepo holds the repository name to mount from
	// for ReqBlobMount.
	FromRepo string

	// UploadID holds the upload identifier as used for
	// chunked uploads.
	// Valid for:
	//	ReqBlobUploadInfo
	//	ReqBlobUploadChunk
	UploadID string

	// ListN holds the maximum count for listing.
	// It's -1 to specify that all items should be returned.
	//
	// Valid for:
	//	ReqTagsList
	ListN int

	// listLast holds the item to start just after
	// when listing.
	//
	// Valid for:
	//	ReqTagsList
	ListLast string
}

type Kind int

const (
	// end-1	GET	/v2/	200	404/401
	ReqPing = Kind(iota)

	// Blob-related endpoints

	// end-2	GET	/v2/<name>/blobs/<digest>	200	404
	ReqBlobGet

	// end-2	HEAD	/v2/<name>/blobs/<digest>	200	404
	ReqBlobHead

	// end-10	DELETE	/v2/<name>/blobs/<digest>	202	404/405
	ReqBlobDelete

	// end-4a	POST	/v2/<name>/blobs/uploads/	202	404
	ReqBlobStartUpload

	// end-4b	POST	/v2/<name>/blobs/uploads/?digest=<digest>	201/202	404/400
	ReqBlobUploadBlob

	// end-11	POST	/v2/<name>/blobs/uploads/?mount=<digest>&from=<other_name>	201	404
	ReqBlobMount

	// end-13	GET	/v2/<name>/blobs/uploads/<reference>	204	404
	ReqBlobUploadInfo

	// end-5	PATCH	/v2/<name>/blobs/uploads/<reference>	202	404/416
	ReqBlobUploadChunk

	// end-6	PUT	/v2/<name>/blobs/uploads/<reference>?digest=<digest>	201	404/400
	ReqBlobCompleteUpload

	// Manifest-related endpoints

	// end-3	GET	/v2/<name>/manifests/<tagOrDigest>	200	404
	ReqManifestGet

	// end-3	HEAD	/v2/<name>/manifests/<tagOrDigest>	200	404
	ReqManifestHead

	// end-7	PUT	/v2/<name>/manifests/<tagOrDigest>	201	404
	ReqManifestPut

	// end-9	DELETE	/v2/<name>/manifests/<tagOrDigest>	202	404/400/405
	ReqManifestDelete

	// Tag-related endpoints

	// end-8a	GET	/v2/<name>/tags/list	200	404
	// end-8b	GET	/v2/<name>/tags/list?n=<integer>&last=<integer>	200	404
	ReqTagsList

	// Referrer-related endpoints

	// end-12a	GET	/v2/<name>/referrers/<digest>	200	404/400
	ReqReferrersList

	// Catalog endpoints (out-of-spec)
	// 	GET	/v2/_catalog
	ReqCatalogList
)

// Parse parses the given HTTP method and URL as an OCI registry request.
// It understands the endpoints described in the [distribution spec].
//
// If it returns an error, it will be of type *ParseError.
//
// [distribution spec]: https://github.com/opencontainers/distribution-spec/blob/main/spec.md#endpoints
func Parse(method string, u *url.URL) (*Request, error) {
	req, err := parse(method, u)
	if err != nil {
		return nil, &ParseError{err}
	}
	return req, nil
}

func parse(method string, u *url.URL) (*Request, error) {
	path := u.Path
	urlq, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return nil, err
	}

	var rreq Request
	if path == "/v2" || path == "/v2/" {
		rreq.Kind = ReqPing
		return &rreq, nil
	}
	path, ok := strings.CutPrefix(path, "/v2/")
	if !ok {
		return nil, ociregistry.NewError("unknown URL path", ociregistry.ErrNameUnknown.Code(), nil)
	}
	if path == "_catalog" {
		if method != "GET" {
			return nil, ErrMethodNotAllowed
		}
		rreq.Kind = ReqCatalogList
		return &rreq, nil
	}
	uploadPath, ok := strings.CutSuffix(path, "/blobs/uploads/")
	if !ok {
		uploadPath, ok = strings.CutSuffix(path, "/blobs/uploads")
	}
	if ok {
		rreq.Repo = uploadPath
		if !isValidRepoName(rreq.Repo) {
			return nil, ociregistry.ErrNameInvalid
		}
		if method != "POST" {
			return nil, ErrMethodNotAllowed
		}
		if d := urlq.Get("mount"); d != "" {
			// end-11
			rreq.Digest = d
			if !isValidDigest(rreq.Digest) {
				return nil, ociregistry.ErrDigestInvalid
			}
			rreq.FromRepo = urlq.Get("from")
			if rreq.FromRepo == "" {
				// There's no "from" argument so fall back to
				// a regular chunked upload.
				rreq.Kind = ReqBlobStartUpload
				// TODO does the "mount" query argument actually take effect in some way?
				rreq.Digest = ""
				return &rreq, nil
			}
			if !isValidRepoName(rreq.FromRepo) {
				return nil, ociregistry.ErrNameInvalid
			}
			rreq.Kind = ReqBlobMount
			return &rreq, nil
		}
		if d := urlq.Get("digest"); d != "" {
			// end-4b
			rreq.Digest = d
			if !isValidDigest(d) {
				return nil, ErrBadlyFormedDigest
			}
			rreq.Kind = ReqBlobUploadBlob
			return &rreq, nil
		}
		// end-4a
		rreq.Kind = ReqBlobStartUpload
		return &rreq, nil
	}
	path, last, ok := cutLast(path, "/")
	if !ok {
		return nil, ErrNotFound
	}
	path, lastButOne, ok := cutLast(path, "/")
	if !ok {
		return nil, ErrNotFound
	}
	switch lastButOne {
	case "blobs":
		rreq.Repo = path
		if !isValidDigest(last) {
			return nil, ErrBadlyFormedDigest
		}
		if !isValidRepoName(rreq.Repo) {
			return nil, ociregistry.ErrNameInvalid
		}
		rreq.Digest = last
		switch method {
		case "GET":
			rreq.Kind = ReqBlobGet
		case "HEAD":
			rreq.Kind = ReqBlobHead
		case "DELETE":
			rreq.Kind = ReqBlobDelete
		default:
			return nil, ErrMethodNotAllowed
		}
		return &rreq, nil
	case "uploads":
		repo, ok := strings.CutSuffix(path, "/blobs")
		if !ok {
			return nil, ErrNotFound
		}
		rreq.Repo = repo
		if !isValidRepoName(rreq.Repo) {
			return nil, ociregistry.ErrNameInvalid
		}
		// TODO this doesn't allow query parameters inside the upload ID
		// which is something that some registries (e.g. docker) use.
		// Do we need to do that?
		rreq.UploadID = last
		if rreq.UploadID == "" {
			return nil, ErrNotFound
		}
		switch method {
		case "GET":
			rreq.Kind = ReqBlobUploadInfo
		case "PATCH":
			rreq.Kind = ReqBlobUploadChunk
		case "PUT":
			rreq.Kind = ReqBlobCompleteUpload
			rreq.Digest = urlq.Get("digest")
			if !isValidDigest(rreq.Digest) {
				return nil, ErrBadlyFormedDigest
			}
		default:
			return nil, ErrMethodNotAllowed
		}
		return &rreq, nil
	case "manifests":
		rreq.Repo = path
		if !isValidRepoName(rreq.Repo) {
			return nil, ociregistry.ErrNameInvalid
		}
		switch {
		case isValidDigest(last):
			rreq.Digest = last
		case isValidTag(last):
			rreq.Tag = last
		default:
			return nil, ErrNotFound
		}
		switch method {
		case "GET":
			rreq.Kind = ReqManifestGet
		case "HEAD":
			rreq.Kind = ReqManifestHead
		case "PUT":
			rreq.Kind = ReqManifestPut
		case "DELETE":
			rreq.Kind = ReqManifestDelete
		default:
			return nil, ErrMethodNotAllowed
		}
		return &rreq, nil

	case "tags":
		if last != "list" {
			return nil, ErrNotFound
		}
		rreq.ListN = -1
		if nstr := urlq.Get("n"); nstr != "" {
			n, err := strconv.Atoi(nstr)
			if err != nil {
				return nil, ErrBadRequest // TODO withHTTPCode(http.StatusBadRequest, fmt.Errorf("n is not a valid integer"))
			}
			rreq.ListN = n
		}
		rreq.ListLast = urlq.Get("last")
		if method != "GET" {
			return nil, ErrMethodNotAllowed
		}
		rreq.Repo = path
		if !isValidRepoName(rreq.Repo) {
			return nil, ociregistry.ErrNameInvalid
		}
		rreq.Kind = ReqTagsList
		return &rreq, nil
	case "referrers":
		if !isValidDigest(last) {
			return nil, ErrBadlyFormedDigest
		}
		if method != "GET" {
			return nil, ErrMethodNotAllowed
		}
		rreq.Repo = path
		if !isValidRepoName(rreq.Repo) {
			return nil, ociregistry.ErrNameInvalid
		}
		rreq.Digest = last
		rreq.Kind = ReqReferrersList
		return &rreq, nil
	}
	return nil, ErrNotFound
}

func cutLast(s, sep string) (before, after string, found bool) {
	if i := strings.LastIndex(s, sep); i >= 0 {
		return s[:i], s[i+len(sep):], true
	}
	return "", s, false
}

var (
	tagPattern      = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9._-]{0,127}$`)
	repoNamePattern = regexp.MustCompile(`^[a-z0-9]+([._-][a-z0-9]+)*(/[a-z0-9]+([._-][a-z0-9]+)*)*$`)
)

func isValidRepoName(repoName string) bool {
	return repoNamePattern.MatchString(repoName)
}

func isValidTag(tag string) bool {
	return tagPattern.MatchString(tag)
}

func isValidDigest(d string) bool {
	_, err := digest.Parse(d)
	return err == nil
}
