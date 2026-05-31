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
	"github.com/google/go-cmp/cmp"
	"github.com/rogpeppe/go-internal/testscript"
)

func TestMain(m *testing.M) {
	// We're using testscript, not for txtar tests,
	// but to access the test executable functionality.
	testscript.Main(m, map[string]func(){
		"docker-credential-test": helperMain,
	})
}

func TestLoadWithNoConfig(t *testing.T) {
	qt.Patch(t, &userHomeDir, func(getenv func(string) string) string {
		return getenv("HOME")
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
	qt.Patch(t, &userHomeDir, func(getenv func(string) string) string {
		return getenv("HOME")
	})
	locations := []struct {
		extraHost string
		env       string
		dir       string
		file      string
		isInline  bool
	}{
		{
			extraHost: "inline-docker-auth-env.example.com",
			env:       "DOCKER_AUTH_CONFIG",
			isInline:  true,
		},
		{
			extraHost: "default-dockerconfig.example.com",
			env:       "HOME",
			dir:       "home",
			file:      ".docker/config.json",
		},
		{
			extraHost: "runtime-containers-auth.example.com",
			env:       "XDG_RUNTIME_DIR",
			dir:       "xdg",
			file:      "containers/auth.json",
		},
	}

	for _, loc := range locations {
		c := []byte(`
{
	"auths": {
		"someregistry.example.com": {
			"username": ` + fmt.Sprintf("%q", loc.env) + `,
			"password": "somepassword"
		},
		` + fmt.Sprintf("%q", loc.extraHost) + `: {
			"username": ` + fmt.Sprintf("%q", loc.env) + `,
			"password": "somepassword"
		}
	}
}`)
		if loc.isInline {
			// Inline config for DOCKER_AUTH_CONFIG.
			t.Setenv(loc.env, string(c))
		} else {
			epath := filepath.Join(d, loc.dir)
			t.Setenv(loc.env, epath)
			cfgPath := filepath.Join(epath, filepath.FromSlash(loc.file))
			err := os.MkdirAll(filepath.Dir(cfgPath), 0o777)
			qt.Assert(t, qt.IsNil(err))

			// Write the config file with a username that
			// reflects where it's stored.
			err = os.WriteFile(cfgPath, c, 0o666)
			qt.Assert(t, qt.IsNil(err))
		}
	}
	for _, loc := range locations {
		t.Run(loc.env+"/per-source", func(t *testing.T) {
			c, err := Load(noRunner)
			qt.Assert(t, qt.IsNil(err))

			info, err := c.EntryForRegistry(loc.extraHost)
			qt.Assert(t, qt.IsNil(err))
			qt.Assert(t, qt.Equals(info, ConfigEntry{
				Username: loc.env,
				Password: "somepassword",
			}))
		})
	}
	for _, loc := range locations {
		t.Run(loc.env+"/precedence", func(t *testing.T) {
			c, err := Load(noRunner)
			qt.Assert(t, qt.IsNil(err))

			info, err := c.EntryForRegistry("someregistry.example.com")
			qt.Assert(t, qt.IsNil(err))
			qt.Assert(t, qt.Equals(info, ConfigEntry{
				Username: loc.env,
				Password: "somepassword",
			}))

			if loc.isInline {
				// Remove the DOCKER_AUTH_CONFIG so that the next
				// level of precedence can be checked.
				err := os.Unsetenv(loc.env)
				qt.Assert(t, qt.IsNil(err))
			} else {
				// Remove the directory containing the above
				// config file so that the next level of precedence
				// can be checked.
				err = os.RemoveAll(filepath.Join(d, loc.dir))
				qt.Assert(t, qt.IsNil(err))
			}
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

// TestLoadDockerConfigOverridesHome checks that $DOCKER_CONFIG replaces
// ~/.docker rather than supplementing it, matching the Docker CLI.
func TestLoadDockerConfigOverridesHome(t *testing.T) {
	d := t.TempDir()
	qt.Patch(t, &userHomeDir, func(getenv func(string) string) string {
		return getenv("HOME")
	})
	homeCfg := filepath.Join(d, "home", ".docker", "config.json")
	err := os.MkdirAll(filepath.Dir(homeCfg), 0o777)
	qt.Assert(t, qt.IsNil(err))
	err = os.WriteFile(homeCfg, []byte(`{"auths": {"home-only.example.com": {"username": "HOME", "password": "pw"}}}`), 0o666)
	qt.Assert(t, qt.IsNil(err))

	t.Setenv("HOME", filepath.Join(d, "home"))
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("DOCKER_AUTH_CONFIG", "")

	// With DOCKER_CONFIG unset, ~/.docker/config.json is consulted.
	t.Setenv("DOCKER_CONFIG", "")
	c, err := Load(noRunner)
	qt.Assert(t, qt.IsNil(err))
	info, err := c.EntryForRegistry("home-only.example.com")
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.Equals(info, ConfigEntry{Username: "HOME", Password: "pw"}))

	// With DOCKER_CONFIG set, ~/.docker is ignored even for home-only hosts.
	t.Setenv("DOCKER_CONFIG", d)
	c, err = Load(noRunner)
	qt.Assert(t, qt.IsNil(err))
	info, err = c.EntryForRegistry("home-only.example.com")
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
	qt.Assert(t, qt.ErrorMatches(err, `invalid config file at \$DOCKER_CONFIG: cannot decode auth field for "someregistry.example.com": invalid base64-encoded string`))
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
	qt.Assert(t, qt.IsNil(err))
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

func TestWithDefaultHelper(t *testing.T) {
	// Note: "test" matches the executable installed using testscript in RunMain.
	c, err := load(t, nil, `
{
	"credsStore": "test"
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

func TestWithDefaultHelperNotFound(t *testing.T) {
	// When there's a helper not associated with any specific
	// host, it ignores the fact that the executable isn't
	// found and uses the regular "auths" info.
	// See https://github.com/cue-lang/cue/issues/2934.
	c, err := load(t, nil, `
{
	"credsStore": "definitely-not-found-executable",
	"auths": {
		"registry-with-basic-auth.com": {
			"username": "u1",
			"password": "p"
		}
	}
}
`)
	qt.Assert(t, qt.IsNil(err))
	info, err := c.EntryForRegistry("registry-with-basic-auth.com")
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.Equals(info, ConfigEntry{
		Username: "u1",
		Password: "p",
	}))
}

func TestWithDefaultHelperOtherError(t *testing.T) {
	// When there's a helper not associated with any specific
	// host, it's still an error if it's any error other than HelperNotFound.
	errHelper := func(helperName, serverURL string) (ConfigEntry, error) {
		return ConfigEntry{}, fmt.Errorf("some error")
	}
	c, err := load(t, errHelper, `
{
	"credsStore": "test",
	"auths": {
		"registry-with-basic-auth.com": {
			"username": "u1",
			"password": "p"
		}
	}
}
`)
	qt.Assert(t, qt.IsNil(err))
	_, err = c.EntryForRegistry("registry-with-basic-auth.com")
	qt.Assert(t, qt.ErrorMatches(err, `some error`))
}

func TestWithSpecificHelperNotFound(t *testing.T) {
	// When there's a helper specifically configured for a host,
	// it _is_ an error that the helper isn't found.
	c, err := load(t, nil, `
{
	"credHelpers": {
		"registry-with-basic-auth.com": "definitely-not-found-executable"
	}
}
`)
	qt.Assert(t, qt.IsNil(err))
	_, err = c.EntryForRegistry("registry-with-basic-auth.com")
	qt.Assert(t, qt.ErrorMatches(err, `helper not found: exec: "docker-credential-definitely-not-found-executable": executable file not found .*`))
}

func TestWithHelperAndExplicitEnv(t *testing.T) {
	d := t.TempDir()
	// Note: "test" matches the executable installed using testscript in RunMain.
	err := os.WriteFile(filepath.Join(d, "config.json"), []byte(`
{
	"credHelpers": {
		"registry-with-env-lookup.com": "test"
	}
}
`), 0o666)
	qt.Assert(t, qt.IsNil(err))
	c, err := LoadWithEnv(nil, []string{
		"DOCKER_CONFIG=" + d,
		"TEST_SECRET=foo",
	})
	qt.Assert(t, qt.IsNil(err))
	info, err := c.EntryForRegistry("registry-with-env-lookup.com")
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.Equals(info, ConfigEntry{
		Username: "someuser",
		Password: "foo",
	}))
}

func TestConfigSourceRead(t *testing.T) {
	tests := []struct {
		name   string
		source func(t *testing.T) configSource
		want   []byte
	}{
		{
			name: "zero",
			source: func(t *testing.T) configSource {
				return configSource{}
			},
			want: nil,
		},
		{
			name: "raw",
			source: func(t *testing.T) configSource {
				return configSource{Raw: []byte("hello world")}
			},
			want: []byte("hello world"),
		},
		{
			name: "valid file",
			source: func(t *testing.T) configSource {
				path := filepath.Join(t.TempDir(), "some-file.txt")
				err := os.WriteFile(path, []byte("content of some file"), 0o600)
				qt.Assert(t, qt.IsNil(err))
				return configSource{Path: path}
			},
			want: []byte("content of some file"),
		},
		{
			name: "file not found",
			source: func(t *testing.T) configSource {
				return configSource{Path: "/path/that/does/not/exist"}
			},
			want: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.source(t).Read()
			qt.Assert(t, qt.IsNil(err))
			qt.Assert(t, qt.DeepEquals(got, test.want))
		})
	}
}

func TestConfigSourceReadError(t *testing.T) {
	source := configSource{Path: t.TempDir()}
	_, err := source.Read()
	qt.Assert(t, qt.IsNotNil(err))
}

func TestConfigDataMergeInto(t *testing.T) {
	tests := []struct {
		name   string
		config configData
		other  configData
		want   configData
	}{
		{
			name:   "zero",
			config: configData{},
			other: configData{
				Auths:       map[string]authConfig{},
				CredHelpers: map[string]string{},
			},
			want: configData{
				Auths:       map[string]authConfig{},
				CredHelpers: map[string]string{},
			},
		},
		{
			name: "set on empty",
			config: configData{
				Auths:       map[string]authConfig{"foo": {Username: "hello"}},
				CredHelpers: map[string]string{"bar": "echo"},
				CredsStore:  "cat",
			},
			other: configData{
				Auths:       map[string]authConfig{},
				CredHelpers: map[string]string{},
			},
			want: configData{
				Auths:       map[string]authConfig{"foo": {Username: "hello"}},
				CredHelpers: map[string]string{"bar": "echo"},
				CredsStore:  "cat",
			},
		},
		{
			name: "keep original",
			config: configData{
				Auths:       map[string]authConfig{"foo": {Username: "hello"}},
				CredHelpers: map[string]string{"bar": "echo"},
				CredsStore:  "cat",
			},
			other: configData{
				Auths:       map[string]authConfig{"foo": {Auth: "hmm", IdentityToken: "cool"}},
				CredHelpers: map[string]string{"bar": "lorem"},
				CredsStore:  "yes",
			},
			want: configData{
				Auths:       map[string]authConfig{"foo": {Auth: "hmm", IdentityToken: "cool"}},
				CredHelpers: map[string]string{"bar": "lorem"},
				CredsStore:  "yes",
			},
		},
		{
			name: "merge",
			config: configData{
				Auths:       map[string]authConfig{"foo": {Username: "hello"}},
				CredHelpers: map[string]string{"bar": "echo"},
				CredsStore:  "cat",
			},
			other: configData{
				Auths:       map[string]authConfig{"moo": {Auth: "hmm", IdentityToken: "cool"}},
				CredHelpers: map[string]string{"doo": "ipsum"},
			},
			want: configData{
				Auths: map[string]authConfig{
					"foo": {Username: "hello"},
					"moo": {Auth: "hmm", IdentityToken: "cool"},
				},
				CredHelpers: map[string]string{
					"bar": "echo",
					"doo": "ipsum",
				},
				CredsStore: "cat",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.config.mergeInto(&test.other)
			qt.Assert(t, qt.CmpEquals(test.other, test.want, cmp.AllowUnexported(authConfig{})))
		})
	}
}

func load(t *testing.T, runner HelperRunner, cfgData string) (Config, error) {
	d := t.TempDir()
	t.Setenv("DOCKER_CONFIG", d)
	err := os.WriteFile(filepath.Join(d, "config.json"), []byte(cfgData), 0o666)
	qt.Assert(t, qt.IsNil(err))
	return Load(runner)
}

func noRunner(helperName, serverURL string) (ConfigEntry, error) {
	panic("no helpers available")
}

// helperMain implements a docker credential command main function.
func helperMain() {
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
	case "registry-with-env-lookup.com":
		fmt.Printf(`
{
	"Username": "someuser",
	"Secret": %q
}`, os.Getenv("TEST_SECRET"))
	case "registry-with-error.com":
		fmt.Fprintf(os.Stderr, "some error\n")
		os.Exit(1)
	default:
		fmt.Printf("credentials not found in native keychain\n")
		os.Exit(1)
	}
}
