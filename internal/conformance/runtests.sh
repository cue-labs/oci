#!/usr/bin/env rc

# Registry details
OCI_ROOT_URL='http://localhost:5000'
OCI_NAMESPACE='myorg/myrepo'
OCI_CROSSMOUNT_NAMESPACE='myorg/other'
OCI_USERNAME='myuser'
OCI_PASSWORD='mypass'

# Which workflows to run
OCI_TEST_PULL=1
OCI_TEST_PUSH=1
OCI_TEST_CONTENT_DISCOVERY=1
OCI_TEST_CONTENT_MANAGEMENT=1

# Extra settings
OCI_HIDE_SKIPPED_WORKFLOWS=0
OCI_DEBUG=0
OCI_DELETE_MANIFEST_BEFORE_BLOBS=0

ACK_GINKGO_DEPRECATIONS=1.16.5
go test -v github.com/opencontainers/distribution-spec/conformance
