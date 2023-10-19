package ociauthconfig

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Config holds auth information for OCI registries as read from a configuration file.
type Config struct {
	data   configData
	runner Runner
}

// AuthInfo holds authorization information that can be used to connect
// to a registry.
type AuthInfo struct {
	// IdentityToken holds a token for use in OAuth flows; if this is empty,
	// Username and Password will hold basic auth credentials.
	IdentityToken string

	// Username holds the username for use with basic auth.
	// This is only valid if IdentityToken is empty. If it's empty,
	// no auth information is configured.
	Username string `json:"username,omitempty"`

	// Password holds the password for use with Username.
	Password string `json:"password,omitempty"`
}

// Runner is the function used to execute auth "helper"
// commands. It's passed the helper name as specified in the configuration file,
// without the "docker-credential-helper-" prefix.
//
// If the credentials are not found, it should return the zero AuthInfo
// and no error.
type Runner = func(helperName string, serverURL string) (AuthInfo, error)

// configData holds the part of ~/.docker/config.json that pertains to auth.
type configData struct {
	Auths       map[string]authConfig `json:"auths"`
	CredsStore  string                `json:"credsStore,omitempty"`
	CredHelpers map[string]string     `json:"credHelpers,omitempty"`
}

// authConfig contains authorization information for connecting to a Registry.
type authConfig struct {
	// derivedFrom records the entries from which this one was derived.
	// If this is empty, the entry was explicitly present.
	derivedFrom []string

	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Auth     string `json:"auth,omitempty"`

	// IdentityToken is used to authenticate the user and get
	// an access token for the registry.
	IdentityToken string `json:"identitytoken,omitempty"`

	// RegistryToken is a bearer token to be sent to a registry
	RegistryToken string `json:"registrytoken,omitempty"`
}

// Load loads the auth configuration from the first location it can find.
// It uses runner to run any external helper commands; if runner
// is nil, ExecHelper will be used.
//
// In order it tries:
// - $DOCKER_CONFIG/config.json
// - ~/.docker/config.json
// - $XDG_RUNTIME_DIR/containers/auth.json
func Load(runner Runner) (*Config, error) {
	if runner == nil {
		runner = ExecHelper
	}
	for _, f := range configFileLocations {
		filename := f()
		if filename == "" {
			continue
		}
		data, err := os.ReadFile(filename)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		f, err := decodeConfig(data)
		if err != nil {
			return nil, fmt.Errorf("invalid config file %q: %v", filename, err)
		}
		return &Config{
			data:   f,
			runner: runner,
		}, nil
	}
	return &Config{
		runner: runner,
	}, nil
}

// osUserHomeDir is defined as a variable so it can be overridden by tests.
var osUserHomeDir = os.UserHomeDir

var configFileLocations = []func() string{
	func() string {
		if d := os.Getenv("DOCKER_CONFIG"); d != "" {
			return filepath.Join(d, "config.json")
		}
		return ""
	},
	func() string {
		home, err := osUserHomeDir()
		if err != nil {
			return ""
		}
		return filepath.Join(home, ".docker", "config.json")
	},
	// If neither of the above locations was found, look for Podman's auth at
	// $XDG_RUNTIME_DIR/containers/auth.json and attempt to load it as a
	// Docker config.
	func() string {
		if d := os.Getenv("XDG_RUNTIME_DIR"); d != "" {
			return filepath.Join(d, "containers", "auth.json")
		}
		return ""
	},
}

// AuthInfoForRegistry returns auth information for connecting to the registry with the
// given host name. If no registry is found, it returns the zero AuthInfo and a nil error.
func (c *Config) AuthInfoForRegistry(registryHostname string) (AuthInfo, error) {
	var helper string
	helper, ok := c.data.CredHelpers[registryHostname]
	if !ok {
		helper = c.data.CredsStore
	}
	if helper != "" {
		return c.runner(helper, registryHostname)
	}
	auth := c.data.Auths[registryHostname]
	if auth.IdentityToken != "" && auth.Username != "" {
		return AuthInfo{}, fmt.Errorf("ambiguous auth credentials")
	}
	if len(auth.derivedFrom) > 1 {
		return AuthInfo{}, fmt.Errorf("more than one auths entry for %q (%s)", registryHostname, strings.Join(auth.derivedFrom, ", "))
	}

	return AuthInfo{
		IdentityToken: auth.IdentityToken,
		Username:      auth.Username,
		Password:      auth.Password,
	}, nil
}

func decodeConfig(data []byte) (configData, error) {
	var f configData
	if err := json.Unmarshal(data, &f); err != nil {
		return configData{}, fmt.Errorf("decode failed: %v", err)
	}
	for addr, ac := range f.Auths {
		if ac.Auth != "" {
			var err error
			ac.Username, ac.Password, err = decodeAuth(ac.Auth)
			if err != nil {
				return configData{}, fmt.Errorf("cannot decode auth field for %q: %v", addr, err)
			}
		}
		f.Auths[addr] = ac
		if !strings.Contains(addr, "//") {
			continue
		}
		// It looks like it might be a URL, so follow the original logic
		// and extract the host name for later lookup. Explicit
		// entries override implicit, and if several entries map to
		// the same host, we record that so we can return an error
		// later if that host is looked up (this avoids the nondeterministic
		// behavior found in the original code when this happens).
		addr1 := urlHost(addr)
		if addr1 == addr {
			continue
		}
		if ac1, ok := f.Auths[addr1]; ok {
			if len(ac1.derivedFrom) == 0 {
				// Don't override an explicit entry.
				continue
			}
			ac = ac1
		}
		ac.derivedFrom = append(ac.derivedFrom, addr)
		sort.Strings(ac.derivedFrom)
		f.Auths[addr1] = ac
	}
	return f, nil
}

// urlHost returns the host part of a registry URL.
// Copied from github.com/docker/docker/registry.ConvertToHostname
// to keep the logic the same as that.
func urlHost(url string) string {
	stripped := url
	if strings.HasPrefix(url, "http://") {
		stripped = strings.TrimPrefix(url, "http://")
	} else if strings.HasPrefix(url, "https://") {
		stripped = strings.TrimPrefix(url, "https://")
	}

	hostName, _, _ := strings.Cut(stripped, "/")
	return hostName
}

// decodeAuth decodes a base64 encoded string and returns username and password
func decodeAuth(authStr string) (string, string, error) {
	s, err := base64.StdEncoding.DecodeString(authStr)
	if err != nil {
		return "", "", fmt.Errorf("invalid base64-encoded string")
	}
	username, password, ok := strings.Cut(string(s), ":")
	if !ok || username == "" {
		return "", "", errors.New("no username found")
	}
	return username, strings.Trim(password, "\x00"), nil
}

// ExecHelper executes an external program to get the credentials from a native store.
// It implements Runner.
func ExecHelper(helperName string, serverURL string) (AuthInfo, error) {
	var out bytes.Buffer
	cmd := exec.Command("docker-credential-"+helperName, "get")
	// TODO this doesn't produce a decent error message for
	// other helpers such as gcloud that print errors to stderr.
	cmd.Stderr = os.Stderr
	cmd.Stdin = strings.NewReader(serverURL)
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		if !errors.As(err, new(*exec.ExitError)) {
			return AuthInfo{}, fmt.Errorf("cannot run auth helper: %v", err)
		}
		t := strings.TrimSpace(out.String())
		if t == "credentials not found in native keychain" {
			return AuthInfo{}, nil
		}
		return AuthInfo{}, fmt.Errorf("error getting credentials: %s", t)
	}

	// helperCredentials defines the JSON encoding of the data printed
	// by credentials helper programs.
	//
	// TODO in docker, there are no json tags for this which implies
	// that it also accepts case-insensitive keys. Find out whether
	// real credentials helpers print non-uppercase keys or not.
	type helperCredentials struct {
		Username string `json:"Username"`
		Secret   string `json:"Secret"`
	}
	var creds helperCredentials
	if err := json.Unmarshal(out.Bytes(), &creds); err != nil {
		return AuthInfo{}, err
	}
	if creds.Username == "<token>" {
		return AuthInfo{
			IdentityToken: creds.Secret,
		}, nil
	}
	return AuthInfo{
		Password: creds.Secret,
		Username: creds.Username,
	}, nil
}
