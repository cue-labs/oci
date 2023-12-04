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

package ocirequest

import (
	"net/url"
	"testing"

	"github.com/go-quicktest/qt"
)

var parseRequestTests = []struct {
	testName string
	method   string
	url      string

	wantRequest   *Request
	wantError     string
	wantConstruct string
}{{
	testName: "ping",
	method:   "GET",
	url:      "/v2",
	wantRequest: &Request{
		Kind: ReqPing,
	},
	wantConstruct: "/v2/",
}, {
	testName: "ping",
	method:   "GET",
	url:      "/v2/",
	wantRequest: &Request{
		Kind: ReqPing,
	},
}, {
	testName: "getBlob",
	method:   "GET",
	url:      "/v2/foo/bar/blobs/sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
	wantRequest: &Request{
		Kind:   ReqBlobGet,
		Repo:   "foo/bar",
		Digest: "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
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
	wantRequest: &Request{
		Kind: ReqBlobStartUpload,
		Repo: "somerepo",
	},
}, {
	testName: "uploadChunk",
	method:   "PATCH",
	url:      "/v2/somerepo/blobs/uploads/YmxhaGJsYWg",
	wantRequest: &Request{
		Kind:     ReqBlobUploadChunk,
		Repo:     "somerepo",
		UploadID: "blahblah",
	},
}, {
	testName: "uploadChunk",
	method:   "PATCH",
	url:      "/v2/somerepo/blobs/uploads/YmxhaGJsYWg",
	wantRequest: &Request{
		Kind:     ReqBlobUploadChunk,
		Repo:     "somerepo",
		UploadID: "blahblah",
	},
}, {
	testName:  "badlyFormedUploadDigest",
	method:    "POST",
	url:       "/v2/foo/blobs/uploads?digest=sha256:fake",
	wantError: "badly formed digest",
}, {
	testName: "getUploadInfo",
	method:   "GET",
	url:      "/v2/myorg/myrepo/blobs/uploads/YmxhaGJsYWg",
	wantRequest: &Request{
		Kind:     ReqBlobUploadInfo,
		Repo:     "myorg/myrepo",
		UploadID: "blahblah",
	},
}, {
	testName: "mount",
	method:   "POST",
	url:      "/v2/x/y/blobs/uploads/?mount=sha256:c659529df24a1878f6df8d93c652280235a50b95e862d8e5cb566ee5b9ed6386&from=somewhere/other",
	wantRequest: &Request{
		Kind:     ReqBlobMount,
		Repo:     "x/y",
		Digest:   "sha256:c659529df24a1878f6df8d93c652280235a50b95e862d8e5cb566ee5b9ed6386",
		FromRepo: "somewhere/other",
	},
}, {
	testName: "mount2",
	method:   "POST",
	url:      "/v2/myorg/other/blobs/uploads/?from=myorg%2Fmyrepo&mount=sha256%3Ad647b322fff1e9dcb828ee67a6c6d1ed0ceef760988fdf54f9cfdeb96186e001",
	wantRequest: &Request{
		Kind:     ReqBlobMount,
		Repo:     "myorg/other",
		Digest:   "sha256:d647b322fff1e9dcb828ee67a6c6d1ed0ceef760988fdf54f9cfdeb96186e001",
		FromRepo: "myorg/myrepo",
	},
}, {
	testName: "mountWithNoFrom",
	method:   "POST",
	url:      "/v2/x/y/blobs/uploads/?mount=sha256:c659529df24a1878f6df8d93c652280235a50b95e862d8e5cb566ee5b9ed6386",
	wantRequest: &Request{
		Kind: ReqBlobStartUpload,
		Repo: "x/y",
	},
	wantConstruct: "/v2/x/y/blobs/uploads/",
}, {
	testName: "manifestHead",
	method:   "HEAD",
	url:      "/v2/myorg/myrepo/manifests/sha256:681aef2367e055f33cb8a6ab9c3090931f6eefd0c3ef15c6e4a79bdadfdb8982",
	wantRequest: &Request{
		Kind:   ReqManifestHead,
		Repo:   "myorg/myrepo",
		Digest: "sha256:681aef2367e055f33cb8a6ab9c3090931f6eefd0c3ef15c6e4a79bdadfdb8982",
	},
}}

func TestParseRequest(t *testing.T) {
	for _, test := range parseRequestTests {
		t.Run(test.testName, func(t *testing.T) {
			u, err := url.Parse(test.url)
			if err != nil {
				t.Fatal(err)
			}
			rreq, err := Parse(test.method, u)
			if test.wantError != "" {
				qt.Assert(t, qt.ErrorMatches(err, test.wantError))
				// TODO http code
				return
			}
			qt.Assert(t, qt.IsNil(err))
			qt.Assert(t, qt.DeepEquals(rreq, test.wantRequest))
			method, ustr := rreq.MustConstruct()
			if test.wantConstruct == "" {
				test.wantConstruct = test.url
			}

			qt.Check(t, qt.Equals(method, test.method))
			qt.Check(t, qt.Equals(canonURL(ustr), canonURL(test.wantConstruct)))
		})
	}
}

func canonURL(ustr string) string {
	u, err := url.Parse(ustr)
	if err != nil {
		panic(err)
	}
	qv := u.Query()
	if len(qv) == 0 {
		return ustr
	}
	u.RawQuery = qv.Encode()
	return u.String()
}
