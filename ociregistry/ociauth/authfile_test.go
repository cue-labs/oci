package ociauth

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-quicktest/qt"
	"github.com/rogpeppe/go-internal/testscript"
)

func TestMain(m *testing.M) {
	// We're using testscript, not for txtar tests,
	// but to access the test executable functionality.
	os.Exit(testscript.RunMain(m, map[string]func() int{
		"docker-credential-test": helperMain,
	}))
}

func TestLoadWithNoConfig(t *testing.T) {
	qt.Patch(t, &osUserHomeDir, func() (string, error) {
		return os.Getenv("HOME"), nil
	})
	t.Setenv("HOME", "")
	t.Setenv("DOCKER_CONFIG", "")
	t.Setenv("XDG_RUNTIME_DIR", "")
	c, err := Load(noRunner)
	qt.Assert(t, qt.IsNil(err))
	info, err := c.EntryForRegistry("some.org")
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.Equals(info, ConfigEntry{}))
}

func TestLoad(t *testing.T) {
	// Write config files in all the places, so we can check
	// that the precedence works OK.
	d := t.TempDir()
	qt.Patch(t, &osUserHomeDir, func() (string, error) {
		return os.Getenv("HOME"), nil
	})
	locations := []struct {
		env  string
		dir  string
		file string
	}{{
		env:  "DOCKER_CONFIG",
		dir:  "dockerconfig",
		file: "config.json",
	}, {
		env:  "HOME",
		dir:  "home",
		file: ".docker/config.json",
	}, {
		env:  "XDG_RUNTIME_DIR",
		dir:  "xdg",
		file: "containers/auth.json",
	}}
	for _, loc := range locations {
		epath := filepath.Join(d, loc.dir)
		t.Setenv(loc.env, epath)
		cfgPath := filepath.Join(epath, filepath.FromSlash(loc.file))
		err := os.MkdirAll(filepath.Dir(cfgPath), 0o777)
		qt.Assert(t, qt.IsNil(err))
		// Write the config file with a username that
		// reflects where it's stored.
		err = os.WriteFile(cfgPath, []byte(`
{
	"auths": {
		"someregistry.example.com": {
			"username": `+fmt.Sprintf("%q", loc.env)+`,
			"password": "somepassword"
		}
	}
}`), 0o666)
	}
	for _, loc := range locations {
		t.Run(loc.env, func(t *testing.T) {
			c, err := Load(noRunner)
			qt.Assert(t, qt.IsNil(err))
			info, err := c.EntryForRegistry("someregistry.example.com")
			qt.Assert(t, qt.IsNil(err))
			qt.Assert(t, qt.Equals(info, ConfigEntry{
				Username: loc.env,
				Password: "somepassword",
			}))
			// Remove the directory containing the above
			// config file so that the next level of precedence
			// can be checked.
			err = os.RemoveAll(filepath.Join(d, loc.dir))
			qt.Assert(t, qt.IsNil(err))
		})
	}
	// When there's no config file available, it should return
	// an empty configuration and no error.
	c, err := Load(noRunner)
	qt.Assert(t, qt.IsNil(err))

	info, err := c.EntryForRegistry("someregistry.example.com")
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.Equals(info, ConfigEntry{}))
}

func TestWithBase64Auth(t *testing.T) {
	c, err := load(t, noRunner, `
{
	"auths": {
		"someregistry.example.com": {
			"auth": "dGVzdHVzZXI6cGFzc3dvcmQ="
		}
	}
}`)
	qt.Assert(t, qt.IsNil(err))
	info, err := c.EntryForRegistry("someregistry.example.com")
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.Equals(info, ConfigEntry{
		Username: "testuser",
		Password: "password",
	}))
}

func TestWithMalformedBase64Auth(t *testing.T) {
	_, err := load(t, noRunner, `
{
	"auths": {
		"someregistry.example.com": {
			"auth": "!!!"
		}
	}
}`)
	qt.Assert(t, qt.ErrorMatches(err, `invalid config file ".*": cannot decode auth field for "someregistry.example.com": invalid base64-encoded string`))
}

func TestWithAuthAndUsername(t *testing.T) {
	// An auth field overrides the username/password pair.
	c, err := load(t, noRunner, `
{
	"auths": {
		"someregistry.example.com": {
			"auth": "dGVzdHVzZXI6cGFzc3dvcmQ=",
			"username": "foo",
			"password": "bar"
		}
	}
}`)
	qt.Assert(t, qt.IsNil(err))
	info, err := c.EntryForRegistry("someregistry.example.com")
	qt.Assert(t, qt.Equals(info, ConfigEntry{
		Username: "testuser",
		Password: "password",
	}))
}

func TestWithURLEntry(t *testing.T) {
	c, err := load(t, noRunner, `
{
	"auths": {
		"https://someregistry.example.com/v1": {
			"username": "foo",
			"password": "bar"
		}
	}
}`)
	qt.Assert(t, qt.IsNil(err))
	info, err := c.EntryForRegistry("someregistry.example.com")
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.Equals(info, ConfigEntry{
		Username: "foo",
		Password: "bar",
	}))
}

func TestWithURLEntryAndExplicitHost(t *testing.T) {
	c, err := load(t, noRunner, `
{
	"auths": {
		"https://someregistry.example.com/v1": {
			"username": "foo",
			"password": "bar"
		},
		"someregistry.example.com": {
			"username": "baz",
			"password": "arble"
		}
	}
}`)
	qt.Assert(t, qt.IsNil(err))
	info, err := c.EntryForRegistry("someregistry.example.com")
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.Equals(info, ConfigEntry{
		Username: "baz",
		Password: "arble",
	}))
	info, err = c.EntryForRegistry("https://someregistry.example.com/v1")
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.Equals(info, ConfigEntry{
		Username: "foo",
		Password: "bar",
	}))
}

func TestWithMultipleURLsAndSameHost(t *testing.T) {
	c, err := load(t, noRunner, `
{
	"auths": {
		"https://someregistry.example.com/v1": {
			"username": "u1",
			"password": "p"
		},
		"http://someregistry.example.com/v1": {
			"username": "u2",
			"password": "p"
		},
		"http://someregistry.example.com/v2": {
			"username": "u3",
			"password": "p"
		}
	}
}`)
	qt.Assert(t, qt.IsNil(err))
	_, err = c.EntryForRegistry("someregistry.example.com")
	qt.Assert(t, qt.ErrorMatches(err, `more than one auths entry for "someregistry.example.com" \(http://someregistry.example.com/v1, http://someregistry.example.com/v2, https://someregistry.example.com/v1\)`))
}

func TestWithHelperBasic(t *testing.T) {
	// Note: "test" matches the executable installed using testscript in RunMain.
	c, err := load(t, nil, `
{
	"credHelpers": {
		"registry-with-basic-auth.com": "test"
	}
}
`)
	qt.Assert(t, qt.IsNil(err))
	info, err := c.EntryForRegistry("registry-with-basic-auth.com")
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.Equals(info, ConfigEntry{
		Username: "someuser",
		Password: "somesecret",
	}))
}

func TestWithHelperToken(t *testing.T) {
	// Note: "test" matches the executable installed using testscript in RunMain.
	c, err := load(t, nil, `
{
	"credHelpers": {
		"registry-with-token.com": "test"
	}
}
`)
	qt.Assert(t, qt.IsNil(err))
	info, err := c.EntryForRegistry("registry-with-token.com")
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.Equals(info, ConfigEntry{
		RefreshToken: "sometoken",
	}))
}

func TestWithHelperRegistryNotFound(t *testing.T) {
	// Note: "test" matches the executable installed using testscript in RunMain.
	c, err := load(t, nil, `
{
	"credHelpers": {
		"other.com": "test"
	}
}
`)
	qt.Assert(t, qt.IsNil(err))
	info, err := c.EntryForRegistry("other.com")
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.Equals(info, ConfigEntry{}))
}

func TestWithHelperRegistryOtherError(t *testing.T) {
	// Note: "test" matches the executable installed using testscript in RunMain.
	c, err := load(t, nil, `
{
	"credHelpers": {
		"registry-with-error.com": "test"
	}
}
`)
	qt.Assert(t, qt.IsNil(err))
	_, err = c.EntryForRegistry("registry-with-error.com")
	qt.Assert(t, qt.ErrorMatches(err, `error getting credentials: some error`))
}

func load(t *testing.T, runner HelperRunner, cfgData string) (Config, error) {
	d := t.TempDir()
	t.Setenv("DOCKER_CONFIG", d)
	err := os.WriteFile(filepath.Join(d, "config.json"), []byte(cfgData), 0o666)
	qt.Assert(t, qt.IsNil(err))
	return Load(runner)
}

func noRunner(helperName string, serverURL string) (ConfigEntry, error) {
	panic("no helpers available")
}

// helperMain implements a docker credential command main function.
func helperMain() int {
	flag.Parse()
	if flag.NArg() != 1 || flag.Arg(0) != "get" {
		log.Fatal("usage: docker-credential-test get")
	}
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		log.Fatal(err)
	}
	switch string(input) {
	case "registry-with-basic-auth.com":
		fmt.Printf(`
{
	"Username": "someuser",
	"Secret": "somesecret"
}`)
	case "registry-with-token.com":
		fmt.Printf(`
{
	"Username": "<token>",
	"Secret": "sometoken"
}
`)
	case "registry-with-error.com":
		fmt.Fprintf(os.Stderr, "some error\n")
		return 1
	default:
		fmt.Printf("credentials not found in native keychain\n")
		return 1
	}
	return 0
}
