package ociclient

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"unicode"

	"github.com/rogpeppe/ociregistry"
)

// errorBodySizeLimit holds the maximum number of response bytes aallowed in
// the server's error response. A typical error message is around 200
// bytes. Hence, 8 KiB should be sufficient.
const errorBodySizeLimit = 8 * 1024

type wireError struct {
	Code    string          `json:"code"`
	Message string          `json:"message,omitempty"`
	Detail  json.RawMessage `json:"detail,omitempty"`
}

func (e *wireError) Error() string {
	var buf strings.Builder
	for _, r := range e.Code {
		if r == '_' {
			buf.WriteByte(' ')
		} else {
			buf.WriteRune(unicode.ToLower(r))
		}
	}
	if buf.Len() == 0 {
		buf.WriteString("(no code)")
	}
	buf.WriteString(": ")
	if e.Message != "" {
		buf.WriteString(": ")
		buf.WriteString(e.Message)
	}
	if len(e.Detail) != 0 && !bytes.Equal(e.Detail, []byte("null")) {
		buf.WriteString("; detail: ")
		buf.Write(e.Detail)
	}
	return buf.String()
}

// Is makes it possible for users to write `if errors.Is(err, ociregistry.ErrBlobUnknown)`
// even when the error hasn't exactly wrapped that error.
func (e *wireError) Is(err error) bool {
	var rerr ociregistry.Error
	return errors.As(err, &rerr) && rerr.Code() == e.Code
}

type wireErrors struct {
	httpStatus string
	Errors     []wireError `json:"errors"`
}

func (e *wireErrors) Unwrap() []error {
	// TODO we could do this only once.
	errs := make([]error, len(e.Errors))
	for i := range e.Errors {
		errs[i] = &e.Errors[i]
	}
	return errs
}

func (e *wireErrors) Error() string {
	var buf strings.Builder
	buf.WriteString(e.httpStatus)
	buf.WriteString(": ")
	buf.WriteString(e.Errors[0].Error())
	for i := range e.Errors[1:] {
		buf.WriteString("; ")
		buf.WriteString(e.Errors[i+1].Error())
	}
	return buf.String()
}

// makeError forms an error from a non-OK response.
func makeError(resp *http.Response) error {
	if !isJSONMediaType(resp.Header.Get("Content-Type")) {
		// TODO include some of the body in this case?
		return errors.New(resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, errorBodySizeLimit+1))
	if err != nil {
		return fmt.Errorf("%s: cannot read error body: %v", resp.Status, err)
	}
	if len(data) > errorBodySizeLimit {
		// TODO include some part of the body
		return fmt.Errorf("error body too large")
	}
	var errs wireErrors
	if err := json.Unmarshal(data, &errs); err != nil {
		return fmt.Errorf("%s: malformed error response: %v", resp.Status, err)
	}
	if len(errs.Errors) == 0 {
		return fmt.Errorf("%s: no errors in body (probably a server issue)", resp.Status)
	}
	errs.httpStatus = resp.Status
	return &errs
}

// isJSONMediaType reports whether the content type implies
// that the content is JSON.
func isJSONMediaType(contentType string) bool {
	mediaType, _, _ := mime.ParseMediaType(contentType)
	m := strings.TrimPrefix(mediaType, "application/")
	if len(m) == len(mediaType) {
		return false
	}
	// Look for +json suffix. See https://tools.ietf.org/html/rfc6838#section-4.2.8
	// We recognize multiple suffixes too (e.g. application/something+json+other)
	// as that seems to be a possibility.
	for {
		i := strings.Index(m, "+")
		if i == -1 {
			return m == "json"
		}
		if m[0:i] == "json" {
			return true
		}
		m = m[i+1:]
	}
}
