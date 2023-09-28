// Note: we're defining this as a separate module so we don't have
// to take on the official conformance module's dependencies.

module cuelabs.dev/go/oci/ociregistry/internal/conformance

go 1.21

// Note that we use a replace directive for the ociregistry module
// even though we're using a go.work workspace as well,
// so that `go mod tidy` always uses ociregistry from the same repository.
// Moreover, we always want the conformance tests to run against the local version.
replace cuelabs.dev/go/oci/ociregistry => ../..

require (
	cuelabs.dev/go/oci/ociregistry v0.0.0-00010101000000-000000000000
	github.com/go-quicktest/qt v1.100.0
	github.com/opencontainers/distribution-spec/conformance v0.0.0-20230516164017-7fcdf80f3c8b
	github.com/opencontainers/go-digest v1.0.0
	github.com/opencontainers/image-spec v1.1.0-rc4
	oras.land/oras-go/v2 v2.2.0
)

require (
	github.com/asaskevich/govalidator v0.0.0-20230301143203-a9d515a09cc2 // indirect
	github.com/bloodorangeio/reggie v0.6.1 // indirect
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	github.com/go-resty/resty/v2 v2.7.0 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/google/go-cmp v0.5.9 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/kr/pretty v0.3.0 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/nxadm/tail v1.4.8 // indirect
	github.com/onsi/ginkgo v1.16.5 // indirect
	github.com/rogpeppe/go-internal v1.9.0 // indirect
	golang.org/x/net v0.8.0 // indirect
	golang.org/x/sync v0.2.0 // indirect
	golang.org/x/sys v0.6.0 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
)
