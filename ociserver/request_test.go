package ociserver

import (
	"net/http"
	"testing"

	"github.com/go-quicktest/qt"
	"github.com/google/go-cmp/cmp"
)

var parseRequestTests = []struct {
	testName string
	method   string
	url      string

	wantRequest  *registryRequest
	wantError    string
	wantHTTPCode int
}{{
	testName: "ping",
	method:   "GET",
	url:      "/v2",
	wantRequest: &registryRequest{
		kind: reqPing,
	},
}, {
	testName: "getBlob",
	method:   "GET",
	url:      "/v2/foo/bar/blobs/sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
	wantRequest: &registryRequest{
		kind:   reqBlobGet,
		repo:   "foo/bar",
		digest: "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
	},
}, {
	testName:  "getBlobInvalidDigest",
	method:    "GET",
	url:       "/v2/foo/bar/blobs/sha256:wrong",
	wantError: `badly formed digest`,
}, {
	testName:  "getBlobInvalidRepo",
	method:    "GET",
	url:       "/v2/foo/bAr/blobs/sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
	wantError: `invalid repository name`,
}, {
	testName: "startUpload",
	method:   "POST",
	url:      "/v2/somerepo/blobs/uploads/",
	wantRequest: &registryRequest{
		kind: reqBlobStartUpload,
		repo: "somerepo",
	},
}, {
	testName: "uploadChunk",
	method:   "PATCH",
	url:      "/v2/somerepo/blobs/uploads/blahblah",
	wantRequest: &registryRequest{
		kind:     reqBlobUploadChunk,
		repo:     "somerepo",
		uploadID: "blahblah",
	},
}, {
	testName: "uploadChunk",
	method:   "PATCH",
	url:      "/v2/somerepo/blobs/uploads/blahblah",
	wantRequest: &registryRequest{
		kind:     reqBlobUploadChunk,
		repo:     "somerepo",
		uploadID: "blahblah",
	},
}, {
	testName:  "badlyFormedUploadDigest",
	method:    "POST",
	url:       "/v2/foo/blobs/uploads?digest=sha256:fake",
	wantError: "badly formed digest",
}, {
	testName: "getUploadInfo",
	method:   "GET",
	url:      "/v2/myorg/myrepo/blobs/uploads/c659529df24a1878f6df8d93c652280235a50b95e862d8e5cb566ee5b9ed6386",
	wantRequest: &registryRequest{
		kind:     reqBlobUploadInfo,
		repo:     "myorg/myrepo",
		uploadID: "c659529df24a1878f6df8d93c652280235a50b95e862d8e5cb566ee5b9ed6386",
	},
}, {
	testName: "mount",
	method:   "POST",
	url:      "/v2/x/y/blobs/uploads/?mount=sha256:c659529df24a1878f6df8d93c652280235a50b95e862d8e5cb566ee5b9ed6386&from=somewhere/other",
	wantRequest: &registryRequest{
		kind:     reqBlobMount,
		repo:     "x/y",
		digest:   "sha256:c659529df24a1878f6df8d93c652280235a50b95e862d8e5cb566ee5b9ed6386",
		fromRepo: "somewhere/other",
	},
}, {
	testName: "mount2",
	method:   "POST",
	url:      "/v2/myorg/other/blobs/uploads/?from=myorg%2Fmyrepo&mount=sha256%3Ad647b322fff1e9dcb828ee67a6c6d1ed0ceef760988fdf54f9cfdeb96186e001",
	wantRequest: &registryRequest{
		kind:     reqBlobMount,
		repo:     "myorg/other",
		digest:   "sha256:d647b322fff1e9dcb828ee67a6c6d1ed0ceef760988fdf54f9cfdeb96186e001",
		fromRepo: "myorg/myrepo",
	},
}, {
	testName: "mountWithNoFrom",
	method:   "POST",
	url:      "/v2/x/y/blobs/uploads/?mount=sha256:c659529df24a1878f6df8d93c652280235a50b95e862d8e5cb566ee5b9ed6386",
	wantRequest: &registryRequest{
		kind: reqBlobStartUpload,
		repo: "x/y",
	},
}, {
	testName: "manifestHead",
	method:   "HEAD",
	url:      "/v2/myorg/myrepo/manifests/sha256:681aef2367e055f33cb8a6ab9c3090931f6eefd0c3ef15c6e4a79bdadfdb8982",
	wantRequest: &registryRequest{
		kind:   reqManifestHead,
		repo:   "myorg/myrepo",
		digest: "sha256:681aef2367e055f33cb8a6ab9c3090931f6eefd0c3ef15c6e4a79bdadfdb8982",
	},
}}

func TestParseRequest(t *testing.T) {
	for _, test := range parseRequestTests {
		t.Run(test.testName, func(t *testing.T) {
			req, err := http.NewRequest(test.method, test.url, nil)
			qt.Assert(t, qt.IsNil(err))
			rreq, err := parseRequest(req)
			if test.wantError != "" {
				qt.Assert(t, qt.ErrorMatches(err, test.wantError))
				// TODO http code
				return
			}
			qt.Assert(t, qt.IsNil(err))
			qt.Assert(t, qt.CmpEquals(rreq, test.wantRequest, cmp.AllowUnexported(registryRequest{})))
		})
	}
}
