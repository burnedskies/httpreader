package httpreader

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

// ErrInvalidStatusCode is returned when a ranged HTTP request does not return
// the expected status code.
type InvalidStatusCodeError struct {
	StatusCode int // StatusCode returned by the server
	Expected   int // Expected status code.
}

func (e *InvalidStatusCodeError) Error() string {
	return fmt.Sprintf("invalid HTTP status code: %d (expected: %d)", e.StatusCode, e.Expected)
}

// InvalidHeaderError is returned when a required HTTP header is missing, malformed,
// or an unexpected value.
type InvalidHeaderError struct {
	Name   string
	Reason string
}

func (e *InvalidHeaderError) Error() string {
	return fmt.Sprintf("invalid header %q: %s", e.Name, e.Reason)
}

type HTTPRangeReader interface {
	io.Seeker
	io.Reader
	io.ReaderAt
}

var _ HTTPRangeReader = &Reader{} // implementation assertion

// Reader implements HTTPRangeReader for an HTTP resource using
// range requests.
type Reader struct {
	resourceURL *url.URL
	// resourceSize is the reported size of the resource. It is determined by
	// the value of the `Content-Range` header returned by the server.
	resourceSize int64
	discardWnd   int
	// The headers returned by the server during reader init.
	initHeader http.Header
	// Primary connection used by Read() and Seek()
	mainResp   *http.Response
	mainOffset int64
	// Temporary connection used by ReadAt() to adhere to the interface description.
	// 	- Clients of ReadAt can execute parallel ReadAt calls on the same
	//    input source
	// 	- If ReadAt is reading from an input source with a seek offset, ReadAt
	//    should not affect nor be affected by the underlying seek offset
	tempResp   *http.Response
	tempOffset int64

	httpClient *http.Client
	httpHeader http.Header

	mu sync.Mutex
}

func (r *Reader) ResourceSize() int64 {
	return r.resourceSize
}

func (r *Reader) ResourceURL() *url.URL {
	u := *r.resourceURL
	return &u
}

func (r *Reader) InitHeader() http.Header {
	return r.initHeader
}

// Seek sets the offset for the next Read to `offset`, interpreted according
// to `whence`
//
// If the requested offset is within the configured discard window, Seek will
// discard data from the response body to reach the new offset. This is done
// to avoid initiating another HTTP request.
func (r *Reader) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		offset = r.mainOffset + offset
	case io.SeekEnd:
		offset = r.resourceSize + offset
	}

	if offset >= r.resourceSize {
		return 0, errors.New("seek beyond end of resource")
	}
	if offset < 0 {
		return 0, errors.New("seek before beginning of resource")
	}

	distance := offset - r.mainOffset
	if distance >= 0 && distance <= int64(r.discardWnd) {
		// Forward seek within the discard window
		n, err := io.CopyN(io.Discard, r, distance)
		if err != nil {
			return 0, err
		}
		if n != distance {
			return 0, errors.New("skip data error")
		}
	} else {
		// Backward seek OR forward seek beyond discard window
		if err := r.request(offset); err != nil {
			return 0, err
		}
		r.mainOffset = offset
	}
	return offset, nil
}

func (r *Reader) Read(p []byte) (int, error) {
	if r.mainOffset >= r.resourceSize {
		return 0, io.EOF
	}
	if r.mainResp == nil {
		err := r.request(r.mainOffset)
		if err != nil {
			return 0, err
		}
	}
	n, err := r.mainResp.Body.Read(p)
	r.mainOffset += int64(n)
	return n, err
}

func (r *Reader) ReadAt(p []byte, offset int64) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// If the current body is empty, the new offset is behind the current offset, or
	// the new offset is too far ahead of the current offset.
	if r.tempResp == nil ||
		offset < r.tempOffset ||
		offset >= r.tempOffset+int64(r.discardWnd) {

		if r.tempResp != nil {
			// The new offset is behind or too far forward. Close the current response
			// body before we initiate a new request
			r.tempResp.Body.Close()
		}
		req, err := newRangeRequest(
			r.resourceURL,
			r.httpHeader,
			&httpRange{start: offset, end: -1},
		)
		if err != nil {
			return 0, err
		}

		res, err := r.httpClient.Do(req)
		if err != nil {
			return 0, err
		}
		r.tempOffset = offset
		r.tempResp = res
	}

	if r.tempOffset < offset {
		// At this point tempOffset should either equal `offset` or be within the
		// set discard window.
		n, err := io.CopyN(io.Discard, r.tempResp.Body, offset-r.tempOffset)
		if err != nil {
			return 0, err
		}
		if n+r.tempOffset != offset {
			return 0, errors.New("skip data error")
		}
		r.tempOffset = offset
	}
	n, err := r.tempResp.Body.Read(p)
	r.tempOffset += int64(n)
	return n, err
}

func (r *Reader) request(offset int64) error {
	if r.mainResp != nil {
		r.mainResp.Body.Close()
	}

	req, err := newRangeRequest(
		r.resourceURL,
		r.httpHeader,
		&httpRange{start: offset, end: -1},
	)
	if err != nil {
		return err
	}

	res, err := r.httpClient.Do(req)
	if err != nil {
		return err
	}

	if err = isRangeResponse(res); err != nil {
		res.Body.Close()
		return err
	}
	r.mainOffset = offset
	r.mainResp = res
	return err
}

func (r *Reader) init() error {
	// initial request to determine if the server supports range requests
	// as well as the size of the requested resource.
	req, err := newRangeRequest(
		r.resourceURL,
		r.httpHeader,
		&httpRange{start: 0, end: 511},
	)
	if err != nil {
		return err
	}

	res, err := r.httpClient.Do(req)
	if err != nil {
		return err
	}
	res.Body.Close()

	if err = isRangeResponse(res); err != nil {
		return err
	}
	contentRange := strings.Split(res.Header.Get("Content-Range"), "/")
	if len(contentRange) == 2 {
		size, err := strconv.ParseInt(contentRange[1], 10, 64)
		if err != nil {
			return &InvalidHeaderError{
				Name:   "Content-Range",
				Reason: fmt.Sprintf("failed to parse resource size: %v", err),
			}
		}
		r.resourceSize = size
	} else {
		return &InvalidHeaderError{
			Name: "Content-Range",
			Reason: fmt.Sprintf(
				"failed to parse value: %q", res.Header.Get("Content-Range")),
		}
	}
	return err
}

func isRangeResponse(res *http.Response) error {
	if res.StatusCode != http.StatusPartialContent {
		return &InvalidStatusCodeError{res.StatusCode, http.StatusPartialContent}
	}
	if ar := res.Header.Get("Accept-Ranges"); ar != "bytes" {
		return &InvalidHeaderError{
			Name: "Accept-Ranges",
			Reason: fmt.Sprintf(
				"server (%s) returned unexpected value: %q (expected: `bytes`)",
				res.Request.URL.Host,
				ar),
		}
	}
	return nil
}

var defaultHTTPClient = &http.Client{
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
	},
}

func NewReader(u *url.URL, options ...Option) (*Reader, error) {
	reader := &Reader{
		resourceURL: u,
		httpClient:  defaultHTTPClient,
		discardWnd:  1024 * 512,
	}

	for _, option := range options {
		if err := option(reader); err != nil {
			return nil, err
		}
	}
	return reader, reader.init()
}
