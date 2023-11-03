package ociauth

import (
	"testing"

	"github.com/go-quicktest/qt"
)

var parseScopeTests = []struct {
	testName        string
	in              string
	canonicalString string
	wantScopes      []ResourceScope
}{{
	testName:        "SingleRepository",
	in:              "repository:foo/bar/baz:pull",
	canonicalString: "repository:foo/bar/baz:pull",
	wantScopes: []ResourceScope{{
		ResourceType: "repository",
		Resource:     "foo/bar/baz",
		Action:       "pull",
	}},
}, {
	testName:        "SingleRepositoryMultipleAction",
	in:              "repository:foo/bar/baz:push,pull",
	canonicalString: "repository:foo/bar/baz:pull,push",
	wantScopes: []ResourceScope{{
		ResourceType: "repository",
		Resource:     "foo/bar/baz",
		Action:       "pull",
	}, {
		ResourceType: "repository",
		Resource:     "foo/bar/baz",
		Action:       "push",
	}},
}, {
	testName:        "MultipleRepositories",
	in:              "repository:foo/bar/baz:push,pull repository:other:pull",
	canonicalString: "repository:foo/bar/baz:pull,push repository:other:pull",
	wantScopes: []ResourceScope{{
		ResourceType: "repository",
		Resource:     "foo/bar/baz",
		Action:       "pull",
	}, {
		ResourceType: "repository",
		Resource:     "foo/bar/baz",
		Action:       "push",
	}, {
		ResourceType: "repository",
		Resource:     "other",
		Action:       "pull",
	}},
}, {
	testName:        "MultipleRepositoriesWithCatalog",
	in:              "repository:foo/bar/baz:push,pull registry:catalog:* repository:other:pull",
	canonicalString: "registry:catalog:* repository:foo/bar/baz:pull,push repository:other:pull",
	wantScopes: []ResourceScope{CatalogScope, {
		ResourceType: "repository",
		Resource:     "foo/bar/baz",
		Action:       "pull",
	}, {
		ResourceType: "repository",
		Resource:     "foo/bar/baz",
		Action:       "push",
	}, {
		ResourceType: "repository",
		Resource:     "other",
		Action:       "pull",
	}},
}, {
	testName:        "UnknownScope",
	in:              "otherScope",
	canonicalString: "otherScope",
	wantScopes: []ResourceScope{{
		ResourceType: "otherScope",
	}},
}, {
	testName:        "UnknownAction",
	in:              "repository:foo/bar/baz:delete,push,pull",
	canonicalString: "repository:foo/bar/baz:delete,pull,push",
	wantScopes: []ResourceScope{{
		ResourceType: "repository",
		Resource:     "foo/bar/baz",
		Action:       "delete",
	}, {
		ResourceType: "repository",
		Resource:     "foo/bar/baz",
		Action:       "pull",
	}, {
		ResourceType: "repository",
		Resource:     "foo/bar/baz",
		Action:       "push",
	}},
}, {
	testName:        "SeveralUnknown",
	in:              "repository:foo/bar/baz:delete,pull,push repository:other:pull otherScope",
	canonicalString: "otherScope repository:foo/bar/baz:delete,pull,push repository:other:pull",
	wantScopes: []ResourceScope{{
		ResourceType: "otherScope",
	}, {
		ResourceType: "repository",
		Resource:     "foo/bar/baz",
		Action:       "delete",
	}, {
		ResourceType: "repository",
		Resource:     "foo/bar/baz",
		Action:       "pull",
	}, {
		ResourceType: "repository",
		Resource:     "foo/bar/baz",
		Action:       "push",
	}, {
		ResourceType: "repository",
		Resource:     "other",
		Action:       "pull",
	}},
}, {
	testName:        "duplicates",
	in:              "repository:foo/bar/baz:delete,pull,push otherScope repository:foo/bar/baz:pull,push repository:other:pull otherScope",
	canonicalString: "otherScope repository:foo/bar/baz:delete,pull,push repository:other:pull",
	wantScopes: []ResourceScope{{
		ResourceType: "otherScope",
	}, {
		ResourceType: "repository",
		Resource:     "foo/bar/baz",
		Action:       "delete",
	}, {
		ResourceType: "repository",
		Resource:     "foo/bar/baz",
		Action:       "pull",
	}, {
		ResourceType: "repository",
		Resource:     "foo/bar/baz",
		Action:       "push",
	}, {
		ResourceType: "repository",
		Resource:     "other",
		Action:       "pull",
	}},
}}

func TestParseScope(t *testing.T) {
	for _, test := range parseScopeTests {
		t.Run(test.testName, func(t *testing.T) {
			scope := ParseScope(test.in)
			t.Logf("parsed scope: %#v", scope)
			qt.Check(t, qt.Equals(scope.Canonical().String(), test.canonicalString))
			qt.Check(t, qt.Equals(scope.String(), test.in))
			qt.Check(t, qt.DeepEquals(all(scope.Iter()), test.wantScopes))
			checkStrictOrder(t, scope.Iter(), ResourceScope.Compare)
			// Check that it does actually preserve identity on round-trip.
			scope1 := ParseScope(scope.String())
			qt.Check(t, qt.Equals(scope1.Equal(scope), true))
		})
	}
}

var scopeUnionTests = []struct {
	testName      string
	s1            string
	s2            string
	want          string
	wantUnlimited bool
}{{
	testName: "Empty",
	s1:       "",
	s2:       "",
	want:     "",
}, {
	testName: "EmptyAndSingle",
	s1:       "",
	s2:       "repository:foo:pull",
	want:     "repository:foo:pull",
}, {
	testName: "SingleAndEmpty",
	s1:       "repository:foo:pull",
	s2:       "",
	want:     "repository:foo:pull",
}, {
	testName:      "UnlimitedAndSomething",
	s1:            "*",
	s2:            "repository:foo:pull",
	want:          "*",
	wantUnlimited: true,
}, {
	testName:      "SomethingAndUnlimited",
	s1:            "repository:foo:pull",
	s2:            "*",
	want:          "*",
	wantUnlimited: true,
}, {
	testName:      "UnlimitedAndUnlimited",
	s1:            "*",
	s2:            "*",
	want:          "*",
	wantUnlimited: true,
}, {
	testName: "Multiple",
	s1:       "anotherScope:bad otherScope repository:arble:pull repository:foo:pull,push",
	s2:       "otherScope registry:catalog:* repository:foo:delete repository:bar/baz:pull yetAnotherScope",
	want:     "anotherScope:bad otherScope registry:catalog:* repository:arble:pull repository:bar/baz:pull repository:foo:delete,pull,push yetAnotherScope",
}, {
	testName: "Identical",
	s1:       "otherScope registry:catalog:* repository:bar/baz:pull repository:foo:delete yetAnotherScope",
	s2:       "otherScope registry:catalog:* repository:bar/baz:pull repository:foo:delete yetAnotherScope",
	want:     "otherScope registry:catalog:* repository:bar/baz:pull repository:foo:delete yetAnotherScope",
}, {
	testName: "Identical",
	s1:       "otherScope registry:catalog:* repository:bar/baz:pull repository:foo:delete yetAnotherScope",
	s2:       "otherScope registry:catalog:* repository:bar/baz:pull repository:foo:delete yetAnotherScope",
	want:     "otherScope registry:catalog:* repository:bar/baz:pull repository:foo:delete yetAnotherScope",
}, {
	testName: "StringPreservedWhenResultEqual",
	s1:       "repository:bar/baz:something,pull arble",
	s2:       "arble",
	want:     "repository:bar/baz:something,pull arble",
}}

func TestScopeUnion(t *testing.T) {
	for _, test := range scopeUnionTests {
		t.Run(test.testName, func(t *testing.T) {
			s1 := parseScopeMaybeUnlimited(test.s1)
			s2 := parseScopeMaybeUnlimited(test.s2)
			u1 := s1.Union(s2)
			qt.Check(t, qt.Equals(u1.String(), test.want))
			qt.Check(t, qt.Equals(u1.IsUnlimited(), test.wantUnlimited))

			// Check that it's commutative.
			u2 := s2.Union(s1)
			qt.Check(t, qt.Equals(u1.String(), test.want))
			qt.Check(t, qt.Equals(u1.IsUnlimited(), test.wantUnlimited))

			qt.Check(t, qt.IsTrue(u1.Equal(u2)))
		})
	}
}

var scopeHoldsTests = []struct {
	testName string
	s        string
	holds    ResourceScope
	want     bool
}{{
	testName: "Empty",
	s:        "",
	holds:    ResourceScope{"repository", "foo", "pull"},
	want:     false,
}, {
	testName: "RepoMemberPresent",
	s:        "otherScope registry:catalog:* repository:bar/baz:pull repository:foo:delete yetAnotherScope",
	holds:    ResourceScope{"repository", "bar/baz", "pull"},
	want:     true,
}, {
	testName: "RepoMemberNotPresent",
	s:        "otherScope registry:catalog:* repository:bar/baz:pull repository:foo:delete yetAnotherScope",
	holds:    ResourceScope{"repository", "bar/baz", "push"},
	want:     false,
}, {
	testName: "CatalogScopePresent",
	s:        "otherScope registry:catalog:* repository:bar/baz:pull repository:foo:delete yetAnotherScope",
	holds:    CatalogScope,
	want:     true,
}, {
	testName: "CatalogScopeNotPresent",
	s:        "otherScope repository:bar/baz:pull repository:foo:delete yetAnotherScope",
	holds:    CatalogScope,
	want:     false,
}, {
	testName: "OtherScopePresent",
	s:        "otherScope repository:bar/baz:pull repository:foo:delete yetAnotherScope",
	holds:    ResourceScope{"otherScope", "", ""},
	want:     true,
}, {
	testName: "OtherScopeNotPresent",
	s:        "otherScope repository:bar/baz:pull repository:foo:delete yetAnotherScope",
	holds:    ResourceScope{"notThere", "", ""},
	want:     false,
}, {
	testName: "Unlimited",
	s:        "*",
	holds:    ResourceScope{"repository", "bar/baz", "push"},
	want:     true,
}}

func TestScopeHolds(t *testing.T) {
	for _, test := range scopeHoldsTests {
		t.Run(test.testName, func(t *testing.T) {
			qt.Assert(t, qt.Equals(parseScopeMaybeUnlimited(test.s).Holds(test.holds), test.want))
		})
	}
}

var scopeContainsTests = []struct {
	testName string
	s1       string
	s2       string
	want     bool
}{{
	testName: "EmptyContainsEmpty",
	s1:       "",
	s2:       "",
	want:     true,
}, {
	testName: "SomethingContainsEmpty",
	s1:       "foo",
	s2:       "",
	want:     true,
}, {
	testName: "UnlimitedContainsSomething",
	s1:       "*",
	s2:       "foo",
	want:     true,
}, {
	testName: "SomethingDoesNotContainUnlimited",
	s1:       "foo",
	s2:       "*",
	want:     false,
}, {
	testName: "UnlimitedContainsUnlimited",
	s1:       "*",
	s2:       "*",
	want:     true,
}, {
	testName: "MultipleContainsMultiple",
	s1:       "otherScope registry:catalog:* repository:bar/baz:push,pull repository:foo:delete yetAnotherScope",
	s2:       "otherScope registry:catalog:* repository:bar/baz:pull",
	want:     true,
}, {
	testName: "MultipleDoesNotContainMultiple",
	s1:       "otherScope registry:catalog:* repository:bar/baz:push repository:foo:delete yetAnotherScope",
	s2:       "otherScope registry:catalog:* repository:bar/baz:pull",
	want:     false,
}, {
	testName: "RepositoryNotPresent",
	s1:       "otherScope registry:catalog:* repository:bar/baz:push repository:foo:delete yetAnotherScope",
	s2:       "repository:other:pull",
	want:     false,
}, {
	testName: "OtherNotPresent#1",
	s1:       "otherScope registry:catalog:* repository:bar/baz:push repository:foo:delete yetAnotherScope",
	s2:       "arble zaphod",
	want:     false,
}, {
	testName: "OtherNotPresent#2",
	s1:       "otherScope registry:catalog:* repository:bar/baz:push repository:foo:delete yetAnotherScope",
	s2:       "arble",
	want:     false,
}}

func TestScopeContains(t *testing.T) {
	for _, test := range scopeContainsTests {
		t.Run(test.testName, func(t *testing.T) {
			s1 := parseScopeMaybeUnlimited(test.s1)
			s2 := parseScopeMaybeUnlimited(test.s2)
			qt.Assert(t, qt.Equals(s1.Contains(s2), test.want))
			if s1.Equal(s2) {
				qt.Assert(t, qt.IsTrue(s2.Contains(s1)))
			} else if test.want {
				qt.Assert(t, qt.IsFalse(s2.Contains(s1)))
			}
		})
	}
}

var scopeLenTests = []struct {
	scope Scope
	want  int
}{{
	scope: ParseScope("repository:foo:pull,push repository:bar:pull,delete other registry:catalog:*"),
	want:  6,
}, {
	scope: NewScope(),
	want:  0,
}, {
	scope: ParseScope("repository:foo:pull,push repository:bar:pull,delete other").Union(
		ParseScope("repository:bar:pull repository:bar:push repository:baz:pull more"),
	),
	want: 8,
}, {
	scope: NewScope(CatalogScope),
	want:  1,
}}

func TestScopeLen(t *testing.T) {
	for _, test := range scopeLenTests {
		t.Run(test.scope.String(), func(t *testing.T) {
			qt.Assert(t, qt.Equals(test.scope.Len(), test.want), qt.Commentf("%v", test.scope))
		})
	}
}

func TestScopeLenOnUnlimitedScopePanics(t *testing.T) {
	qt.Assert(t, qt.PanicMatches(func() {
		UnlimitedScope().Len()
	}, "Len called on unlimited scope"))
}

func parseScopeMaybeUnlimited(s string) Scope {
	if s == "*" {
		return UnlimitedScope()
	}
	return ParseScope(s)
}

func checkStrictOrder[T any](t *testing.T, iter func(func(T) bool), cmp func(T, T) int) {
	hasPrev := false
	var prev T
	i := -1
	iter(func(x T) bool {
		i++
		if !hasPrev {
			prev = x
			hasPrev = true
			return true
		}
		if c := cmp(prev, x); c != -1 {
			t.Fatalf("unexpected ordering at index %d: %v >= %v", i, prev, x)
		}
		prev = x
		return true
	})
}

func all[T any](iter func(func(T) bool)) []T {
	xs := []T{}
	iter(func(x T) bool {
		xs = append(xs, x)
		return true
	})
	return xs
}

func first[T any](iter func(func(T) bool)) T {
	found := false
	var x T
	iter(func(x1 T) bool {
		x = x1
		found = true
		return false
	})
	if !found {
		panic("no items in iterator")
	}
	return x
}
