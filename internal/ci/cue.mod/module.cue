module: "cuelabs.dev/go/oci/internal/ci"
language: {
	version: "v0.16.0"
}
deps: {
	"cue.dev/x/githubactions@v0": {
		v:       "v0.4.0"
		default: true
	}
	"github.com/cue-lang/tmp/internal/ci@v0": {
		v:       "v0.0.19"
		default: true
	}
}
