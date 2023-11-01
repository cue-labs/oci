package ociauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/exp/slices"
)

// TODO decide on a good value for this.
const oauthClientID = "ociregistry"

type Authorizer interface {
	// DoRequest acquires authorization and invokes the given
	// request. It may invoke the request more than once, and can
	// use [http.Request.GetBody] to reset the request body if it
	// gets consumed.
	//
	// It ensures that the authorization token used will have at least
	// the capability to execute operations in requiredScope; wantScope may contain
	// other operations that may be executed in the future: if a new
	// token is acquired, that will be taken into account too.
	//
	// It's OK to call AuthorizeRequest concurrently.
	DoRequest(req *http.Request, requiredScope, wantScope Scope) (*http.Response, error)
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
	// scopeString holds the original string representing the scope
	// of the token. We keep this around because Scope does
	// not necessarily round-trip without loss and servers can be
	// picky about the exact representation.
	scopeString string
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

// AuthorizeRequest implements [Authorizer].DoRequest.
func (a *StdAuthorizer) DoRequest(req *http.Request, requiredScope, wantScope Scope) (*http.Response, error) {
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
	// TODO think about locking in general
	return r.doRequest(req.Context(), req, requiredScope, wantScope)
}

func (r *registry) doRequest(ctx context.Context, req *http.Request, requiredScope, wantScope Scope) (*http.Response, error) {
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
	resp.Body.Close()
	challenge := challengeFromResponse(resp)
	if challenge == nil {
		return resp, nil
	}
	r.wwwAuthenticate = challenge

	if r.wwwAuthenticate.scheme == "bearer" {
		scope := ParseScope(r.wwwAuthenticate.params["scope"])
		scope = scope.Union(requiredScope).Union(wantScope)
		log.Printf("srvScope %v; requiredScope %v; wantScope %v", scope, requiredScope, wantScope)
		log.Printf("acquiring token with full scope %v", scope)
		accessToken, err := r.acquireAccessToken(ctx, scope)
		if err != nil {
			log.Printf("-> cannot acquire token: %v", err)
			return nil, err
		}
		log.Printf("-> token %q", accessToken)
		req.Header.Set("Authorization", "Bearer "+accessToken)
	} else if r.basic != nil {
		req.SetBasicAuth(r.basic.username, r.basic.password)
	}
	return r.authorizer.httpClient.Do(req)
}

func (r *registry) setAuthorization(ctx context.Context, req *http.Request, requiredScope, wantScope Scope) error {
	// Remove tokens that have expired or will expire soon so that
	// the caller doesn't start using a token only for it to expire while it's
	// making the request.
	r.deleteExpiredTokens(time.Now().Add(-time.Second))

	if accessToken := r.accessTokenForScope(requiredScope); accessToken != nil {
		// We have a potentially valid access token. Use it.
		req.Header.Set("Authorization", "Bearer "+accessToken.token)
		return nil
	}
	if r.refreshToken != "" && r.wwwAuthenticate != nil && r.wwwAuthenticate.scheme == "bearer" {
		// We've got a refresh token that we can use to try to
		// acquire an access token and we've seen a Www-Authenticate response
		// that tells us how we can use it.

		// TODO
		// maybe we should look at r.wwwAuthenticate.params["scope"] and
		// union it with requiredScope, but maybe not:
		// the scope is (probably) specific to a given request and that's
		// not necessarily the same as the request we're making here.

		// TODO we could also somehow manage acquisition of tokens in parallel.
		// although maybe we don't want to do that - we could just serialize
		// all token acquisition for a given registry for now. the client can
		// guard against multiple round trips by setting up the scope appropriately.

		accessToken, err := r.acquireAccessToken(ctx, requiredScope.Union(wantScope))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
		return nil
	}
	if r.wwwAuthenticate == nil {
		// We haven't seen a 401 response yet. Avoid putting any
		// basic authorization in the request, because that can mean that
		// the server sends a 401 response without a Www-Authenticate
		// header.
		return nil
	}
	if r.wwwAuthenticate.scheme != "bearer" && r.basic != nil {
		req.SetBasicAuth(r.basic.username, r.basic.password)
		return nil
	}
	return nil
}

// init initializes the registry instance by acquiring auth information from
// the Config, if available. As this might be slow (invoking InfoForRegistry
// can end up invoking slow external commands), we ensure that it's only
// done once.
// TODO it's possible that this could take a very long time, during which
// the outer context is cancelled, but we'll ignore that. We probably shouldn't.
func (r *registry) init() error {
	inner := func() error {
		info, err := r.authorizer.config.InfoForRegistry(r.host)
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
// The token will be acquired with the given scope.
//
// This method assumes that there has been a previous 401 response with
// a Www-Authenticate: Bearer... header.
func (r *registry) acquireAccessToken(ctx context.Context, scope Scope) (string, error) {
	tok, err := r.acquireToken(ctx, scope)
	if err != nil {
		return "", err
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
	if tok.ExpiresIn == 0 {
		expires = time.Now().Add(60 * time.Second) // TODO link to where this is mentioned
	} else {
		expires = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
	}
	r.accessTokens = append(r.accessTokens, &scopedToken{
		scope: scope,
		// TODO scopeString ?
		token:   accessToken,
		expires: expires,
	})
	// TODO persist the access token?
	return accessToken, nil
}

func (r *registry) acquireToken(ctx context.Context, scope Scope) (*wireToken, error) {
	log.Printf("acquireToken, basic: %#v", r.basic)
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

func (emptyConfig) InfoForRegistry(host string) (ConfigInfo, error) {
	return ConfigInfo{}, nil
}
