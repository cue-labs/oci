package ocirequest

import (
	"fmt"
	"net/url"
)

func (req *Request) Construct() (method string, url string) {
	switch req.Kind {
	case ReqPing:
		return "GET", "/v2/"
	case ReqBlobGet:
		return "GET", "/v2/" + req.Repo + "/blobs/" + req.Digest
	case ReqBlobHead:
		return "HEAD", "/v2/" + req.Repo + "/blobs/" + req.Digest
	case ReqBlobDelete:
		return "DELETE", "/v2/" + req.Repo + "/blobs/" + req.Digest
	case ReqBlobStartUpload:
		return "POST", "/v2/" + req.Repo + "/blobs/uploads/"
	case ReqBlobUploadBlob:
		return "POST", "/v2/" + req.Repo + "/blobs/uploads/?digest=" + req.Digest
	case ReqBlobMount:
		return "POST", "/v2/" + req.Repo + "/blobs/uploads/?mount=" + req.Digest + "&from=" + req.FromRepo
	case ReqBlobUploadInfo:
		// Note: this is specific to the ociserver implementation.
		return "GET", "/v2/" + req.Repo + "/blobs/uploads/" + req.UploadID
	case ReqBlobUploadChunk:
		// Note: this is specific to the ociserver implementation.
		return "PATCH", "/v2/" + req.Repo + "/blobs/uploads/" + req.UploadID
	case ReqBlobCompleteUpload:
		// Note: this is specific to the ociserver implementation.
		// TODO this is bogus when the upload ID contains query parameters.
		return "PUT", "/v2/" + req.Repo + "/blobs/uploads/" + req.UploadID + "?digest=" + req.Digest
	case ReqManifestGet:
		return "GET", "/v2/" + req.Repo + "/manifests/" + req.tagOrDigest()
	case ReqManifestHead:
		return "HEAD", "/v2/" + req.Repo + "/manifests/" + req.tagOrDigest()
	case ReqManifestPut:
		return "PUT", "/v2/" + req.Repo + "/manifests/" + req.tagOrDigest()
	case ReqManifestDelete:
		return "DELETE", "/v2/" + req.Repo + "/manifests/" + req.tagOrDigest()
	case ReqTagsList:
		return "GET", "/v2/" + req.Repo + "/tags/list" + req.listParams()
	case ReqReferrersList:
		return "GET", "/v2/" + req.Repo + "/referrers/" + req.Digest
	case ReqCatalogList:
		return "GET", "/v2/_catalog" + req.listParams()
	default:
		panic("invalid request kind")
	}
}

func (req *Request) listParams() string {
	q := make(url.Values)
	if req.ListN >= 0 {
		q.Set("n", fmt.Sprint(req.ListN))
	}
	if req.ListLast != "" {
		q.Set("last", req.ListLast)
	}
	if len(q) > 0 {
		return "?" + q.Encode()
	}
	return ""
}

func (req *Request) tagOrDigest() string {
	if req.Tag != "" {
		return req.Tag
	}
	return req.Digest
}
