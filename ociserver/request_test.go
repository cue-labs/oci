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
