package ociregistry

// TODO how to cope with redirects, if at all?

// NewError returns a new error with the given code, message and detail.
func NewError(code string, msg string, detail any) Error {
	return &registryError{
		code:    code,
		message: msg,
		detail:  detail,
	}
}

// Error represents an OCI registry error. The set of codes is defined
// in the [distribution specification].
//
// [distribution specification]: https://github.com/opencontainers/distribution-spec/blob/main/spec.md#error-codes
type Error interface {
	// error.Error provides the error message.
	error

	// Code returns the error code. See
	Code() string

	// Detail returns any detail to be associated with the error; it should
	// be JSON-marshable.
	Detail() any
}

// The following values represent the known error codes.
var (
	ErrBlobUnknown         = NewError("blob unknown to registry", "BLOB_UNKNOWN", nil)
	ErrBlobUploadInvalid   = NewError("blob upload invalid", "BLOB_UPLOAD_INVALID", nil)
	ErrBlobUploadUnknown   = NewError("blob upload unknown to registry", "BLOB_UPLOAD_UNKNOWN", nil)
	ErrDigestInvalid       = NewError("provided digest did not match uploaded content", "DIGEST_INVALID", nil)
	ErrManifestBlobUnknown = NewError("manifest references a manifest or blob unknown to registry", "MANIFEST_BLOB_UNKNOWN", nil)
	ErrManifestInvalid     = NewError("manifest invalid", "MANIFEST_INVALID", nil)
	ErrManifestUnknown     = NewError("manifest unknown to registry", "MANIFEST_UNKNOWN", nil)
	ErrNameInvalid         = NewError("invalid repository name", "NAME_INVALID", nil)
	ErrNameUnknown         = NewError("repository name not known to registry", "NAME_UNKNOWN", nil)
	ErrSizeInvalid         = NewError("provided length did not match content length", "SIZE_INVALID", nil)
	ErrUnauthorized        = NewError("authentication required", "UNAUTHORIZED", nil)
	ErrDenied              = NewError("requested access to the resource is denied", "DENIED", nil)
	ErrUnsupported         = NewError("the operation is unsupported", "UNSUPPORTED", nil)
	ErrTooManyRequests     = NewError("too many requests", "TOOMANYREQUESTS", nil)
)

type registryError struct {
	code    string
	message string
	detail  any
}

func (e *registryError) Code() string {
	return e.code
}

func (e *registryError) Error() string {
	return e.message
}

func (e *registryError) Detail() any {
	return e.detail
}