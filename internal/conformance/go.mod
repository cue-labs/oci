// Note: we're defining this as a separate module so we don't have
// to take on the official conformance module's dependencies.

module github.com/rogpeppe/ociregistry/internal/conformance

go 1.21

// Use the local repository for the ociregistry module.
replace github.com/rogpeppe/ociregistry => ../..

replace github.com/opencontainers/distribution-spec/conformance => /home/rogpeppe/other/oci-distribution-spec/conformance

require (
	github.com/opencontainers/distribution-spec/conformance v0.0.0-20230516164017-7fcdf80f3c8b
	github.com/rogpeppe/ociregistry v0.0.0-00010101000000-000000000000
)

require (
	github.com/asaskevich/govalidator v0.0.0-20230301143203-a9d515a09cc2 // indirect
	github.com/bloodorangeio/reggie v0.6.1 // indirect
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	github.com/go-resty/resty/v2 v2.7.0 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/nxadm/tail v1.4.8 // indirect
	github.com/onsi/ginkgo v1.16.5 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.0-rc2 // indirect
	golang.org/x/net v0.8.0 // indirect
	golang.org/x/sys v0.6.0 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
)
