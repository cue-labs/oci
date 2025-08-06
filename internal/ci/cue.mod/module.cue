module: "cuelabs.dev/go/oci/internal/ci"
language: {
	version: "v0.10.0"
}
deps: {
	"cue.dev/x/githubactions@v0": {
		v:       "v0.2.0"
		default: true
	}
	"github.com/cue-lang/tmp/internal/ci@v0": {
		v:       "v0.0.8"
		default: true
	}
}
