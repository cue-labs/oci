package ociserver

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/opencontainers/go-digest"

	"github.com/rogpeppe/ociregistry"
)

type registryRequest struct {
	kind requestKind

	// repo holds the repository name. Valid for all request kinds
	// except reqCatalogList and reqPing.
	repo string

	// digest holds the digest being used in the request.
	// Valid for:
	//	reqBlobMount
	//	reqBlobUploadBlob
	//	reqBlobGet
	//	reqBlobHead
	//	reqBlobDelete
	//	reqBlobCompleteUpload
	//	reqReferrersList
	//
	// Valid for these manifest requests when they're referring to a digest
	// rather than a tag:
	//	reqManifestGet
	//	reqManifestHead
	//	reqManifestPut
	//	reqManifestDelete
	digest string

	// tag holds the tag being used in the request. Valid for
	// these manifest requests when they're referring to a tag:
	//	reqManifestGet
	//	reqManifestHead
	//	reqManifestPut
	//	reqManifestDelete
	tag string

	// fromRepo holds the repository name to mount from
	// for reqBlobMount.
	fromRepo string

	// uploadID holds the upload identifier as used for
	// chunked uploads.
	// Valid for:
	//	reqBlobUploadInfo
	//	reqBlobUploadChunk
	uploadID string

	// listN holds the maximum count for listing.
	// It's -1 to specify that all items should be returned.
	//
	// Valid for:
	//	reqTagsList
	listN int

	// listLast holds the item to start just after
	// when listing.
	//
	// Valid for:
	//	reqTagsList
	listLast string
}

type requestKind int

// The following constants represent classes of request. Each is dealt with in its own
// handler.
const (
	reqBlobKinds requestKind = (iota + 1) << 10
	reqManifestKinds
	reqTagKinds
	reqReferrerKinds
	reqCatalogKinds

	reqKindMask = 0xff << 10
)

const (
	// end-1	GET	/v2/	200	404/401
	reqPing = requestKind(iota)

	// Blob-related endpoints

	// end-2	GET	/v2/<name>/blobs/<digest>	200	404
	reqBlobGet = iota + reqBlobKinds

	// end-2	HEAD	/v2/<name>/blobs/<digest>	200	404
	reqBlobHead

	// end-10	DELETE	/v2/<name>/blobs/<digest>	202	404/405
	reqBlobDelete

	// end-4a	POST	/v2/<name>/blobs/uploads/	202	404
	reqBlobStartUpload

	// end-4b	POST	/v2/<name>/blobs/uploads/?digest=<digest>	201/202	404/400
	reqBlobUploadBlob

	// end-11	POST	/v2/<name>/blobs/uploads/?mount=<digest>&from=<other_name>	201	404
	reqBlobMount

	// end-13	GET	/v2/<name>/blobs/uploads/<reference>	204	404
	reqBlobUploadInfo

	// end-5	PATCH	/v2/<name>/blobs/uploads/<reference>	202	404/416
	reqBlobUploadChunk

	// end-6	PUT	/v2/<name>/blobs/uploads/<reference>?digest=<digest>	201	404/400
	reqBlobCompleteUpload

	// Manifest-related endpoints

	// end-3	GET	/v2/<name>/manifests/<tagOrDigest>	200	404
	reqManifestGet = iota + reqManifestKinds

	// end-3	HEAD	/v2/<name>/manifests/<tagOrDigest>	200	404
	reqManifestHead

	// end-7	PUT	/v2/<name>/manifests/<tagOrDigest>	201	404
	reqManifestPut

	// end-9	DELETE	/v2/<name>/manifests/<tagOrDigest>	202	404/400/405
	reqManifestDelete

	// Tag-related endpoints

	// end-8a	GET	/v2/<name>/tags/list	200	404
	// end-8b	GET	/v2/<name>/tags/list?n=<integer>&last=<integer>	200	404
	reqTagsList = iota + reqTagKinds

	// Referrer-related endpoints

	// end-12a	GET	/v2/<name>/referrers/<digest>	200	404/400
	reqReferrersList = iota + reqReferrerKinds

	// Catalog endpoints
	// (out-of-spec)
	reqCatalogList = iota + reqCatalogKinds
)

func parseRequest(req *http.Request) (*registryRequest, error) {
	path := req.URL.Path
	urlq, err := url.ParseQuery(req.URL.RawQuery)
	if err != nil {
		return nil, err
	}

	var rreq registryRequest
	if path == "/v2" || path == "/v2/" {
		rreq.kind = reqPing
		return &rreq, nil
	}
	path, ok := strings.CutPrefix(path, "/v2/")
	if !ok {
		return nil, ociregistry.NewError("unknown URL path", ociregistry.ErrNameUnknown.Code(), nil)
	}
	if path == "_catalog" {
		if req.Method != "GET" {
			return nil, errMethodNotAllowed
		}
		rreq.kind = reqCatalogList
		return &rreq, nil
	}
	uploadPath, ok := strings.CutSuffix(path, "/blobs/uploads/")
	if !ok {
		uploadPath, ok = strings.CutSuffix(path, "/blobs/uploads")
	}
	if ok {
		rreq.repo = uploadPath
		if !isValidRepoName(rreq.repo) {
			return nil, ociregistry.ErrNameInvalid
		}
		if req.Method != "POST" {
			return nil, errMethodNotAllowed
		}
		if d := urlq.Get("mount"); d != "" {
			// end-11
			rreq.digest = d
			if !isValidDigest(rreq.digest) {
				return nil, ociregistry.ErrDigestInvalid
			}
			rreq.fromRepo = urlq.Get("from")
			if rreq.fromRepo == "" {
				// There's no "from" argument so fall back to
				// a regular chunked upload.
				rreq.kind = reqBlobStartUpload
				// TODO does the "mount" query argument actually take effect in some way?
				rreq.digest = ""
				return &rreq, nil
			}
			if !isValidRepoName(rreq.fromRepo) {
				return nil, ociregistry.ErrNameInvalid
			}
			rreq.kind = reqBlobMount
			return &rreq, nil
		}
		if d := urlq.Get("digest"); d != "" {
			// end-4b
			rreq.digest = d
			if !isValidDigest(d) {
				return nil, errBadlyFormedDigest
			}
			rreq.kind = reqBlobUploadBlob
			return &rreq, nil
		}
		// end-4a
		rreq.kind = reqBlobStartUpload
		return &rreq, nil
	}
	path, last, ok := cutLast(path, "/")
	if !ok {
		return nil, errNotFound
	}
	path, lastButOne, ok := cutLast(path, "/")
	if !ok {
		return nil, errNotFound
	}
	switch lastButOne {
	case "blobs":
		rreq.repo = path
		if !isValidDigest(last) {
			return nil, errBadlyFormedDigest
		}
		if !isValidRepoName(rreq.repo) {
			return nil, ociregistry.ErrNameInvalid
		}
		rreq.digest = last
		switch req.Method {
		case "GET":
			rreq.kind = reqBlobGet
		case "HEAD":
			rreq.kind = reqBlobHead
		case "DELETE":
			rreq.kind = reqBlobDelete
		default:
			return nil, errMethodNotAllowed
		}
		return &rreq, nil
	case "uploads":
		repo, ok := strings.CutSuffix(path, "/blobs")
		if !ok {
			return nil, errNotFound
		}
		rreq.repo = repo
		if !isValidRepoName(rreq.repo) {
			return nil, ociregistry.ErrNameInvalid
		}
		// TODO this doesn't allow query parameters inside the upload ID
		// which is something that some registries (e.g. docker) use.
		// Do we need to do that?
		rreq.uploadID = last
		if rreq.uploadID == "" {
			return nil, errNotFound
		}
		switch req.Method {
		case "GET":
			rreq.kind = reqBlobUploadInfo
		case "PATCH":
			rreq.kind = reqBlobUploadChunk
		case "PUT":
			rreq.kind = reqBlobCompleteUpload
			rreq.digest = urlq.Get("digest")
			if !isValidDigest(rreq.digest) {
				return nil, errBadlyFormedDigest
			}
		default:
			return nil, errMethodNotAllowed
		}
		return &rreq, nil
	case "manifests":
		rreq.repo = path
		if !isValidRepoName(rreq.repo) {
			return nil, ociregistry.ErrNameInvalid
		}
		switch {
		case isValidDigest(last):
			rreq.digest = last
		case isValidTag(last):
			rreq.tag = last
		default:
			return nil, errNotFound
		}
		switch req.Method {
		case "GET":
			rreq.kind = reqManifestGet
		case "HEAD":
			rreq.kind = reqManifestHead
		case "PUT":
			rreq.kind = reqManifestPut
		case "DELETE":
			rreq.kind = reqManifestDelete
		default:
			return nil, errMethodNotAllowed
		}
		return &rreq, nil

	case "tags":
		if last != "list" {
			return nil, errNotFound
		}
		rreq.listN = -1
		if nstr := urlq.Get("n"); nstr != "" {
			n, err := strconv.Atoi(nstr)
			if err != nil {
				return nil, withHTTPCode(http.StatusBadRequest, fmt.Errorf("n is not a valid integer"))
			}
			rreq.listN = n
		}
		rreq.listLast = urlq.Get("last")
		if req.Method != "GET" {
			return nil, errMethodNotAllowed
		}
		rreq.repo = path
		if !isValidRepoName(rreq.repo) {
			return nil, ociregistry.ErrNameInvalid
		}
		rreq.kind = reqTagsList
		return &rreq, nil
	case "referrers":
		if !isValidDigest(last) {
			return nil, errBadlyFormedDigest
		}
		if req.Method != "GET" {
			return nil, errMethodNotAllowed
		}
		rreq.repo = path
		if !isValidRepoName(rreq.repo) {
			return nil, ociregistry.ErrNameInvalid
		}
		rreq.digest = last
		rreq.kind = reqReferrersList
		return &rreq, nil
	}
	return nil, errNotFound
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
