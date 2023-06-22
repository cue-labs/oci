// Package ociref implements cross-registry OCI operations.
package ociref

import (
	"fmt"
	"regexp"
	"strings"

	"cuelabs.dev/go/oci/ociregistry"
)

// The following regular expressions derived from code in the
// [github.com/distribution/distribution/v3/reference] package.
const (
	// alphanumeric defines the alphanumeric atom, typically a
	// component of names. This only allows lower case characters and digits.
	alphanumeric = `[a-z0-9]+`

	// separator defines the separators allowed to be embedded in name
	// components. This allows one period, one or two underscore and multiple
	// dashes. Repeated dashes and underscores are intentionally treated
	// differently. In order to support valid hostnames as name components,
	// supporting repeated dash was added. Additionally double underscore is
	// now allowed as a separator to loosen the restriction for previously
	// supported names.
	// TODO the distribution spec doesn't allow these variations.
	separator = `(?:[._]|__|[-]+)`

	// domainNameComponent restricts the registry domain component of a
	// repository name to start with a component as defined by DomainRegexp.
	domainNameComponent = `(?:[a-zA-Z0-9](?:[a-zA-Z0-9-]*[a-zA-Z0-9])?)`

	// ipv6address are enclosed between square brackets and may be represented
	// in many ways, see rfc5952. Only IPv6 in compressed or uncompressed format
	// are allowed, IPv6 zone identifiers (rfc6874) or Special addresses such as
	// IPv4-Mapped are deliberately excluded.
	ipv6address = `(?:\[[a-fA-F0-9:]+\])`

	// optionalPort matches an optional port-number including the port separator
	// (e.g. ":80").
	port = `[0-9]+`

	// tag matches valid tag names. From docker/docker:graph/tags.go.
	// TODO The distribution spec allows underscores here.
	// Also: max 127 characters.
	tag = `(?:\w[\w.-]*)`

	// domainName defines the structure of potential domain components
	// that may be part of image names. This is purposely a subset of what is
	// allowed by DNS to ensure backwards compatibility with Docker image
	// names. This includes IPv4 addresses on decimal format.
	//
	// Note: we purposely exclude domain names without dots here,
	// because otherwise we can't tell if the first component is
	// a host name or not when it doesn't have a port.
	// When it does have a port, the distinction is clear.
	//
	domainName = `(?:` + domainNameComponent + `(?:\.` + domainNameComponent + `)+` + `)`

	// host defines the structure of potential domains based on the URI
	// Host subcomponent on rfc3986. It may be a subset of DNS domain name,
	// or an IPv4 address in decimal format, or an IPv6 address between square
	// brackets (excluding zone identifiers as defined by rfc6874 or special
	// addresses such as IPv4-Mapped).
	host = `(?:` + domainName + `|` + ipv6address + `)`

	// allowed by the URI Host subcomponent on rfc3986 to ensure backwards
	// compatibility with Docker image names.
	// Note: that we require the port when the host name looks like a regular
	// name component.
	domainAndPort = `(?:` + host + `(?:` + `:` + port + `)?` + `|` + domainNameComponent + `:` + port + `)`

	// pathComponent restricts path-components to start with an alphanumeric
	// character, with following parts able to be separated by a separator
	// (one period, one or two underscore and multiple dashes).
	pathComponent = `(?:` + alphanumeric + `(?:` + separator + alphanumeric + `)*` + `)`

	// repoName matches the name of a repository. It consists of one
	// or more forward slash (/) delimited path-components:
	//
	//	pathComponent[[/pathComponent] ...] // e.g., "library/ubuntu"
	repoName = pathComponent + `(?:` + `/` + pathComponent + `)*`
)

var referencePat = regexp.MustCompile(
	`^(?:` +
		`(?:` + `(` + domainAndPort + `)` + `/` + `)?` + // capture 1: host
		`(` + repoName + `)` + // capture 2: repository name
		`(?:` + `:(` + tag + `))?` + // capture 3: tag
		`(?:` + `@(.+))?` + // capture 4: digest; rely on go-digest to find issues
		`)$`,
)

// Reference represents an entry in an OCI repository.
type Reference struct {
	// Host holds the host name of the registry
	// within which the repository is stored. This might
	// be empty.
	Host string

	// Repository holds the repository name.
	Repository string

	// Tag holds the TAG part of a :TAG or :TAG@DIGEST reference.
	// When Digest is set as well as Tag, the tag will be verified
	// to exist and have the expected digest.
	Tag string

	// Digest holds the DIGEST part of an @DIGEST reference
	// or of a :TAG@DIGEST reference.
	Digest ociregistry.Digest
}

// Parse parses a reference string that must include
// a host name component.
//
// It is represented in string form as HOST/NAME[:TAG|@DIGEST]
// form: the same syntax accepted by "docker pull".
// Unlike "docker pull" however, there is no default registry: when
// presented with a bare repository name, Parse will return an error.
func Parse(refStr string) (Reference, error) {
	ref, err := ParseRelative(refStr)
	if err != nil {
		return Reference{}, err
	}
	if ref.Host == "" {
		return Reference{}, fmt.Errorf("reference does not contain host name")
	}
	return ref, nil
}

// ParseRelative parses a reference string that may
// or may not include a host name component.
//
// It is represented in string form as [HOST/]NAME[:TAG|@DIGEST]
// form: the same syntax accepted by "docker pull".
// Unlike "docker pull" however, there is no default registry: when
// presented with a bare repository name, the Host field will be empty.
func ParseRelative(refStr string) (Reference, error) {
	m := referencePat.FindStringSubmatch(refStr)
	if m == nil {
		return Reference{}, fmt.Errorf("invalid reference syntax")
	}
	var ref Reference
	ref.Host, ref.Repository, ref.Tag, ref.Digest = m[1], m[2], m[3], ociregistry.Digest(m[4])
	// Check lengths and digest: we don't check these as part of the regexp
	// because it's more efficient to do it in Go and we get
	// nicer error messages as a result.
	if len(ref.Tag) > 127 {
		return Reference{}, fmt.Errorf("tag too long")
	}
	if len(ref.Digest) > 0 {
		if err := ref.Digest.Validate(); err != nil {
			return Reference{}, fmt.Errorf("invalid digest: %v", err)
		}
	}
	if len(ref.Repository) > 255 {
		return Reference{}, fmt.Errorf("repository name too long")
	}
	return ref, nil
}

// String returns the string form of a reference in the form
//	[HOST/]NAME[:TAG|@DIGEST]
func (ref Reference) String() string {
	var buf strings.Builder
	buf.Grow(len(ref.Host) + 1 + len(ref.Repository) + 1 + len(ref.Tag) + 1 + len(ref.Digest))
	if ref.Host != "" {
		buf.WriteString(ref.Host)
		buf.WriteByte('/')
	}
	buf.WriteString(ref.Repository)
	if len(ref.Tag) > 0 {
		buf.WriteByte(':')
		buf.WriteString(ref.Tag)
	}
	if len(ref.Digest) > 0 {
		buf.WriteByte('@')
		buf.WriteString(string(ref.Digest))
	}
	return buf.String()
}
