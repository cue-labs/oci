// Copyright 2024 CUE Labs AG
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

package ocifilter

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/go-quicktest/qt"
	"github.com/opencontainers/go-digest"

	"cuelabs.dev/go/oci/ociregistry"
	"cuelabs.dev/go/oci/ociregistry/ocimem"
)

func TestAccessCheckerErrorReturn(t *testing.T) {
	ctx := context.Background()
	testErr := errors.New("some error")
	r1 := AccessChecker(ocimem.New(), func(repoName string, access AccessKind) error {
		qt.Check(t, qt.Equals(repoName, "foo/bar"))
		qt.Check(t, qt.Equals(access, AccessRead))
		return testErr
	})
	_, err := r1.GetTag(ctx, "foo/bar", "t1")
	qt.Assert(t, qt.ErrorIs(err, testErr))
}

func TestAccessCheckerAccessRequest(t *testing.T) {
	assertAccess := func(wantAccess []accessCheck, do func(ctx context.Context, r ociregistry.Interface) error) {
		testErr := errors.New("some error")
		var gotAccess []accessCheck
		r := AccessChecker(&ociregistry.Funcs{
			NewError: func(ctx context.Context, methodName, repo string) error {
				return testErr
			},
		}, func(repoName string, access AccessKind) error {
			gotAccess = append(gotAccess, accessCheck{repoName, access})
			return nil
		})
		err := do(context.Background(), r)
		qt.Check(t, qt.ErrorIs(err, testErr))
		qt.Check(t, qt.DeepEquals(gotAccess, wantAccess))
	}
	assertAccess([]accessCheck{
		{"foo/read", AccessRead},
	}, func(ctx context.Context, r ociregistry.Interface) error {
		_, err := r.GetBlob(ctx, "foo/read", "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
		return err
	})
	assertAccess([]accessCheck{
		{"foo/read", AccessRead},
	}, func(ctx context.Context, r ociregistry.Interface) error {
		rd, err := r.GetBlobRange(ctx, "foo/read", "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", 100, 200)
		if rd != nil {
			rd.Close()
		}
		return err
	})

	assertAccess([]accessCheck{
		{"foo/read", AccessRead},
	}, func(ctx context.Context, r ociregistry.Interface) error {
		rd, err := r.GetManifest(ctx, "foo/read", "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
		if rd != nil {
			rd.Close()
		}
		return err
	})

	assertAccess([]accessCheck{
		{"foo/read", AccessRead},
	}, func(ctx context.Context, r ociregistry.Interface) error {
		rd, err := r.GetTag(ctx, "foo/read", "sometag")
		if rd != nil {
			rd.Close()
		}
		return err
	})

	assertAccess([]accessCheck{
		{"foo/read", AccessRead},
	}, func(ctx context.Context, r ociregistry.Interface) error {
		_, err := r.ResolveBlob(ctx, "foo/read", "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
		return err
	})

	assertAccess([]accessCheck{
		{"foo/read", AccessRead},
	}, func(ctx context.Context, r ociregistry.Interface) error {
		_, err := r.ResolveManifest(ctx, "foo/read", "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
		return err
	})

	assertAccess([]accessCheck{
		{"foo/read", AccessRead},
	}, func(ctx context.Context, r ociregistry.Interface) error {
		_, err := r.ResolveTag(ctx, "foo/read", "sometag")
		return err
	})

	assertAccess([]accessCheck{
		{"foo/write", AccessWrite},
	}, func(ctx context.Context, r ociregistry.Interface) error {
		_, err := r.PushBlob(ctx, "foo/write", ociregistry.Descriptor{
			MediaType: "application/json",
			Digest:    "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			Size:      3,
		}, strings.NewReader("foo"))
		return err
	})

	assertAccess([]accessCheck{
		{"foo/write", AccessWrite},
	}, func(ctx context.Context, r ociregistry.Interface) error {
		w, err := r.PushBlobChunked(ctx, "foo/write", 0)
		if err != nil {
			return err
		}
		w.Close()
		return nil
	})

	assertAccess([]accessCheck{
		{"foo/write", AccessWrite},
	}, func(ctx context.Context, r ociregistry.Interface) error {
		w, err := r.PushBlobChunkedResume(ctx, "foo/write", "/someid", 3, 0)
		if err != nil {
			return err
		}
		data := []byte("some data")
		if _, err := w.Write(data); err != nil {
			return err
		}
		_, err = w.Commit(digest.FromBytes(data))
		return err
	})

	assertAccess([]accessCheck{
		{"foo/read", AccessRead},
		{"foo/write", AccessWrite},
	}, func(ctx context.Context, r ociregistry.Interface) error {
		_, err := r.MountBlob(ctx, "foo/read", "foo/write", "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
		return err
	})

	assertAccess([]accessCheck{
		{"foo/write", AccessWrite},
	}, func(ctx context.Context, r ociregistry.Interface) error {
		_, err := r.PushManifest(ctx, "foo/write", "sometag", []byte("something"), "application/json")
		return err
	})

	assertAccess([]accessCheck{
		{"foo/write", AccessDelete},
	}, func(ctx context.Context, r ociregistry.Interface) error {
		return r.DeleteBlob(ctx, "foo/write", "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
	})

	assertAccess([]accessCheck{
		{"foo/write", AccessDelete},
	}, func(ctx context.Context, r ociregistry.Interface) error {
		return r.DeleteManifest(ctx, "foo/write", "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
	})

	assertAccess([]accessCheck{
		{"foo/write", AccessDelete},
	}, func(ctx context.Context, r ociregistry.Interface) error {
		return r.DeleteTag(ctx, "foo/write", "sometag")
	})

	assertAccess([]accessCheck{
		{"*", AccessList},
	}, func(ctx context.Context, r ociregistry.Interface) error {
		_, err := ociregistry.All(r.Repositories(ctx, ""))
		return err
	})

	assertAccess([]accessCheck{
		{"foo/read", AccessList},
	}, func(ctx context.Context, r ociregistry.Interface) error {
		_, err := ociregistry.All(r.Tags(ctx, "foo/read", ""))
		return err
	})

	assertAccess([]accessCheck{
		{"foo/read", AccessList},
	}, func(ctx context.Context, r ociregistry.Interface) error {
		_, err := ociregistry.All(r.Referrers(ctx, "foo/read", "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", ""))
		return err
	})
}

type accessCheck struct {
	Repo  string
	Check AccessKind
}
