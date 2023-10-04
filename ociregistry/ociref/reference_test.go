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

package ociref

import (
	_ "crypto/sha256"
	_ "crypto/sha512"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/go-quicktest/qt"
)

var parseReferenceTests = []struct {
	testName string
	// input is the repository name or name component testcase
	input string
	// err is the error expected from Parse, or nil
	wantErr string
	wantRef Reference
}{
	{
		input: "test_com",
		wantRef: Reference{
			Repository: "test_com",
		},
	},
	{
		input: "test.com:tag",
		wantRef: Reference{
			Repository: "test.com",
			Tag:        "tag",
		},
	},
	{
		input: "test.com:5000",
		wantRef: Reference{
			Repository: "test.com",
			Tag:        "5000",
		},
	},
	{
		input: "test.com/repo:tag",
		wantRef: Reference{
			Host:       "test.com",
			Repository: "repo",
			Tag:        "tag",
		},
	},
	{
		input: "test:5000/repo",
		wantRef: Reference{
			Host:       "test:5000",
			Repository: "repo",
		},
	},
	{
		input: "test:5000/repo:tag",
		wantRef: Reference{
			Host:       "test:5000",
			Repository: "repo",
			Tag:        "tag",
		},
	},
	{
		input: "test:5000/repo@sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		wantRef: Reference{
			Host:       "test:5000",
			Repository: "repo",
			Digest:     "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		},
	},
	{
		input: "test:5000/repo:tag@sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		wantRef: Reference{
			Host:       "test:5000",
			Repository: "repo",
			Tag:        "tag",
			Digest:     "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		},
	},
	{
		input: "test:5000/repo",
		wantRef: Reference{
			Host:       "test:5000",
			Repository: "repo",
		},
	},
	{
		testName: "EmptyString",
		input:    "",
		wantErr:  `invalid reference syntax`,
	},
	{
		input:   ":justtag",
		wantErr: `invalid reference syntax`,
	},
	{
		input:   "@sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		wantErr: `invalid reference syntax`,
	},
	{
		input:   "repo@sha256:ffffffffffffffffffffffffffffffffff",
		wantErr: `invalid digest "sha256:ffffffffffffffffffffffffffffffffff": invalid checksum digest length`,
	},
	{
		input:   "validname@invalidDigest:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		wantErr: `invalid digest "invalidDigest:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff": invalid checksum digest format`,
	},
	{
		input:   "Uppercase:tag",
		wantErr: `invalid reference syntax`,
	},
	// FIXME "Uppercase" is incorrectly handled as a domain-name here, therefore passes.
	// See https://github.com/distribution/distribution/pull/1778, and https://github.com/docker/docker/pull/20175
	// {
	//	input: "Uppercase/lowercase:tag",
	//	err:   ErrNameContainsUppercase,
	// },
	{
		input:   "test:5000/Uppercase/lowercase:tag",
		wantErr: `tag "5000/Uppercase/lowercase:tag" contains invalid invalid character '/'`,
	},
	{
		input: "lowercase:Uppercase",
		wantRef: Reference{
			Repository: "lowercase",
			Tag:        "Uppercase",
		},
	},
	{
		testName: "RepoTooLong",
		input:    strings.Repeat("a/", 128) + "a:tag",
		wantErr:  `repository name too long`,
	},
	{
		testName: "RepoAlmostTooLong",
		input:    strings.Repeat("a/", 127) + "a:tag-puts-this-over-max",
		wantRef: Reference{
			// Note: docker/reference parses Host as "a".
			Repository: strings.Repeat("a/", 127) + "a",
			Tag:        "tag-puts-this-over-max",
		},
	},
	{
		input:   "aa/asdf$$^/aa",
		wantErr: `invalid reference syntax`,
	},
	{
		input: "sub-dom1.foo.com/bar/baz/quux",
		wantRef: Reference{
			Host:       "sub-dom1.foo.com",
			Repository: "bar/baz/quux",
		},
	},
	{
		input: "sub-dom1.foo.com/bar/baz/quux:some-long-tag",
		wantRef: Reference{
			Host:       "sub-dom1.foo.com",
			Repository: "bar/baz/quux",
			Tag:        "some-long-tag",
		},
	},
	{
		input: "b.gcr.io/test.example.com/my-app:test.example.com",
		wantRef: Reference{
			Host:       "b.gcr.io",
			Repository: "test.example.com/my-app",
			Tag:        "test.example.com",
		},
	},
	{
		input: "xn--n3h.com/myimage:xn--n3h.com", // ☃.com in punycode
		wantRef: Reference{
			Host:       "xn--n3h.com",
			Repository: "myimage",
			Tag:        "xn--n3h.com",
		},
	},
	{
		input: "xn--7o8h.com/myimage:xn--7o8h.com@sha512:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", // 🐳.com in punycode
		wantRef: Reference{
			Host:       "xn--7o8h.com",
			Repository: "myimage",
			Tag:        "xn--7o8h.com",
			Digest:     "sha512:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		},
	},
	{
		input: "foo_bar.com:8080",
		wantRef: Reference{
			Repository: "foo_bar.com",
			Tag:        "8080",
		},
	},
	{
		input: "foo.com/bar:8080",
		wantRef: Reference{
			Host:       "foo.com",
			Repository: "bar",
			Tag:        "8080",
		},
	},
	{
		input: "foo/foo_bar.com:8080",
		wantRef: Reference{
			Repository: "foo/foo_bar.com",
			Tag:        "8080",
		},
	},
	{
		input: "192.168.1.1",
		wantRef: Reference{
			Repository: "192.168.1.1",
		},
	},
	{
		input: "192.168.1.1:tag",
		wantRef: Reference{
			Repository: "192.168.1.1",
			Tag:        "tag",
		},
	},
	{
		input: "192.168.1.1:5000",
		wantRef: Reference{
			Repository: "192.168.1.1",
			Tag:        "5000",
		},
	},
	{
		input: "192.168.1.1/repo",
		wantRef: Reference{
			Host:       "192.168.1.1",
			Repository: "repo",
		},
	},
	{
		input: "192.168.1.1:5000/repo",
		wantRef: Reference{
			Host:       "192.168.1.1:5000",
			Repository: "repo",
		},
	},
	{
		input: "192.168.1.1:5000/repo:5050",
		wantRef: Reference{
			Host:       "192.168.1.1:5000",
			Repository: "repo",
			Tag:        "5050",
		},
	},
	{
		input:   "[2001:db8::1]",
		wantErr: `invalid reference syntax`,
	},
	{
		input:   "[2001:db8::1]:5000",
		wantErr: `invalid reference syntax`,
	},
	{
		input:   "[2001:db8::1]:tag",
		wantErr: `invalid reference syntax`,
	},
	{
		input: "[2001:db8::1]/repo",
		wantRef: Reference{
			Host:       "[2001:db8::1]",
			Repository: "repo",
		},
	},
	{
		input: "[2001:db8:1:2:3:4:5:6]/repo:tag",
		wantRef: Reference{
			Host:       "[2001:db8:1:2:3:4:5:6]",
			Repository: "repo",
			Tag:        "tag",
		},
	},
	{
		input: "[2001:db8::1]:5000/repo",
		wantRef: Reference{
			Host:       "[2001:db8::1]:5000",
			Repository: "repo",
		},
	},
	{
		input: "[2001:db8::1]:5000/repo:tag",
		wantRef: Reference{
			Host:       "[2001:db8::1]:5000",
			Repository: "repo",
			Tag:        "tag",
		},
	},
	{
		input: "[2001:db8::1]:5000/repo@sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		wantRef: Reference{
			Host:       "[2001:db8::1]:5000",
			Repository: "repo",
			Digest:     "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		},
	},
	{
		input: "[2001:db8::1]:5000/repo:tag@sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		wantRef: Reference{
			Host:       "[2001:db8::1]:5000",
			Repository: "repo",
			Tag:        "tag",
			Digest:     "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		},
	},
	{
		input: "[2001:db8::]:5000/repo",
		wantRef: Reference{
			Host:       "[2001:db8::]:5000",
			Repository: "repo",
		},
	},
	{
		input: "[::1]:5000/repo",
		wantRef: Reference{
			Host:       "[::1]:5000",
			Repository: "repo",
		},
	},
	{
		input:   "[fe80::1%eth0]:5000/repo",
		wantErr: `invalid reference syntax`,
	},
	{
		input:   "[fe80::1%@invalidzone]:5000/repo",
		wantErr: `invalid reference syntax`,
	},
}

func TestParseReference(t *testing.T) {
	for _, test := range parseReferenceTests {
		if test.testName == "" {
			test.testName = test.input
		}
		t.Run(test.testName, func(t *testing.T) {
			ref, err := ParseRelative(test.input)
			t.Logf("ref: %#v", ref)
			if test.wantErr != "" {
				if test.wantErr == "invalid reference syntax" {
					test.wantErr += regexp.QuoteMeta(fmt.Sprintf(" (%q)", test.input))
				}
				qt.Assert(t, qt.ErrorMatches(err, test.wantErr))
				return
			}
			qt.Assert(t, qt.IsNil(err))
			qt.Check(t, qt.Equals(ref, test.wantRef))
			qt.Check(t, qt.Equals(ref.String(), test.input))
			if test.wantRef.Host != "" {
				ref1, err := Parse(test.input)
				qt.Assert(t, qt.IsNil(err))
				qt.Check(t, qt.Equals(ref1, test.wantRef))
			} else {
				_, err := Parse(test.input)
				qt.Assert(t, qt.ErrorMatches(err, `reference does not contain host name`))
			}
		})
	}
}

var isValidHostTests = []struct {
	host string
	want bool
}{{
	host: "foo.com:5000",
	want: true,
}, {
	host: "foo.com",
	want: true,
}, {
	host: "localhost:1234",
	want: true,
}, {
	host: "localhost",
	want: false,
}, {
	host: "foo",
	want: false,
}, {
	host: "foo..com",
	want: false,
}, {
	host: "[::1]",
	want: true,
}, {
	host: "[::1]:3456",
	want: true,
}}

func TestIsValidHost(t *testing.T) {
	for _, test := range isValidHostTests {
		t.Run(test.host, func(t *testing.T) {
			qt.Assert(t, qt.Equals(IsValidHost(test.host), test.want))
		})
	}
}
