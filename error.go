package ociregistry

// TODO how to cope with redirects, if at all?

type registryError struct {
	Code_   string `json:"code"`
	Message string `json:"message"`
	Detail  any    `json:"detail"`
	// httpStatusCode holds the conventional HTTP status code
	// associated with the error.
	httpCode int `json:"-"`
}

func (e *registryError) Code() string {
	return e.Code_
}

func (e *registryError) Error() string {
	return e.Message
}

func NewError(code string, msg string, detail any) Error {
	return &registryError{
		Code_:   code,
		Message: msg,
		Detail:  detail,
	}
}

type Error interface {
	error
	// Code returns the error code for the error.
	Code() string
}

type ErrorCode struct {
	code string
}

func newErrorCode(code string, msg string, httpCode int) Error {
	return &registryError{
		Code_:    code,
		Message:  msg,
		httpCode: httpCode,
	}
}

// The following errors correspond to error codes in the API.
// See https://github.com/opencontainers/distribution-spec/blob/main/spec.md#error-codes
//
var (
	ErrBlobUnknown         = newErrorCode("blob unknown to registry", "BLOB_UNKNOWN", 404)
	ErrBlobUploadInvalid   = newErrorCode("blob upload invalid", "BLOB_UPLOAD_INVALID", 400)
	ErrBlobUploadUnknown   = newErrorCode("blob upload unknown to registry", "BLOB_UPLOAD_UNKNOWN", 404)
	ErrDigestInvalid       = newErrorCode("provided digest did not match uploaded content", "DIGEST_INVALID", 400)
	ErrManifestBlobUnknown = newErrorCode("manifest references a manifest or blob unknown to registry", "MANIFEST_BLOB_UNKNOWN", 404)
	ErrManifestInvalid     = newErrorCode("manifest invalid", "MANIFEST_INVALID", 400)
	ErrManifestUnknown     = newErrorCode("manifest unknown to registry", "MANIFEST_UNKNOWN", 404)
	ErrNameInvalid         = newErrorCode("invalid repository name", "NAME_INVALID", 400)
	ErrNameUnknown         = newErrorCode("repository name not known to registry", "NAME_UNKNOWN", 404)
	ErrSizeInvalid         = newErrorCode("provided length did not match content length", "SIZE_INVALID", 400)
	ErrUnauthorized        = newErrorCode("authentication required", "UNAUTHORIZED", 401)
	ErrDenied              = newErrorCode("requested access to the resource is denied", "DENIED", 403)
	ErrUnsupported         = newErrorCode("the operation is unsupported", "UNSUPPORTED", 400)
	ErrTooManyRequests     = newErrorCode("too many requests", "TOOMANYREQUESTS", 429)
)
