# OCI Go modules

This repository holds functionality related to OCI (Open Container Initiative).
It holds the following public Go modules:

* [`ociregistry`](./ociregistry): an abstraction of the OCI registry API,
  along with HTTP client and server implementations and related packages.
* [`cmd/ocisrv`](./cmd/ocisrv): an OCI registry server configured with CUE,
  composing `ociregistry` implementations such as in-memory storage and proxying.

### Contributing

To contribute, please read the [Contribution Guide](CONTRIBUTING.md).

## Code of Conduct

We follow guidelines for participating in CUE community spaces and a reporting
process for handling issues can be found in the [Code of
Conduct](https://cuelang.org/docs/contribution_guidelines/conduct).
