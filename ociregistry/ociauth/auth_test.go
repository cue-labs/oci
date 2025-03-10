package ociauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-quicktest/qt"
)

func TestBasicAuth(t *testing.T) {
	ts := newTargetServer(t, func(req *http.Request) *httpError {
		username, password, _ := req.BasicAuth()
		if username != "testuser" || password != "testpassword" {
			return &httpError{
				statusCode: http.StatusUnauthorized,
				header: http.Header{
					"Www-Authenticate": {"Basic"},
				},
			}
		}
		return nil
	})
	client := &http.Client{
		Transport: NewStdTransport(StdTransportParams{
			Config: configFunc(func(host string) (ConfigEntry, error) {
				if host != ts.Host {
					return ConfigEntry{}, nil
				}
				return ConfigEntry{
					Username: "testuser",
					Password: "testpassword",
				}, nil
			}),
		}),
	}
	assertRequest(context.Background(), t, ts, "/test", client, Scope{})
}

func TestBearerAuth(t *testing.T) {
	testScope := ParseScope("repository:foo:push,pull")
	authSrv := newAuthServer(t, func(req *http.Request) (any, *httpError) {
		username, password, ok := req.BasicAuth()
		if !ok || username != "testuser" || password != "testpassword" {
			return nil, &httpError{
				statusCode: http.StatusUnauthorized,
			}
		}
		requestedScope := ParseScope(req.Form.Get("scope"))
		if !runNonFatal(t, func(t testing.TB) {
			qt.Assert(t, qt.DeepEquals(requestedScope, testScope))
			qt.Assert(t, qt.DeepEquals(req.Form["service"], []string{"someService"}))
		}) {
			return nil, &httpError{
				statusCode: http.StatusInternalServerError,
			}
		}
		return &wireToken{
			Token: token{requestedScope}.String(),
		}, nil
	})
	ts := newTargetServer(t, func(req *http.Request) *httpError {
		if req.Header.Get("Authorization") == "" {
			return &httpError{
				statusCode: http.StatusUnauthorized,
				header: http.Header{
					"Www-Authenticate": []string{fmt.Sprintf("Bearer realm=%q,service=someService,scope=%q", authSrv, testScope)},
				},
			}
		}
		runNonFatal(t, func(t testing.TB) {
			qt.Assert(t, qt.DeepEquals(authScopeFromRequest(t, req), testScope))
		})
		return nil
	})
	client := &http.Client{
		Transport: NewStdTransport(StdTransportParams{
			Config: configFunc(func(host string) (ConfigEntry, error) {
				if host != ts.Host {
					return ConfigEntry{}, nil
				}
				return ConfigEntry{
					Username: "testuser",
					Password: "testpassword",
				}, nil
			}),
		}),
	}
	assertRequest(context.Background(), t, ts, "/test", client, Scope{})
}

func TestBearerAuthAdditionalScope(t *testing.T) {
	// This tests the scenario where there's a larger scope in the context
	// than the required scope.
	requiredScope := ParseScope("repository:foo:push,pull")
	additionalScope := ParseScope("repository:bar:pull somethingElse")
	authSrv := newAuthServer(t, func(req *http.Request) (any, *httpError) {
		username, password, ok := req.BasicAuth()
		if !ok || username != "testuser" || password != "testpassword" {
			return nil, &httpError{
				statusCode: http.StatusUnauthorized,
			}
		}
		requestedScope := ParseScope(strings.Join(req.Form["scope"], " "))
		if !runNonFatal(t, func(t testing.TB) {
			qt.Assert(t, qt.DeepEquals(requestedScope, requiredScope.Union(additionalScope)))
			qt.Assert(t, qt.DeepEquals(req.Form["service"], []string{"someService"}))
		}) {
		}
		return &wireToken{
			Token: token{requestedScope}.String(),
		}, nil
	})
	ts := newTargetServer(t, func(req *http.Request) *httpError {
		if req.Header.Get("Authorization") == "" {
			return &httpError{
				statusCode: http.StatusUnauthorized,
				header: http.Header{
					"Www-Authenticate": []string{fmt.Sprintf("Bearer realm=%q,service=someService,scope=%q", authSrv, requiredScope)},
				},
			}
		}
		runNonFatal(t, func(t testing.TB) {
			qt.Assert(t, qt.DeepEquals(authScopeFromRequest(t, req), requiredScope.Union(additionalScope)))
		})
		return nil
	})
	client := &http.Client{
		Transport: NewStdTransport(StdTransportParams{
			Config: configFunc(func(host string) (ConfigEntry, error) {
				if host != ts.Host {
					return ConfigEntry{}, nil
				}
				return ConfigEntry{
					Username: "testuser",
					Password: "testpassword",
				}, nil
			}),
		}),
	}
	ctx := ContextWithScope(context.Background(), additionalScope)
	assertRequest(ctx, t, ts, "/test", client, Scope{})
}

func TestBearerAuthRequiresExactScope(t *testing.T) {
	// This tests the scenario where an auth server requires exactly the
	// scope that was present in the challenge.
	requiredScope := ParseScope("repository:foo:pull,push")
	exactScope := "other repository:foo:push,pull"
	exactScopeAsToken := base64.StdEncoding.EncodeToString([]byte("token-" + exactScope))
	authSrv := newAuthServer(t, func(req *http.Request) (any, *httpError) {
		username, password, ok := req.BasicAuth()
		if !ok || username != "testuser" || password != "testpassword" {
			return nil, &httpError{
				statusCode: http.StatusUnauthorized,
			}
		}
		requestedScope := strings.Join(req.Form["scope"], " ")
		if requestedScope != exactScope {
			return nil, &httpError{
				statusCode: http.StatusUnauthorized,
			}
		}
		return &wireToken{
			Token: exactScopeAsToken,
		}, nil
	})
	ts := newTargetServer(t, func(req *http.Request) *httpError {
		if req.Header.Get("Authorization") == "" {
			return &httpError{
				statusCode: http.StatusUnauthorized,
				header: http.Header{
					"Www-Authenticate": []string{fmt.Sprintf("Bearer realm=%q,service=someService,scope=%q", authSrv, exactScope)},
				},
			}
		}
		qt.Check(t, qt.Equals(req.Header.Get("Authorization"), "Bearer "+exactScopeAsToken))
		return nil
	})
	client := &http.Client{
		Transport: NewStdTransport(StdTransportParams{
			Config: configFunc(func(host string) (ConfigEntry, error) {
				if host != ts.Host {
					return ConfigEntry{}, nil
				}
				return ConfigEntry{
					Username: "testuser",
					Password: "testpassword",
				}, nil
			}),
		}),
	}
	assertRequest(context.Background(), t, ts, "/test", client, requiredScope)
}

func TestAuthNotAvailableAfterChallenge(t *testing.T) {
	// This tests the scenario where the target server returns a challenge
	// that we can't meet.
	requestCount := 0
	ts := newTargetServer(t, func(req *http.Request) *httpError {
		if req.Header.Get("Authorization") == "" {
			requestCount++
			return &httpError{
				statusCode: http.StatusUnauthorized,
				header: http.Header{
					"Www-Authenticate": []string{"Basic service=someService"},
				},
			}
		}
		t.Errorf("authorization unexpectedly presented")
		return nil
	})
	client := &http.Client{
		Transport: NewStdTransport(StdTransportParams{
			Config: configFunc(func(host string) (ConfigEntry, error) {
				return ConfigEntry{}, nil
			}),
		}),
	}
	req, err := http.NewRequestWithContext(context.Background(), "GET", ts.String()+"/test", nil)
	qt.Assert(t, qt.IsNil(err))
	resp, err := client.Do(req)
	qt.Assert(t, qt.IsNil(err))
	defer resp.Body.Close()
	qt.Assert(t, qt.Equals(resp.StatusCode, http.StatusUnauthorized))
	qt.Check(t, qt.Equals(requestCount, 1))
}

func Test401ResponseWithJustAcquiredToken(t *testing.T) {
	// This tests the scenario where a server returns a 401 response
	// when the client has just successfully acquired a token from
	// the auth server.
	//
	// In this case, a "correct" server should return
	// either 403 (access to the resource is forbidden because the
	// client's credentials are not sufficient) or 404 (either the
	// repository really doesn't exist or the credentials are insufficient
	// and the server doesn't allow clients to see whether repositories
	// they don't have access to might exist).
	//
	// However, some real-world servers instead return a 401 response
	// erroneously indicating that the client needs to acquire
	// authorization credentials, even though they have in fact just
	// done so.
	//
	// As a workaround for this case, we treat the response as a 404.

	testScope := ParseScope("repository:foo:pull")
	authSrv := newAuthServer(t, func(req *http.Request) (any, *httpError) {
		requestedScope := ParseScope(req.Form.Get("scope"))
		if !runNonFatal(t, func(t testing.TB) {
			qt.Assert(t, qt.DeepEquals(requestedScope, testScope))
			qt.Assert(t, qt.DeepEquals(req.Form["service"], []string{"someService"}))
		}) {
			return nil, &httpError{
				statusCode: http.StatusInternalServerError,
			}
		}
		return &wireToken{
			Token: token{requestedScope}.String(),
		}, nil
	})
	ts := newTargetServer(t, func(req *http.Request) *httpError {
		if req.Header.Get("Authorization") == "" {
			return &httpError{
				statusCode: http.StatusUnauthorized,
				header: http.Header{
					"Www-Authenticate": []string{fmt.Sprintf("Bearer realm=%q,service=someService,scope=%q", authSrv, testScope)},
				},
			}
		}
		if !runNonFatal(t, func(t testing.TB) {
			qt.Assert(t, qt.DeepEquals(authScopeFromRequest(t, req), testScope))
		}) {
			return &httpError{
				statusCode: http.StatusInternalServerError,
			}
		}
		return &httpError{
			statusCode: http.StatusUnauthorized,
			header: http.Header{
				"Www-Authenticate": []string{fmt.Sprintf("Bearer realm=%q,service=someService,scope=%q", authSrv, testScope)},
			},
		}
	})
	client := &http.Client{
		Transport: NewStdTransport(StdTransportParams{
			Config: configFunc(func(host string) (ConfigEntry, error) {
				return ConfigEntry{}, nil
			}),
		}),
	}
	req, err := http.NewRequestWithContext(context.Background(), "GET", ts.String()+"/test", nil)
	qt.Assert(t, qt.IsNil(err))
	resp, err := client.Do(req)
	qt.Assert(t, qt.IsNil(err))
	defer resp.Body.Close()
	qt.Assert(t, qt.Equals(resp.StatusCode, http.StatusForbidden))
}

func Test401ResponseWithNonAcquiredToken(t *testing.T) {
	// This tests the scenario where a server returns a 401 response
	// when the client has provided credentials already present in
	// the configuration file.
	//
	// In this case, we don't want to trigger the fake-403-response
	// behaviour test for in Test401ResponseWithJustAcquiredToken.

	ts := newTargetServer(t, func(req *http.Request) *httpError {
		if req.Header.Get("Authorization") == "" {
			return &httpError{
				statusCode: http.StatusUnauthorized,
				header: http.Header{
					"Www-Authenticate": []string{"Basic"},
				},
				body: "no auth creds provided",
			}
		}
		return &httpError{
			statusCode: http.StatusUnauthorized,
			header: http.Header{
				"Www-Authenticate": []string{"Basic"},
			},
			body: "password mismatch",
		}
	})
	client := &http.Client{
		Transport: NewStdTransport(StdTransportParams{
			Config: configFunc(func(host string) (ConfigEntry, error) {
				return ConfigEntry{
					Username: "someuser",
					Password: "somepassword",
				}, nil
			}),
		}),
	}
	req, err := http.NewRequestWithContext(context.Background(), "GET", ts.String()+"/test", nil)
	qt.Assert(t, qt.IsNil(err))
	resp, err := client.Do(req)
	qt.Assert(t, qt.IsNil(err))
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	qt.Assert(t, qt.Equals(resp.StatusCode, http.StatusUnauthorized))
	qt.Assert(t, qt.Equals(string(data), "password mismatch"))
}

func TestConfigHasAccessToken(t *testing.T) {
	accessToken := "somevalue"
	ts := newTargetServer(t, func(req *http.Request) *httpError {
		if req.Header.Get("Authorization") == "" {
			t.Errorf("no authorization presented")
			return &httpError{
				statusCode: http.StatusUnauthorized,
			}
		}
		qt.Check(t, qt.Equals(req.Header.Get("Authorization"), "Bearer "+accessToken))
		return nil
	})
	client := &http.Client{
		Transport: NewStdTransport(StdTransportParams{
			Config: configFunc(func(host string) (ConfigEntry, error) {
				if host == ts.Host {
					return ConfigEntry{
						AccessToken: accessToken,
					}, nil
				}
				return ConfigEntry{}, nil
			}),
		}),
	}
	assertRequest(context.Background(), t, ts, "/test", client, Scope{})
}

func TestConfigErrorNilRequestBody(t *testing.T) {
	// stdTransport used to panic when given a nil request body
	// if something failed before it called the underlying transport,
	// as it would always try to close the body even when nil.
	ts := newTargetServer(t, func(req *http.Request) *httpError {
		return &httpError{statusCode: http.StatusUnauthorized}
	})
	client := &http.Client{
		Transport: NewStdTransport(StdTransportParams{
			Config: configFunc(func(host string) (ConfigEntry, error) {
				return ConfigEntry{}, fmt.Errorf("always fails")
			}),
		}),
	}
	_, err := client.Get(ts.String() + "/test")
	qt.Assert(t, qt.ErrorMatches(err, `.*cannot acquire auth.*always fails`))
}

func TestLaterRequestCanUseEarlierTokenWithLargerScope(t *testing.T) {
	authCount := 0
	authSrv := newAuthServer(t, func(req *http.Request) (any, *httpError) {
		authCount++
		return &wireToken{
			Token: token{ParseScope(strings.Join(req.Form["scope"], " "))}.String(),
		}, nil
	})
	ts := newTargetServer(t, func(req *http.Request) *httpError {
		resource := strings.TrimPrefix(req.URL.Path, "/test/")
		requiredScope := NewScope(ResourceScope{
			ResourceType: TypeRepository,
			Resource:     resource,
			Action:       ActionPull,
		})
		if req.Header.Get("Authorization") == "" {
			return &httpError{
				statusCode: http.StatusUnauthorized,
				header: http.Header{
					"Www-Authenticate": []string{fmt.Sprintf("Bearer realm=%q,service=someService,scope=%q", authSrv, requiredScope)},
				},
			}
		}
		runNonFatal(t, func(t testing.TB) {
			requestScope := authScopeFromRequest(t, req)
			qt.Assert(t, qt.IsTrue(requestScope.Contains(requiredScope)), qt.Commentf("request scope: %q; required scope: %q", requestScope, requiredScope))
		})
		return nil
	})
	client := &http.Client{
		Transport: NewStdTransport(StdTransportParams{
			Config: configFunc(func(host string) (ConfigEntry, error) {
				return ConfigEntry{}, nil
			}),
		}),
	}
	ctx := ContextWithScope(context.Background(), ParseScope("repository:foo1:pull repository:foo2:pull"))
	assertRequest(ctx, t, ts, "/test/foo1", client, Scope{})
	assertRequest(ctx, t, ts, "/test/foo2", client, Scope{})
	// One token fetch should have been sufficient for both requests.
	qt.Assert(t, qt.Equals(authCount, 1))
}

func TestAuthServerRejectsRequestsWithTooMuchScope(t *testing.T) {
	// This tests the scenario described in the comment in registry.acquireAccessToken.
	userHasScope := ParseScope("repository:foo:pull")

	authSrv := newAuthServer(t, func(req *http.Request) (any, *httpError) {
		requestedScope := ParseScope(strings.Join(req.Form["scope"], " "))
		if !userHasScope.Contains(requestedScope) {
			// Client is asking for more scope than the authenticated user
			// has access to. Technically this should be OK, but some
			// servers don't like it.
			return nil, &httpError{
				statusCode: http.StatusUnauthorized,
			}
		}
		return &wireToken{
			Token: token{requestedScope}.String(),
		}, nil
	})
	ts := newTargetServer(t, func(req *http.Request) *httpError {
		requiredScope := ParseScope("repository:foo:pull")
		if req.Header.Get("Authorization") == "" {
			return &httpError{
				statusCode: http.StatusUnauthorized,
				header: http.Header{
					"Www-Authenticate": []string{fmt.Sprintf("Bearer realm=%q,service=someService,scope=%q", authSrv, requiredScope)},
				},
			}
		}
		runNonFatal(t, func(t testing.TB) {
			qt.Assert(t, qt.IsTrue(authScopeFromRequest(t, req).Contains(requiredScope)))
		})
		return nil
	})
	client := &http.Client{
		Transport: NewStdTransport(StdTransportParams{
			Config: configFunc(func(host string) (ConfigEntry, error) {
				return ConfigEntry{}, nil
			}),
		}),
	}
	ctx := ContextWithScope(context.Background(), ParseScope("repository:foo:pull repository:bar:pull"))
	assertRequest(ctx, t, ts, "/test", client, Scope{})
}

func TestAuthRequestUsesRefreshTokenFromConfig(t *testing.T) {
	authCount := 0
	authSrv := newAuthServer(t, func(req *http.Request) (any, *httpError) {
		authCount++
		if !runNonFatal(t, func(t testing.TB) {
			qt.Assert(t, qt.Equals(req.Form.Get("grant_type"), "refresh_token"))
			qt.Assert(t, qt.Not(qt.Equals(req.Form.Get("client_id"), "")))
			qt.Assert(t, qt.Equals(req.Form.Get("service"), "someService"))
			qt.Assert(t, qt.Equals(req.Form.Get("refresh_token"), "someRefreshToken"))
		}) {
			return nil, &httpError{
				statusCode: http.StatusInternalServerError,
			}
		}
		requestedScope := ParseScope(strings.Join(req.Form["scope"], " "))
		// Return an access token that expires soon so that we can let it expire
		// so the client will be forced to acquire a new one with the original
		// refresh token.
		return &wireToken{
			Token:     token{requestedScope}.String(),
			ExpiresIn: 2, // Two seconds from now.
		}, nil
	})
	requiredScope := ParseScope("repository:foo:pull")
	ts := newTargetServer(t, func(req *http.Request) *httpError {
		if req.Header.Get("Authorization") == "" {
			return &httpError{
				statusCode: http.StatusUnauthorized,
				header: http.Header{
					"Www-Authenticate": []string{fmt.Sprintf("Bearer realm=%q,service=someService,scope=%q", authSrv, requiredScope)},
				},
			}
		}
		runNonFatal(t, func(t testing.TB) {
			qt.Assert(t, qt.IsTrue(authScopeFromRequest(t, req).Contains(requiredScope)))
		})
		return nil
	})
	client := &http.Client{
		Transport: NewStdTransport(StdTransportParams{
			Config: configFunc(func(host string) (ConfigEntry, error) {
				if host == ts.Host {
					return ConfigEntry{
						RefreshToken: "someRefreshToken",
					}, nil
				}
				return ConfigEntry{}, nil
			}),
		}),
	}
	assertRequest(context.Background(), t, ts, "/test", client, requiredScope)

	// Let the original access token expire and then make another request,
	// which should force the client to acquire another token using
	// the original refresh token.

	// Note: the expiry algorithm always leaves at least a second leeway.
	time.Sleep(1100 * time.Millisecond)
	assertRequest(context.Background(), t, ts, "/test", client, requiredScope)
	// Check that it actually has had to acquire two tokens.
	qt.Assert(t, qt.Equals(authCount, 2))
}

func TestAuthRequestUsesRefreshTokenFromAuthServer(t *testing.T) {
	authCount := 0
	authSrv := newAuthServer(t, func(req *http.Request) (any, *httpError) {
		authCount++
		if !runNonFatal(t, func(t testing.TB) {
			// The client should be using a different refresh token each time
			qt.Assert(t, qt.Equals(req.Form.Get("refresh_token"), fmt.Sprintf("someRefreshToken%d", authCount)))
		}) {
			return nil, &httpError{
				statusCode: http.StatusInternalServerError,
			}
		}
		requestedScope := ParseScope(strings.Join(req.Form["scope"], " "))
		// Return an access token that expires soon so that we can let it expire
		// so the client will be forced to acquire a new one with the original
		// refresh token.
		return &wireToken{
			RefreshToken: fmt.Sprintf("someRefreshToken%d", authCount+1),
			Token:        token{requestedScope}.String(),
		}, nil
	})
	ts := newTargetServer(t, func(req *http.Request) *httpError {
		resource := strings.TrimPrefix(req.URL.Path, "/test/")
		requiredScope := NewScope(ResourceScope{
			ResourceType: TypeRepository,
			Resource:     resource,
			Action:       ActionPull,
		})
		if req.Header.Get("Authorization") == "" {
			return &httpError{
				statusCode: http.StatusUnauthorized,
				header: http.Header{
					"Www-Authenticate": []string{fmt.Sprintf("Bearer realm=%q,service=someService,scope=%q", authSrv, requiredScope)},
				},
			}
		}
		runNonFatal(t, func(t testing.TB) {
			requestScope := authScopeFromRequest(t, req)
			qt.Assert(t, qt.IsTrue(requestScope.Contains(requiredScope)), qt.Commentf("request scope: %q; required scope: %q", requestScope, requiredScope))
		})
		return nil
	})
	client := &http.Client{
		Transport: NewStdTransport(StdTransportParams{
			Config: configFunc(func(host string) (ConfigEntry, error) {
				if host == ts.Host {
					return ConfigEntry{
						RefreshToken: "someRefreshToken1",
					}, nil
				}
				return ConfigEntry{}, nil
			}),
		}),
	}
	// Each time we make a new request, we'll be asking for a new scope
	// because we're getting a new resource each time, so that will
	// make another request to the auth server, which will return
	// a new refresh token each time.
	numRequests := 4
	for i := range numRequests {
		repo := fmt.Sprintf("foo%d", i)
		assertRequest(context.Background(), t, ts, fmt.Sprintf("/test/foo%d", i), client, NewScope(ResourceScope{
			ResourceType: TypeRepository,
			Resource:     repo,
			Action:       ActionPull,
		}))
	}
	qt.Assert(t, qt.Equals(authCount, numRequests))
}

func assertRequest(ctx context.Context, t testing.TB, tsURL *url.URL, path string, client *http.Client, needScope Scope) {
	ctx = ContextWithRequestInfo(ctx, RequestInfo{
		RequiredScope: needScope,
	})
	// Try the request twice as the second time often exercises other
	// code paths as caches are warmed up.
	assertRequest1(ctx, t, tsURL, path, client)
	assertRequest1(ctx, t, tsURL, path, client)
}

func assertRequest1(ctx context.Context, t testing.TB, tsURL *url.URL, path string, client *http.Client) {
	req, err := http.NewRequestWithContext(ctx, "POST", tsURL.String()+path, strings.NewReader("test body"))
	qt.Assert(t, qt.IsNil(err))
	// Set ContentLength to -1 to prevent net/http from calling GetBody automatically,
	// thus testing the GetBody-calling code inside registry.doRequest.
	req.ContentLength = -1
	resp, err := client.Do(req)
	qt.Assert(t, qt.IsNil(err))
	defer resp.Body.Close()
	qt.Assert(t, qt.Equals(resp.StatusCode, http.StatusOK))
	data, _ := io.ReadAll(resp.Body)
	qt.Assert(t, qt.Equals(string(data), "test ok"))
}

// newAuthServer returns the URL for an auth server that uses auth to service authorization
// requests. If that returns a nil *httpError, the first return parameter is marshaled
// as a JSON response body; otherwise the error is returned.
func newAuthServer(t *testing.T, auth func(req *http.Request) (any, *httpError)) *url.URL {
	authSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		t.Logf("-> authSrv %s %v {", req.Method, req.URL)
		req.ParseForm()
		bodyJSON, herr := auth(req)
		if herr != nil {
			herr.send(w)
			t.Logf("} <- error %#v", herr)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		data, err := json.Marshal(bodyJSON)
		if err != nil {
			panic(err)
		}
		w.Write(data)
		t.Logf("} <- json %s", data)
	}))
	t.Cleanup(authSrv.Close)
	return mustParseURL(authSrv.URL)
}

// newTargetServer returns the URL for a test target server that uses the targetGate
// parameter to gate requests to the /test endpoint: if targetGate returns nil for a request
// to that endpoint, the request will succeed.
//
// It also returns the URL for an auth server that uses auth to service authorization
// requests. If that returns a nil *httpError, the first return parameter is marshaled
// as a JSON response body; otherwise the error is returned.
func newTargetServer(
	t *testing.T,
	targetGate func(req *http.Request) *httpError,
) *url.URL {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		t.Logf("-> targetSrv %s %v auth=%q {", req.Method, req.URL, req.Header.Get("Authorization"))
		herr := targetGate(req)
		if herr != nil {
			herr.send(w)
			t.Logf("} <- error %#v", herr)
			return
		}
		if req.URL.Path != "/test" && !strings.HasPrefix(req.URL.Path, "/test/") {
			t.Logf("} <- error (wrong path)")
			http.Error(w, "only /test is allowed", http.StatusNotFound)
			return
		}
		if req.Method != "POST" {
			t.Logf("} <- error (wrong method)")
			http.Error(w, "only method POST is allowed", http.StatusMethodNotAllowed)
			return
		}
		data, _ := io.ReadAll(req.Body)
		if gotBody := string(data); gotBody != "test body" {
			t.Logf("} <- error (wrong body %q)", gotBody)
			http.Error(w, "wrong body", http.StatusForbidden)
			return
		}
		t.Logf("} <- OK")
		w.Write([]byte("test ok"))
	}))
	t.Cleanup(srv.Close)
	return mustParseURL(srv.URL)
}

func mustParseURL(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}

type httpError struct {
	header     http.Header
	statusCode int
	body       string
}

func (e *httpError) send(w http.ResponseWriter) {
	maps.Copy(w.Header(), e.header)
	w.WriteHeader(e.statusCode)
	w.Write([]byte(e.body))
}

type configFunc func(host string) (ConfigEntry, error)

func (f configFunc) EntryForRegistry(host string) (ConfigEntry, error) {
	return f(host)
}

type token struct {
	scope Scope
}

func authScopeFromRequest(t testing.TB, req *http.Request) Scope {
	h, ok := req.Header["Authorization"]
	if !ok {
		t.Fatal("no Authorization found in request")
	}
	if len(h) != 1 {
		t.Fatal("multiple Authorization headers found in request")
	}
	tokStr, ok := strings.CutPrefix(h[0], "Bearer ")
	if !ok {
		t.Fatalf("token %q is not bearer token", h)
	}
	tok, err := parseToken(tokStr)
	qt.Assert(t, qt.IsNil(err))
	return tok.scope
}

func parseToken(s string) (token, error) {
	data, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return token{}, fmt.Errorf("invalid token %q: %v", s, err)
	}
	scope, ok := strings.CutPrefix(string(data), "token-")
	if !ok {
		return token{}, fmt.Errorf("invalid token prefix")
	}
	return token{
		scope: ParseScope(scope),
	}, nil
}

func (tok token) String() string {
	return base64.StdEncoding.EncodeToString([]byte("token-" + tok.scope.String()))
}

// runNonFatal runs the given function within t
// but will not call Fatal on t even if Fatal is called
// on the t passed to f. It reports whether all
// checks succeeded.
//
// This makes it suitable for passing to assertion-based
// functions inside goroutines where it's not ok to
// call Fatal.
func runNonFatal(t *testing.T, f func(t testing.TB)) (ok bool) {
	defer func() {
		switch e := recover(); e {
		case errFailNow, errSkipNow:
			ok = false
		case nil:
		default:
			panic(e)
		}
	}()
	f(nonFatalT{t})
	return !t.Failed()
}

var (
	errFailNow = errors.New("failing now")
	errSkipNow = errors.New("skipping now")
)

type nonFatalT struct {
	*testing.T
}

func (t nonFatalT) FailNow() {
	t.Helper()
	t.Fail()
	panic(errFailNow)
}

func (t nonFatalT) Fatal(args ...any) {
	t.Helper()
	t.Error(args...)
	t.FailNow()
}

func (t nonFatalT) Fatalf(format string, args ...any) {
	t.Helper()
	t.Errorf(format, args...)
	t.FailNow()
}

func (t nonFatalT) Skip(args ...any) {
	t.Helper()
	t.Log(args...)
	t.SkipNow()
}

func (t nonFatalT) SkipNow() {
	panic(errSkipNow)
}

func (t nonFatalT) Skipf(format string, args ...any) {
	t.Helper()
	t.Logf(format, args...)
	t.SkipNow()
}
