package ociauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"cuelabs.dev/go/oci/ociregistry/internal/exp/slices"
)

// TODO decide on a good value for this.
const oauthClientID = "cuelabs-ociauth"

// Authorizer defines a way to make authorized requests using the OCI
// authorization scope mechanism. See [StdAuthorizer] for an implementation
// that understands the usual OCI authorization mechanisms.
type Authorizer interface {
	// DoRequest acquires authorization and invokes the given
	// request. It may invoke the request more than once, and can
	// use [http.Request.GetBody] to reset the request body if it
	// gets consumed.
	//
	// It ensures that the authorization token used will have at least
	// the capability to execute operations in requiredScope; any scope
	// inside the context (see [ContextWithScope]) may also be taken
	// into account when acquiring new tokens.
	//
	// It's OK to call AuthorizeRequest concurrently.
	DoRequest(req *http.Request, requiredScope Scope) (*http.Response, error)
}

var ErrNoAuth = fmt.Errorf("no authorization token available to add to request")

// StdAuthorizer implements [Authorizer] using the flows implemented
// by the usual docker clients. Note that this is _not_ documented as
// part of any official OCI spec.
//
// See https://distribution.github.io/distribution/spec/auth/token/ for an overview.
type StdAuthorizer struct {
	config     Config
	httpClient HTTPDoer
	mu         sync.Mutex
	registries map[string]*registry
}

type StdAuthorizerParams struct {
	Config     Config
	HTTPClient HTTPDoer
}

func NewStdAuthorizer(p StdAuthorizerParams) *StdAuthorizer {
	if p.Config == nil {
		p.Config = emptyConfig{}
	}
	if p.HTTPClient == nil {
		p.HTTPClient = http.DefaultClient
	}
	return &StdAuthorizer{
		config:     p.Config,
		httpClient: p.HTTPClient,
		registries: make(map[string]*registry),
	}
}

var _ Authorizer = (*StdAuthorizer)(nil)

// TODO de-dupe this from ociclient.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// registry holds currently known auth information for a registry.
type registry struct {
	host       string
	authorizer *StdAuthorizer
	initOnce   sync.Once
	initErr    error

	// mu guards the fields that follow it.
	mu sync.Mutex

	// wwwAuthenticate holds the Www-Authenticate header from
	// the most recent 401 response. If there was a 401 response
	// that didn't hold such a header, this will still be non-nil
	// but hold a zero authHeader.
	wwwAuthenticate *authHeader

	accessTokens []*scopedToken
	refreshToken string
	basic        *userPass
}

type scopedToken struct {
	// scope holds the scope that the token is good for.
	scope Scope
	// token holds the actual access token.
	token string
	// expires holds when the token expires.
	expires time.Time
}

type userPass struct {
	username string
	password string
}

var forever = time.Date(99999, time.January, 1, 0, 0, 0, 0, time.UTC)

// AuthorizeRequest implements [Authorizer.DoRequest].
func (a *StdAuthorizer) DoRequest(req *http.Request, requiredScope Scope) (*http.Response, error) {
	a.mu.Lock()
	r := a.registries[req.URL.Host]
	if r == nil {
		r = &registry{
			host:       req.URL.Host,
			authorizer: a,
		}
		a.registries[r.host] = r
	}
	a.mu.Unlock()
	if err := r.init(); err != nil {
		return nil, err
	}

	wantScope := ScopeFromContext(req.Context())
	return r.doRequest(req.Context(), req, requiredScope, wantScope)
}

// doRequest performs the given request on the registry r.
func (r *registry) doRequest(ctx context.Context, req *http.Request, requiredScope, wantScope Scope) (*http.Response, error) {
	// TODO set up request body so that we can rewind it when retrying if necessary.
	if err := r.setAuthorization(ctx, req, requiredScope, wantScope); err != nil {
		return nil, err
	}
	resp, err := r.authorizer.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusUnauthorized {
		return resp, nil
	}
	challenge := challengeFromResponse(resp)
	if challenge == nil {
		return resp, nil
	}
	authAdded, err := r.setAuthorizationFromChallenge(ctx, req, challenge, requiredScope, wantScope)
	if err != nil {
		resp.Body.Close()
		return nil, err
	}
	if !authAdded {
		// Couldn't acquire any more authorization than we had initially.
		return resp, nil
	}
	resp.Body.Close()
	// TODO rewind request body if needed.
	return r.authorizer.httpClient.Do(req)
}

// setAuthorization sets up authorization on the given request using any
// auth information currently available.
func (r *registry) setAuthorization(ctx context.Context, req *http.Request, requiredScope, wantScope Scope) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Remove tokens that have expired or will expire soon so that
	// the caller doesn't start using a token only for it to expire while it's
	// making the request.
	r.deleteExpiredTokens(time.Now().UTC().Add(time.Second))

	if accessToken := r.accessTokenForScope(requiredScope); accessToken != nil {
		// We have a potentially valid access token. Use it.
		req.Header.Set("Authorization", "Bearer "+accessToken.token)
		return nil
	}
	if r.wwwAuthenticate == nil {
		// We haven't seen a 401 response yet. Avoid putting any
		// basic authorization in the request, because that can mean that
		// the server sends a 401 response without a Www-Authenticate
		// header.
		return nil
	}
	if r.refreshToken != "" && r.wwwAuthenticate.scheme == "bearer" {
		// We've got a refresh token that we can use to try to
		// acquire an access token and we've seen a Www-Authenticate response
		// that tells us how we can use it.

		// TODO we're holding the lock (r.mu) here, which is precluding
		// acquiring several tokens concurrently. We should relax the lock
		// to allow that.

		accessToken, err := r.acquireAccessToken(ctx, requiredScope, wantScope)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
		return nil
	}
	if r.wwwAuthenticate.scheme != "bearer" && r.basic != nil {
		req.SetBasicAuth(r.basic.username, r.basic.password)
		return nil
	}
	return nil
}

func (r *registry) setAuthorizationFromChallenge(ctx context.Context, req *http.Request, challenge *authHeader, requiredScope, wantScope Scope) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.wwwAuthenticate = challenge

	switch {
	case r.wwwAuthenticate.scheme == "bearer":
		scope := ParseScope(r.wwwAuthenticate.params["scope"])
		accessToken, err := r.acquireAccessToken(ctx, scope, wantScope.Union(requiredScope))
		if err != nil {
			return false, err
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
		return true, nil
	case r.basic != nil:
		req.SetBasicAuth(r.basic.username, r.basic.password)
		return true, nil
	}
	return false, nil
}

// init initializes the registry instance by acquiring auth information from
// the Config, if available. As this might be slow (invoking EntryForRegistry
// can end up invoking slow external commands), we ensure that it's only
// done once.
// TODO it's possible that this could take a very long time, during which
// the outer context is cancelled, but we'll ignore that. We probably shouldn't.
func (r *registry) init() error {
	inner := func() error {
		info, err := r.authorizer.config.EntryForRegistry(r.host)
		if err != nil {
			return fmt.Errorf("cannot acquire auth info for registry %q: %v", r.host, err)
		}
		r.refreshToken = info.RefreshToken
		if info.AccessToken != "" {
			r.accessTokens = append(r.accessTokens, &scopedToken{
				scope:   UnlimitedScope(),
				token:   info.AccessToken,
				expires: forever,
			})
		}
		if info.Username != "" && info.Password != "" {
			r.basic = &userPass{
				username: info.Username,
				password: info.Password,
			}
		}
		return nil
	}
	r.initOnce.Do(func() {
		r.initErr = inner()
	})
	return r.initErr
}

// acquireAccessToken tries to acquire an access token for authorizing a request.
// The requiredScopeStr parameter indicates the scope that's definitely
// required. This is a string because apparently some servers are picky
// about getting exactly the same scope in the auth request that was
// returned in the challenge. The wantScope parameter indicates
// what scope might be required in the future.
//
// This method assumes that there has been a previous 401 response with
// a Www-Authenticate: Bearer... header.
func (r *registry) acquireAccessToken(ctx context.Context, requiredScope, wantScope Scope) (string, error) {
	scope := requiredScope.Union(wantScope)
	tok, err := r.acquireToken(ctx, scope)
	if err != nil {
		var rerr *responseError
		if !errors.As(err, &rerr) || rerr.statusCode != http.StatusUnauthorized {
			return "", err
		}
		// The documentation says this:
		//
		//	If the client only has a subset of the requested
		// 	access it _must not be considered an error_ as it is
		//	not the responsibility of the token server to
		//	indicate authorization errors as part of this
		//	workflow.
		//
		// However it's apparently not uncommon for servers to reject
		// such requests anyway, so if we've got an unauthorized error
		// and wantScope goes beyond requiredScope, it may be because
		// the server is rejecting the request.
		scope = requiredScope
		tok, err = r.acquireToken(ctx, scope)
		if err != nil {
			return "", err
		}
		// TODO mark the registry as picky about tokens so we don't
		// attempt twice every time?
	}
	if tok.RefreshToken != "" {
		r.refreshToken = tok.RefreshToken
	}
	accessToken := tok.Token
	if accessToken == "" {
		accessToken = tok.AccessToken
	}
	if accessToken == "" {
		return "", fmt.Errorf("no access token found in auth server response")
	}
	var expires time.Time
	now := time.Now().UTC()
	if tok.ExpiresIn == 0 {
		expires = now.Add(60 * time.Second) // TODO link to where this is mentioned
	} else {
		expires = now.Add(time.Duration(tok.ExpiresIn) * time.Second)
	}
	r.accessTokens = append(r.accessTokens, &scopedToken{
		scope:   scope,
		token:   accessToken,
		expires: expires,
	})
	// TODO persist the access token to save round trips when doing
	// the authorization flow in a newly run executable.
	return accessToken, nil
}

func (r *registry) acquireToken(ctx context.Context, scope Scope) (*wireToken, error) {
	realm := r.wwwAuthenticate.params["realm"]
	if realm == "" {
		return nil, fmt.Errorf("malformed Www-Authenticate header (missing realm)")
	}
	if r.refreshToken != "" {
		v := url.Values{}
		v.Set("scope", scope.String())
		if service := r.wwwAuthenticate.params["service"]; service != "" {
			v.Set("service", service)
		}
		v.Set("client_id", oauthClientID)
		v.Set("grant_type", "refresh_token")
		v.Set("refresh_token", r.refreshToken)
		req, err := http.NewRequestWithContext(ctx, "POST", realm, strings.NewReader(v.Encode()))
		if err != nil {
			return nil, fmt.Errorf("cannot form HTTP request to %q: %v", realm, err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		tok, err := r.doTokenRequest(req)
		if err == nil {
			return tok, nil
		}
		var rerr *responseError
		if !errors.As(err, &rerr) || rerr.statusCode != http.StatusNotFound {
			return tok, err
		}
		// The request to the endpoint returned 404 from the POST request,
		// Note: Not all token servers implement oauth2, so fall
		// back to using a GET with basic auth.
		// See the Token documentation for the HTTP GET method supported by all token servers.
		// TODO where in that documentation is this documented?
	}
	u, err := url.Parse(realm)
	if err != nil {
		return nil, fmt.Errorf("malformed Www-Authenticate header (malformed realm %q): %v", realm, err)
	}
	v := u.Query()
	// TODO where is it documented that we should send multiple scope
	// attributes rather than a single space-separated attribute as
	// the POST method does?
	v["scope"] = strings.Split(scope.String(), " ")
	if service := r.wwwAuthenticate.params["service"]; service != "" {
		// TODO the containerregistry code sets this even if it's empty.
		// Is that better?
		v.Set("service", service)
	}
	u.RawQuery = v.Encode()
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	// TODO if there's an unlimited-scope access token, the original code
	// will use it as Bearer authorization at this point. If
	// that's valid, why are we even acquiring another token?
	if r.basic != nil {
		req.SetBasicAuth(r.basic.username, r.basic.password)
	}
	return r.doTokenRequest(req)
}

// wireToken describes the JSON encoding used in the response to a token
// acquisition method. The comments are taken from the [token docs]
// and made available here for ease of reference.
//
// [token docs]: https://distribution.github.io/distribution/spec/auth/token/#token-response-fields
type wireToken struct {
	// Token holds an opaque Bearer token that clients should supply
	// to subsequent requests in the Authorization header.
	// AccessToken is provided for compatibility with OAuth 2.0: it's equivalent to Token.
	// At least one of these fields must be specified, but both may also appear (for compatibility with older clients).
	// When both are specified, they should be equivalent; if they differ the client's choice is undefined.
	Token       string `json:"token"`
	AccessToken string `json:"access_token,omitempty"`

	// Refresh token optionally holds a token which can be used to
	// get additional access tokens for the same subject with different scopes.
	// This token should be kept secure by the client and only sent
	// to the authorization server which issues bearer tokens. This
	// field will only be set when `offline_token=true` is provided
	// in the request.
	RefreshToken string `json:"refresh_token"`

	// ExpiresIn holds the duration in seconds since the token was
	// issued that it will remain valid. When omitted, this defaults
	// to 60 seconds. For compatibility with older clients, a token
	// should never be returned with less than 60 seconds to live.
	ExpiresIn int `json:"expires_in"`
}

func (r *registry) doTokenRequest(req *http.Request) (*wireToken, error) {
	resp, err := r.authorizer.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errorFromResponse(resp)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("cannot read response body: %v", err)
	}
	var tok wireToken
	if err := json.Unmarshal(data, &tok); err != nil {
		return nil, fmt.Errorf("malformed JSON token in response: %v", err)
	}
	return &tok, nil
}

type responseError struct {
	statusCode int
	msg        string
}

func errorFromResponse(resp *http.Response) error {
	// TODO include body of response in error message.
	return &responseError{
		statusCode: resp.StatusCode,
	}
}

func (e *responseError) Error() string {
	return fmt.Sprintf("unexpected HTTP response %d", e.statusCode)
}

// deleteExpiredTokens removes all tokens from r that expire after the given
// time.
// TODO ask the store to remove expired tokens?
func (r *registry) deleteExpiredTokens(now time.Time) {
	r.accessTokens = slices.DeleteFunc(r.accessTokens, func(tok *scopedToken) bool {
		return now.After(tok.expires)
	})
}

func (r *registry) accessTokenForScope(scope Scope) *scopedToken {
	for _, tok := range r.accessTokens {
		if tok.scope.Contains(scope) {
			// TODO prefer tokens with less scope?
			return tok
		}
	}
	return nil
}

type emptyConfig struct{}

func (emptyConfig) EntryForRegistry(host string) (ConfigEntry, error) {
	return ConfigEntry{}, nil
}
