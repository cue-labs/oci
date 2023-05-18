// Copyright 2018 Google LLC All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ociregistry_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/rogpeppe/ociregistry/ociserver"
)

const (
	weirdIndex = `{
  "manifests": [
	  {
			"digest":"sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae",
			"mediaType":"application/vnd.oci.image.layer.nondistributable.v1.tar+gzip"
		},{
			"digest":"sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae",
			"mediaType":"application/xml"
		},{
			"digest":"sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae",
			"mediaType":"application/vnd.oci.image.manifest.v1+json"
		}
	]
}`
)

func TestCalls(t *testing.T) {
	tcs := []struct {
		Description string

		// Request / setup
		URL           string
		Digests       map[string]string
		Manifests     map[string]string
		BlobStream    map[string]string
		RequestHeader map[string]string

		// Response
		Code   int
		Header map[string]string
		Method string
		Body   string // request body to send
		Want   string // response body to expect
	}{
		{
			Description: "/v2_returns_200",
			Method:      "GET",
			URL:         "/v2",
			Code:        http.StatusOK,
			Header:      map[string]string{"Docker-Distribution-API-Version": "registry/2.0"},
		},
		{
			Description: "/v2/_returns_200",
			Method:      "GET",
			URL:         "/v2/",
			Code:        http.StatusOK,
			Header:      map[string]string{"Docker-Distribution-API-Version": "registry/2.0"},
		},
		{
			Description: "/v2/bad_returns_404",
			Method:      "GET",
			URL:         "/v2/bad",
			Code:        http.StatusNotFound,
			Header:      map[string]string{"Docker-Distribution-API-Version": "registry/2.0"},
		},
		{
			Description: "GET_non_existent_blob",
			Method:      "GET",
			URL:         "/v2/foo/blobs/sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae",
			Code:        http.StatusNotFound,
		},
		{
			Description: "HEAD_non_existent_blob",
			Method:      "HEAD",
			URL:         "/v2/foo/blobs/sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae",
			Code:        http.StatusNotFound,
		},
		{
			Description: "GET_bad_digest",
			Method:      "GET",
			URL:         "/v2/foo/blobs/sha256:asd",
			Code:        http.StatusBadRequest,
		},
		{
			Description: "HEAD_bad_digest",
			Method:      "HEAD",
			URL:         "/v2/foo/blobs/sha256:asd",
			Code:        http.StatusBadRequest,
		},
		{
			Description: "bad_blob_verb",
			Method:      "FOO",
			URL:         "/v2/foo/blobs/sha256:asd",
			Code:        http.StatusBadRequest,
		},
		{
			Description: "GET_containerless_blob",
			Digests:     map[string]string{"sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae": "foo"},
			Method:      "GET",
			URL:         "/v2/foo/blobs/sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae",
			Code:        http.StatusOK,
			Header:      map[string]string{"Docker-Content-Digest": "sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae"},
			Want:        "foo",
		},
		{
			Description: "GET_blob",
			Digests:     map[string]string{"sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae": "foo"},
			Method:      "GET",
			URL:         "/v2/foo/blobs/sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae",
			Code:        http.StatusOK,
			Header:      map[string]string{"Docker-Content-Digest": "sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae"},
			Want:        "foo",
		},
		{
			Description: "HEAD_blob",
			Digests:     map[string]string{"sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae": "foo"},
			Method:      "HEAD",
			URL:         "/v2/foo/blobs/sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae",
			Code:        http.StatusOK,
			Header: map[string]string{
				"Content-Length":        "3",
				"Docker-Content-Digest": "sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae",
			},
		},
		{
			Description: "DELETE_blob",
			Digests:     map[string]string{"sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae": "foo"},
			Method:      "DELETE",
			URL:         "/v2/foo/blobs/sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae",
			Code:        http.StatusAccepted,
		},
		{
			Description: "blob_url_with_no_container",
			Method:      "GET",
			URL:         "/v2/blobs/sha256:asd",
			Code:        http.StatusBadRequest,
		},
		{
			Description: "uploadurl",
			Method:      "POST",
			URL:         "/v2/foo/blobs/uploads",
			Code:        http.StatusAccepted,
			Header:      map[string]string{"Range": "0-0"},
		},
		{
			Description: "uploadurl",
			Method:      "POST",
			URL:         "/v2/foo/blobs/uploads/",
			Code:        http.StatusAccepted,
			Header:      map[string]string{"Range": "0-0"},
		},
		{
			Description: "upload_put_missing_digest",
			Method:      "PUT",
			URL:         "/v2/foo/blobs/uploads/1",
			Code:        http.StatusBadRequest,
		},
		{
			Description: "monolithic_upload_good_digest",
			Method:      "POST",
			URL:         "/v2/foo/blobs/uploads?digest=sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae",
			Code:        http.StatusCreated,
			Body:        "foo",
			Header:      map[string]string{"Docker-Content-Digest": "sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae"},
		},
		{
			Description: "monolithic_upload_bad_digest",
			Method:      "POST",
			URL:         "/v2/foo/blobs/uploads?digest=sha256:fake",
			Code:        http.StatusBadRequest,
			Body:        "foo",
		},
		{
			Description: "upload_good_digest",
			Method:      "PUT",
			URL:         "/v2/foo/blobs/uploads/1?digest=sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae",
			Code:        http.StatusCreated,
			Body:        "foo",
			Header:      map[string]string{"Docker-Content-Digest": "sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae"},
		},
		{
			Description: "upload_bad_digest",
			Method:      "PUT",
			URL:         "/v2/foo/blobs/uploads/1?digest=sha256:baddigest",
			Code:        http.StatusBadRequest,
			Body:        "foo",
		},
		{
			Description: "stream_upload",
			Method:      "PATCH",
			URL:         "/v2/foo/blobs/uploads/1",
			Code:        http.StatusNoContent,
			Body:        "foo",
			Header: map[string]string{
				"Range":    "0-2",
				"Location": "/v2/foo/blobs/uploads/1",
			},
		},
		{
			Description: "stream_duplicate_upload",
			Method:      "PATCH",
			URL:         "/v2/foo/blobs/uploads/1",
			Code:        http.StatusBadRequest,
			Body:        "foo",
			BlobStream:  map[string]string{"1": "foo"},
		},
		{
			Description: "stream_finish_upload",
			Method:      "PUT",
			URL:         "/v2/foo/blobs/uploads/1?digest=sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae",
			BlobStream:  map[string]string{"1": "foo"},
			Code:        http.StatusCreated,
			Header:      map[string]string{"Docker-Content-Digest": "sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae"},
		},
		{
			Description: "get_missing_manifest",
			Method:      "GET",
			URL:         "/v2/foo/manifests/latest",
			Code:        http.StatusNotFound,
		},
		{
			Description: "head_missing_manifest",
			Method:      "HEAD",
			URL:         "/v2/foo/manifests/latest",
			Code:        http.StatusNotFound,
		},
		{
			Description: "get_missing_manifest_good_container",
			Manifests:   map[string]string{"foo/manifests/latest": "foo"},
			Method:      "GET",
			URL:         "/v2/foo/manifests/bar",
			Code:        http.StatusNotFound,
		},
		{
			Description: "head_missing_manifest_good_container",
			Manifests:   map[string]string{"foo/manifests/latest": "foo"},
			Method:      "HEAD",
			URL:         "/v2/foo/manifests/bar",
			Code:        http.StatusNotFound,
		},
		{
			Description: "get_manifest_by_tag",
			Manifests:   map[string]string{"foo/manifests/latest": "foo"},
			Method:      "GET",
			URL:         "/v2/foo/manifests/latest",
			Code:        http.StatusOK,
			Want:        "foo",
		},
		{
			Description: "get_manifest_by_digest",
			Manifests:   map[string]string{"foo/manifests/latest": "foo"},
			Method:      "GET",
			URL:         "/v2/foo/manifests/" + digestOf("foo"),
			Code:        http.StatusOK,
			Want:        "foo",
		},
		{
			Description: "head_manifest",
			Manifests:   map[string]string{"foo/manifests/latest": "foo"},
			Method:      "HEAD",
			URL:         "/v2/foo/manifests/latest",
			Code:        http.StatusOK,
		},
		{
			Description: "create_manifest",
			Method:      "PUT",
			URL:         "/v2/foo/manifests/latest",
			Code:        http.StatusCreated,
			Body:        "foo",
		},
		{
			Description: "create_index",
			Method:      "PUT",
			URL:         "/v2/foo/manifests/latest",
			Code:        http.StatusCreated,
			Body:        weirdIndex,
			RequestHeader: map[string]string{
				"Content-Type": "application/vnd.oci.image.index.v1+json",
			},
			Manifests: map[string]string{"foo/manifests/image": "foo"},
		},
		{
			Description: "create_index_missing_child",
			Method:      "PUT",
			URL:         "/v2/foo/manifests/latest",
			Code:        http.StatusNotFound,
			Body:        weirdIndex,
			RequestHeader: map[string]string{
				"Content-Type": "application/vnd.oci.image.index.v1+json",
			},
		},
		{
			Description: "bad_index_body",
			Method:      "PUT",
			URL:         "/v2/foo/manifests/latest",
			Code:        http.StatusBadRequest,
			Body:        "foo",
			RequestHeader: map[string]string{
				"Content-Type": "application/vnd.oci.image.index.v1+json",
			},
		},
		{
			Description: "bad_manifest_method",
			Method:      "BAR",
			URL:         "/v2/foo/manifests/latest",
			Code:        http.StatusBadRequest,
		},
		{
			Description:   "Chunk_upload_start",
			Method:        "PATCH",
			URL:           "/v2/foo/blobs/uploads/1",
			RequestHeader: map[string]string{"Content-Range": "0-3"},
			Code:          http.StatusNoContent,
			Body:          "foo",
			Header: map[string]string{
				"Range":    "0-2",
				"Location": "/v2/foo/blobs/uploads/1",
			},
		},
		{
			Description:   "Chunk_upload_bad_content_range",
			Method:        "PATCH",
			URL:           "/v2/foo/blobs/uploads/1",
			RequestHeader: map[string]string{"Content-Range": "0-bar"},
			Code:          http.StatusRequestedRangeNotSatisfiable,
			Body:          "foo",
		},
		{
			Description:   "Chunk_upload_overlaps_previous_data",
			Method:        "PATCH",
			URL:           "/v2/foo/blobs/uploads/1",
			BlobStream:    map[string]string{"1": "foo"},
			RequestHeader: map[string]string{"Content-Range": "2-5"},
			Code:          http.StatusRequestedRangeNotSatisfiable,
			Body:          "bar",
		},
		{
			Description:   "Chunk_upload_after_previous_data",
			Method:        "PATCH",
			URL:           "/v2/foo/blobs/uploads/1",
			BlobStream:    map[string]string{"1": "foo"},
			RequestHeader: map[string]string{"Content-Range": "3-6"},
			Code:          http.StatusNoContent,
			Body:          "bar",
			Header: map[string]string{
				"Range":    "0-5",
				"Location": "/v2/foo/blobs/uploads/1",
			},
		},
		{
			Description: "DELETE_Unknown_name",
			Method:      "DELETE",
			URL:         "/v2/test/honk/manifests/latest",
			Code:        http.StatusNotFound,
		},
		{
			Description: "DELETE_Unknown_manifest",
			Manifests:   map[string]string{"honk/manifests/latest": "honk"},
			Method:      "DELETE",
			URL:         "/v2/honk/manifests/tag-honk",
			Code:        http.StatusNotFound,
		},
		{
			Description: "DELETE_existing_manifest",
			Manifests:   map[string]string{"foo/manifests/latest": "foo"},
			Method:      "DELETE",
			URL:         "/v2/foo/manifests/latest",
			Code:        http.StatusAccepted,
		},
		{
			Description: "DELETE_existing_manifest_by_digest",
			Manifests:   map[string]string{"foo/manifests/latest": "foo"},
			Method:      "DELETE",
			URL:         "/v2/foo/manifests/" + digestOf("foo"),
			Code:        http.StatusAccepted,
		},
		{
			Description: "list_tags",
			Manifests:   map[string]string{"foo/manifests/latest": "foo", "foo/manifests/tag1": "foo"},
			Method:      "GET",
			URL:         "/v2/foo/tags/list?n=1000",
			Code:        http.StatusOK,
			Want:        `{"name":"foo","tags":["latest","tag1"]}`,
		},
		{
			Description: "limit_tags",
			Manifests:   map[string]string{"foo/manifests/latest": "foo", "foo/manifests/tag1": "foo"},
			Method:      "GET",
			URL:         "/v2/foo/tags/list?n=1",
			Code:        http.StatusOK,
			Want:        `{"name":"foo","tags":["latest"]}`,
		},
		{
			Description: "offset_tags",
			Manifests:   map[string]string{"foo/manifests/latest": "foo", "foo/manifests/tag1": "foo"},
			Method:      "GET",
			URL:         "/v2/foo/tags/list?last=latest",
			Code:        http.StatusOK,
			Want:        `{"name":"foo","tags":["tag1"]}`,
		},
		{
			Description: "list_non_existing_tags",
			Method:      "GET",
			URL:         "/v2/foo/tags/list?n=1000",
			Code:        http.StatusNotFound,
		},
		{
			Description: "list_repos",
			Manifests:   map[string]string{"foo/manifests/latest": "foo", "bar/manifests/latest": "bar"},
			Method:      "GET",
			URL:         "/v2/_catalog?n=1000",
			Code:        http.StatusOK,
		},
		{
			Description: "fetch_references",
			Method:      "GET",
			URL:         "/v2/foo/referrers/" + digestOf("foo"),
			Code:        http.StatusOK,
			Manifests: map[string]string{
				"foo/manifests/image":           "foo",
				"foo/manifests/points-to-image": "{\"subject\": {\"digest\": \"" + digestOf("foo") + "\"}}",
			},
		},
		{
			Description: "fetch_references,_subject_pointing_elsewhere",
			Method:      "GET",
			URL:         "/v2/foo/referrers/" + digestOf("foo"),
			Code:        http.StatusOK,
			Manifests: map[string]string{
				"foo/manifests/image":           "foo",
				"foo/manifests/points-to-image": "{\"subject\": {\"digest\": \"" + digestOf("nonexistant") + "\"}}",
			},
		},
		{
			Description: "fetch_references,_no_results",
			Method:      "GET",
			URL:         "/v2/foo/referrers/" + digestOf("foo"),
			Code:        http.StatusOK,
			Manifests: map[string]string{
				"foo/manifests/image": "foo",
			},
		},
		{
			Description: "fetch_references,_missing_repo",
			Method:      "GET",
			URL:         "/v2/does-not-exist/referrers/" + digestOf("foo"),
			Code:        http.StatusNotFound,
		},
		{
			Description: "fetch_references,_bad_target_(tag_vs._digest)",
			Method:      "GET",
			URL:         "/v2/foo/referrers/latest",
			Code:        http.StatusBadRequest,
		},
		{
			Description: "fetch_references,_bad_method",
			Method:      "POST",
			URL:         "/v2/foo/referrers/" + digestOf("foo"),
			Code:        http.StatusBadRequest,
		},
	}

	for _, tc := range tcs {

		testf := func(t *testing.T) {

			r := ociregistry.New(nil)
			s := httptest.NewServer(r)
			defer s.Close()

			for manifest, contents := range tc.Manifests {
				u, err := url.Parse(s.URL + "/v2/" + manifest)
				if err != nil {
					t.Fatalf("Error parsing %q: %v", s.URL+"/v2", err)
				}
				req := &http.Request{
					Method: "PUT",
					URL:    u,
					Body:   io.NopCloser(strings.NewReader(contents)),
				}
				t.Log(req.Method, req.URL)
				resp, err := s.Client().Do(req)
				if err != nil {
					t.Fatalf("Error uploading manifest: %v", err)
				}
				if resp.StatusCode != http.StatusCreated {
					body, _ := io.ReadAll(resp.Body)
					t.Fatalf("Error uploading manifest got status: %d %s", resp.StatusCode, body)
				}
				t.Logf("created manifest with digest %v", resp.Header.Get("Docker-Content-Digest"))
			}

			for digest, contents := range tc.Digests {
				u, err := url.Parse(fmt.Sprintf("%s/v2/foo/blobs/uploads/1?digest=%s", s.URL, digest))
				if err != nil {
					t.Fatalf("Error parsing %q: %v", s.URL+tc.URL, err)
				}
				req := &http.Request{
					Method: "PUT",
					URL:    u,
					Body:   io.NopCloser(strings.NewReader(contents)),
				}
				t.Log(req.Method, req.URL)
				resp, err := s.Client().Do(req)
				if err != nil {
					t.Fatalf("Error uploading digest: %v", err)
				}
				if resp.StatusCode != http.StatusCreated {
					body, _ := io.ReadAll(resp.Body)
					t.Fatalf("Error uploading digest got status: %d %s", resp.StatusCode, body)
				}
			}

			for upload, contents := range tc.BlobStream {
				u, err := url.Parse(fmt.Sprintf("%s/v2/foo/blobs/uploads/%s", s.URL, upload))
				if err != nil {
					t.Fatalf("Error parsing %q: %v", s.URL+tc.URL, err)
				}
				req := &http.Request{
					Method: "PATCH",
					URL:    u,
					Body:   io.NopCloser(strings.NewReader(contents)),
				}
				t.Log(req.Method, req.URL)
				resp, err := s.Client().Do(req)
				if err != nil {
					t.Fatalf("Error streaming blob: %v", err)
				}
				if resp.StatusCode != http.StatusNoContent {
					body, _ := io.ReadAll(resp.Body)
					t.Fatalf("Error streaming blob: %d %s", resp.StatusCode, body)
				}

			}

			u, err := url.Parse(s.URL + tc.URL)
			if err != nil {
				t.Fatalf("Error parsing %q: %v", s.URL+tc.URL, err)
			}
			req := &http.Request{
				Method: tc.Method,
				URL:    u,
				Body:   io.NopCloser(strings.NewReader(tc.Body)),
				Header: map[string][]string{},
			}
			for k, v := range tc.RequestHeader {
				req.Header.Set(k, v)
			}
			t.Log(req.Method, req.URL)
			resp, err := s.Client().Do(req)
			if err != nil {
				t.Fatalf("Error getting %q: %v", tc.URL, err)
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Errorf("Reading response body: %v", err)
			}
			if resp.StatusCode != tc.Code {
				t.Errorf("Incorrect status code, got %d, want %d; body: %s", resp.StatusCode, tc.Code, body)
			}

			for k, v := range tc.Header {
				r := resp.Header.Get(k)
				if r != v {
					t.Errorf("Incorrect header %q received, got %q, want %q", k, r, v)
				}
			}

			if tc.Want != "" && string(body) != tc.Want {
				t.Errorf("Incorrect response body, got %q, want %q", body, tc.Want)
			}
		}
		t.Run(tc.Description, testf)
		t.Run(tc.Description+" - custom log", testf)
	}
}

func digestOf(s string) string {
	return string(digest.FromString(s))
}
