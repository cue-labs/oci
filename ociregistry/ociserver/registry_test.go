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

package ociserver_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"cuelabs.dev/go/oci/ociregistry/ocimem"
	"cuelabs.dev/go/oci/ociregistry/ociserver"
	"github.com/opencontainers/go-digest"
)

const (
	weirdIndex = `{
  "manifests": [
	  {
	  		"size": 3,
			"digest":"sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae",
			"mediaType":"application/vnd.oci.image.layer.nondistributable.v1.tar+gzip"
		},{
	  		"size": 3,
			"digest":"sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae",
			"mediaType":"application/xml"
		},{
	  		"size": 3,
			"digest":"sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae",
			"mediaType":"application/vnd.oci.image.manifest.v1+json"
		}
	]
}`
)

func TestCalls(t *testing.T) {
	tcs := []struct {
		skip bool

		Description string

		// Request / setup
		Method        string
		Body          string // request body to send
		URL           string
		Digests       map[string]string
		Manifests     map[string]string
		BlobStream    map[string]string
		RequestHeader map[string]string

		// Response
		WantCode   int
		WantHeader map[string]string
		WantBody   string // response body to expect
	}{
		{
			Description: "v2_returns_200",
			Method:      "GET",
			URL:         "/v2",
			WantCode:    http.StatusOK,
			WantHeader:  map[string]string{"Docker-Distribution-API-Version": "registry/2.0"},
		},
		{
			Description: "v2_slash_returns_200",
			Method:      "GET",
			URL:         "/v2/",
			WantCode:    http.StatusOK,
			WantHeader:  map[string]string{"Docker-Distribution-API-Version": "registry/2.0"},
		},
		{
			Description: "v2_bad_returns_404",
			Method:      "GET",
			URL:         "/v2/bad",
			WantCode:    http.StatusNotFound,
			WantHeader:  map[string]string{"Docker-Distribution-API-Version": "registry/2.0"},
		},
		{
			Description: "GET_non_existent_blob",
			Method:      "GET",
			URL:         "/v2/foo/blobs/sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae",
			WantCode:    http.StatusNotFound,
		},
		{
			Description: "HEAD_non_existent_blob",
			Method:      "HEAD",
			URL:         "/v2/foo/blobs/sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae",
			WantCode:    http.StatusNotFound,
		},
		{
			Description: "GET_bad_digest",
			Method:      "GET",
			URL:         "/v2/foo/blobs/sha256:asd",
			WantCode:    http.StatusBadRequest,
		},
		{
			Description: "HEAD_bad_digest",
			Method:      "HEAD",
			URL:         "/v2/foo/blobs/sha256:asd",
			WantCode:    http.StatusBadRequest,
		},
		{
			Description: "bad_blob_verb",
			Method:      "FOO",
			URL:         "/v2/foo/blobs/sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae",
			WantCode:    http.StatusMethodNotAllowed,
		},
		{
			Description: "GET_containerless_blob",
			Digests:     map[string]string{"sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae": "foo"},
			Method:      "GET",
			URL:         "/v2/foo/blobs/sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae",
			WantCode:    http.StatusOK,
			WantHeader:  map[string]string{"Docker-Content-Digest": "sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae"},
			WantBody:    "foo",
		},
		{
			Description: "GET_blob",
			Digests:     map[string]string{"sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae": "foo"},
			Method:      "GET",
			URL:         "/v2/foo/blobs/sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae",
			WantCode:    http.StatusOK,
			WantHeader:  map[string]string{"Docker-Content-Digest": "sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae"},
			WantBody:    "foo",
		},
		{
			Description: "GET_blob_range_defined_range",
			Digests: map[string]string{
				"sha256:b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9": "hello world",
			},
			Method: "GET",
			RequestHeader: map[string]string{
				"Range": "bytes=1-4",
			},
			URL:      "/v2/foo/blobs/sha256:b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9",
			WantCode: http.StatusPartialContent,
			WantHeader: map[string]string{
				"Docker-Content-Digest": "sha256:b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9",
				"Content-Length":        "4",
				"Content-Range":         "bytes 1-4/11",
			},
			WantBody: "ello",
		},
		{
			Description: "GET_blob_range_undefined_range_end",
			Digests: map[string]string{
				"sha256:b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9": "hello world",
			},
			Method: "GET",
			RequestHeader: map[string]string{
				"Range": "bytes=3-",
			},
			URL:      "/v2/foo/blobs/sha256:b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9",
			WantCode: http.StatusPartialContent,
			WantHeader: map[string]string{
				"Docker-Content-Digest": "sha256:b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9",
				"Content-Length":        "8",
				"Content-Range":         "bytes 3-10/11",
			},
			WantBody: "lo world",
		},
		{
			Description: "GET_blob_range_invalid-range-start",
			Digests: map[string]string{
				"sha256:b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9": "hello world",
			},
			Method: "GET",
			RequestHeader: map[string]string{
				"Range": "bytes=20-30",
			},
			URL: "/v2/foo/blobs/sha256:b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9",
			// TODO change the error interface to make it possible for ocimem
			// to return an error that results in a 416 status.
			WantCode: http.StatusInternalServerError,
		},
		{
			Description: "HEAD_blob",
			Digests:     map[string]string{"sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae": "foo"},
			Method:      "HEAD",
			URL:         "/v2/foo/blobs/sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae",
			WantCode:    http.StatusOK,
			WantHeader: map[string]string{
				"Content-Length":        "3",
				"Docker-Content-Digest": "sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae",
			},
		},
		{
			Description: "DELETE_blob",
			Digests:     map[string]string{"sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae": "foo"},
			Method:      "DELETE",
			URL:         "/v2/foo/blobs/sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae",
			WantCode:    http.StatusAccepted,
		},
		{
			Description: "blob_url_with_no_container",
			Method:      "GET",
			URL:         "/v2/blobs/sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae",
			WantCode:    http.StatusNotFound,
		},
		{
			Description: "uploadurl",
			Method:      "POST",
			URL:         "/v2/foo/blobs/uploads/",
			WantCode:    http.StatusAccepted,
			WantHeader:  map[string]string{"Range": "0-0"},
		},
		{
			Description: "uploadurl",
			Method:      "POST",
			URL:         "/v2/foo/blobs/uploads/",
			WantCode:    http.StatusAccepted,
			WantHeader:  map[string]string{"Range": "0-0"},
		},
		{
			Description: "upload_put_missing_digest",
			Method:      "PUT",
			URL:         "/v2/foo/blobs/uploads/MQ",
			WantCode:    http.StatusBadRequest,
		},
		{
			Description: "monolithic_upload_good_digest",
			Method:      "POST",
			URL:         "/v2/foo/blobs/uploads?digest=sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae",
			WantCode:    http.StatusCreated,
			Body:        "foo",
			WantHeader:  map[string]string{"Docker-Content-Digest": "sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae"},
		},
		{
			Description: "monolithic_upload_bad_digest",
			Method:      "POST",
			URL:         "/v2/foo/blobs/uploads?digest=sha256:fake",
			WantCode:    http.StatusBadRequest,
			Body:        "foo",
		},
		{
			Description: "upload_good_digest",
			Method:      "PUT",
			URL:         "/v2/foo/blobs/uploads/MQ?digest=sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae",
			WantCode:    http.StatusCreated,
			Body:        "foo",
			WantHeader:  map[string]string{"Docker-Content-Digest": "sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae"},
		},
		{
			Description: "upload_bad_digest",
			Method:      "PUT",
			URL:         "/v2/foo/blobs/uploads/MQ?digest=sha256:baddigest",
			WantCode:    http.StatusBadRequest,
			Body:        "foo",
		},
		{
			Description: "stream_upload",
			Method:      "PATCH",
			URL:         "/v2/foo/blobs/uploads/MQ",
			WantCode:    http.StatusAccepted,
			Body:        "foo",
			RequestHeader: map[string]string{
				"Content-Range": "0-2",
			},
			WantHeader: map[string]string{
				"Range":    "0-2",
				"Location": "/v2/foo/blobs/uploads/MQ",
			},
		},
		{
			skip:        true,
			Description: "stream_duplicate_upload",
			Method:      "PATCH",
			URL:         "/v2/foo/blobs/uploads/MQ",
			WantCode:    http.StatusBadRequest,
			Body:        "foo",
			BlobStream:  map[string]string{"MQ": "foo"},
		},
		{
			Description: "stream_finish_upload",
			Method:      "PUT",
			URL:         "/v2/foo/blobs/uploads/MQ?digest=sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae",
			BlobStream:  map[string]string{"MQ": "foo"},
			WantCode:    http.StatusCreated,
			WantHeader:  map[string]string{"Docker-Content-Digest": "sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae"},
		},
		{
			Description: "get_missing_manifest",
			Method:      "GET",
			URL:         "/v2/foo/manifests/latest",
			WantCode:    http.StatusNotFound,
		},
		{
			Description: "head_missing_manifest",
			Method:      "HEAD",
			URL:         "/v2/foo/manifests/latest",
			WantCode:    http.StatusNotFound,
		},
		{
			Description: "get_missing_manifest_good_container",
			Manifests:   map[string]string{"foo/manifests/latest": "foo"},
			Method:      "GET",
			URL:         "/v2/foo/manifests/bar",
			WantCode:    http.StatusNotFound,
		},
		{
			Description: "head_missing_manifest_good_container",
			Manifests:   map[string]string{"foo/manifests/latest": "foo"},
			Method:      "HEAD",
			URL:         "/v2/foo/manifests/bar",
			WantCode:    http.StatusNotFound,
		},
		{
			Description: "get_manifest_by_tag",
			Manifests:   map[string]string{"foo/manifests/latest": "foo"},
			Method:      "GET",
			URL:         "/v2/foo/manifests/latest",
			WantCode:    http.StatusOK,
			WantBody:    "foo",
		},
		{
			Description: "get_manifest_by_digest",
			Manifests:   map[string]string{"foo/manifests/latest": "foo"},
			Method:      "GET",
			URL:         "/v2/foo/manifests/" + digestOf("foo"),
			WantCode:    http.StatusOK,
			WantBody:    "foo",
		},
		{
			Description: "head_manifest",
			Manifests:   map[string]string{"foo/manifests/latest": "foo"},
			Method:      "HEAD",
			URL:         "/v2/foo/manifests/latest",
			WantCode:    http.StatusOK,
		},
		{
			Description: "create_manifest",
			Method:      "PUT",
			URL:         "/v2/foo/manifests/latest",
			WantCode:    http.StatusCreated,
			Body:        "foo",
		},
		{
			Description: "create_index",
			Method:      "PUT",
			URL:         "/v2/foo/manifests/latest",
			WantCode:    http.StatusCreated,
			Body:        weirdIndex,
			RequestHeader: map[string]string{
				"Content-Type": "application/vnd.oci.image.index.v1+json",
			},
			Manifests: map[string]string{"foo/manifests/image": "foo"},
		},
		{
			skip:        true,
			Description: "create_index_missing_child",
			Method:      "PUT",
			URL:         "/v2/foo/manifests/latest",
			WantCode:    http.StatusNotFound,
			Body:        weirdIndex,
			RequestHeader: map[string]string{
				"Content-Type": "application/vnd.oci.image.index.v1+json",
			},
		},
		{
			skip:        true,
			Description: "bad_index_body",
			Method:      "PUT",
			URL:         "/v2/foo/manifests/latest",
			WantCode:    http.StatusBadRequest,
			Body:        "foo",
			RequestHeader: map[string]string{
				"Content-Type": "application/vnd.oci.image.index.v1+json",
			},
		},
		{
			Description: "bad_manifest_method",
			Method:      "BAR",
			URL:         "/v2/foo/manifests/latest",
			WantCode:    http.StatusMethodNotAllowed,
		},
		{
			Description:   "Chunk_upload_start",
			Method:        "PATCH",
			URL:           "/v2/foo/blobs/uploads/MQ",
			RequestHeader: map[string]string{"Content-Range": "0-2"},
			WantCode:      http.StatusAccepted,
			Body:          "foo",
			WantHeader: map[string]string{
				"Range":    "0-2",
				"Location": "/v2/foo/blobs/uploads/MQ",
			},
		},
		{
			Description:   "Chunk_upload_bad_content_range",
			Method:        "PATCH",
			URL:           "/v2/foo/blobs/uploads/MQ",
			RequestHeader: map[string]string{"Content-Range": "0-bar"},
			// TODO the original had 405 response here. Which is correct?
			WantCode: http.StatusBadRequest,
			Body:     "foo",
		},
		{
			Description:   "Chunk_upload_overlaps_previous_data",
			Method:        "PATCH",
			URL:           "/v2/foo/blobs/uploads/MQ",
			BlobStream:    map[string]string{"MQ": "foo"},
			RequestHeader: map[string]string{"Content-Range": "2-4"},
			WantCode:      http.StatusRequestedRangeNotSatisfiable,
			Body:          "bar",
		},
		{
			Description:   "Chunk_upload_after_previous_data",
			Method:        "PATCH",
			URL:           "/v2/foo/blobs/uploads/MQ",
			BlobStream:    map[string]string{"MQ": "foo"},
			RequestHeader: map[string]string{"Content-Range": "3-5"},
			WantCode:      http.StatusAccepted,
			Body:          "bar",
			WantHeader: map[string]string{
				"Range":    "0-5",
				"Location": "/v2/foo/blobs/uploads/MQ",
			},
		},
		{
			Description: "DELETE_Unknown_name",
			Method:      "DELETE",
			URL:         "/v2/test/honk/manifests/latest",
			WantCode:    http.StatusNotFound,
		},
		{
			Description: "DELETE_Unknown_manifest",
			Manifests:   map[string]string{"honk/manifests/latest": "honk"},
			Method:      "DELETE",
			URL:         "/v2/honk/manifests/tag-honk",
			WantCode:    http.StatusNotFound,
		},
		{
			Description: "DELETE_existing_manifest",
			Manifests:   map[string]string{"foo/manifests/latest": "foo"},
			Method:      "DELETE",
			URL:         "/v2/foo/manifests/latest",
			WantCode:    http.StatusAccepted,
		},
		{
			Description: "DELETE_existing_manifest_by_digest",
			Manifests:   map[string]string{"foo/manifests/latest": "foo"},
			Method:      "DELETE",
			URL:         "/v2/foo/manifests/" + digestOf("foo"),
			WantCode:    http.StatusAccepted,
		},
		{
			Description: "list_tags",
			Manifests:   map[string]string{"foo/manifests/latest": "foo", "foo/manifests/tag1": "foo"},
			Method:      "GET",
			URL:         "/v2/foo/tags/list?n=1000",
			WantCode:    http.StatusOK,
			WantBody:    `{"name":"foo","tags":["latest","tag1"]}`,
		},
		{
			Description: "limit_tags",
			Manifests:   map[string]string{"foo/manifests/latest": "foo", "foo/manifests/tag1": "foo"},
			Method:      "GET",
			URL:         "/v2/foo/tags/list?n=1",
			WantCode:    http.StatusOK,
			WantBody:    `{"name":"foo","tags":["latest"]}`,
		},
		{
			Description: "offset_tags",
			Manifests:   map[string]string{"foo/manifests/latest": "foo", "foo/manifests/tag1": "foo"},
			Method:      "GET",
			URL:         "/v2/foo/tags/list?last=latest",
			WantCode:    http.StatusOK,
			WantBody:    `{"name":"foo","tags":["tag1"]}`,
		},
		{
			Description: "list_non_existing_tags",
			Method:      "GET",
			URL:         "/v2/foo/tags/list?n=1000",
			WantCode:    http.StatusNotFound,
		},
		{
			Description: "list_repos",
			Manifests:   map[string]string{"foo/manifests/latest": "foo", "bar/manifests/latest": "bar"},
			Method:      "GET",
			URL:         "/v2/_catalog?n=1000",
			WantCode:    http.StatusOK,
		},
		{
			Description: "fetch_references",
			Method:      "GET",
			URL:         "/v2/foo/referrers/" + digestOf("foo"),
			WantCode:    http.StatusOK,
			Manifests: map[string]string{
				"foo/manifests/image":           "foo",
				"foo/manifests/points-to-image": "{\"subject\": {\"digest\": \"" + digestOf("foo") + "\"}}",
			},
		},
		{
			Description: "fetch_references,_subject_pointing_elsewhere",
			Method:      "GET",
			URL:         "/v2/foo/referrers/" + digestOf("foo"),
			WantCode:    http.StatusOK,
			Manifests: map[string]string{
				"foo/manifests/image":           "foo",
				"foo/manifests/points-to-image": "{\"subject\": {\"digest\": \"" + digestOf("nonexistant") + "\"}}",
			},
		},
		{
			Description: "fetch_references,_no_results",
			Method:      "GET",
			URL:         "/v2/foo/referrers/" + digestOf("foo"),
			WantCode:    http.StatusOK,
			Manifests: map[string]string{
				"foo/manifests/image": "foo",
			},
		},
		{
			Description: "fetch_references,_missing_repo",
			Method:      "GET",
			URL:         "/v2/does-not-exist/referrers/" + digestOf("foo"),
			WantCode:    http.StatusNotFound,
		},
		{
			Description: "fetch_references,_bad_target_(tag_vs._digest)",
			Method:      "GET",
			URL:         "/v2/foo/referrers/latest",
			WantCode:    http.StatusBadRequest,
		},
		{
			skip:        true,
			Description: "fetch_references,_bad_method",
			Method:      "POST",
			URL:         "/v2/foo/referrers/" + digestOf("foo"),
			WantCode:    http.StatusBadRequest,
		},
	}

	for _, tc := range tcs {

		testf := func(t *testing.T) {
			if tc.skip {
				t.Skip("skipping")
			}
			r := ociserver.New(ocimem.New(), nil)
			s := httptest.NewServer(r)
			defer s.Close()

			for manifest, contents := range tc.Manifests {
				req, _ := http.NewRequest("PUT", s.URL+"/v2/"+manifest, strings.NewReader(contents))
				req.Header.Set("Content-Type", "application/octet-stream") // TODO better media type
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
				req, _ := http.NewRequest(
					"POST",
					fmt.Sprintf("%s/v2/foo/blobs/uploads/?digest=%s", s.URL, digest),
					strings.NewReader(contents),
				)
				req.Header.Set("Content-Length", fmt.Sprint(len(contents))) // TODO better media type
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
				req, err := http.NewRequest(
					"PATCH",
					fmt.Sprintf("%s/v2/foo/blobs/uploads/%s", s.URL, upload),
					io.NopCloser(strings.NewReader(contents)),
				)
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Add("Content-Range", fmt.Sprintf("0-%d", len(contents)-1))
				t.Log(req.Method, req.URL)
				resp, err := s.Client().Do(req)
				if err != nil {
					t.Fatalf("Error streaming blob: %v", err)
				}
				if resp.StatusCode != http.StatusAccepted {
					body, _ := io.ReadAll(resp.Body)
					t.Fatalf("Error streaming blob: %d %s", resp.StatusCode, body)
				}

			}

			u, err := url.Parse(s.URL + tc.URL)
			if err != nil {
				t.Fatalf("Error parsing %q: %v", s.URL+tc.URL, err)
			}
			req := &http.Request{
				Method:        tc.Method,
				URL:           u,
				Body:          io.NopCloser(strings.NewReader(tc.Body)),
				ContentLength: int64(len(tc.Body)),
				Header:        map[string][]string{},
			}
			for k, v := range tc.RequestHeader {
				req.Header.Set(k, v)
			}
			t.Logf("%s %v", req.Method, req.URL)
			resp, err := s.Client().Do(req)
			if err != nil {
				t.Fatalf("Error getting %q: %v", tc.URL, err)
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Errorf("Reading response body: %v", err)
			}
			if resp.StatusCode != tc.WantCode {
				t.Fatalf("Incorrect status code, got %d, want %d; body: %s", resp.StatusCode, tc.WantCode, body)
			}

			for k, v := range tc.WantHeader {
				r := resp.Header.Get(k)
				if r != v {
					t.Errorf("Incorrect header %q received, got %q, want %q", k, r, v)
				}
			}

			if tc.WantBody != "" && string(body) != tc.WantBody {
				t.Errorf("Incorrect response body, got %q, want %q", body, tc.WantBody)
			}
		}
		t.Run(tc.Description, testf)
	}
}

func digestOf(s string) string {
	return string(digest.FromString(s))
}
