package httpreader

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type HTTPRangeReader interface {
	io.Seeker
	io.Reader
	io.ReaderAt
}

var _ HTTPRangeReader = &Reader{} // implementation assertion

// Reader provides random‑access reads over an HTTP resource. It issues
// ranged GET requests as needed and tracks the remote file’s size and
// current read offset.
type Reader struct {
	resourceURL *url.URL
	// resourceSize is the total size of the resource in bytes. It is calculated
	// from the `Content-Range` header returned by the server during reader init.
	resourceSize int64
	offset       int64

	httpHeaders http.Header
	httpClient  *http.Client
}

func (r *Reader) Tell() int64 {
	return r.offset
}

func (r *Reader) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	default:
		return 0, errors.New("Seek: invalid whence")
	case io.SeekStart:
		offset += 0
	case io.SeekCurrent:
		offset += r.offset
	case io.SeekEnd:
		offset += r.resourceSize
	}
	if offset < 0 || offset > r.resourceSize {
		return 0, errors.New("Seek: invalid offset")
	}
	r.offset = offset
	return offset, nil
}

func (r *Reader) read(buf []byte, offset int64) (int, error) {
	if offset >= r.resourceSize {
		return 0, io.EOF
	}

	req, err := newRangeRequest(r.resourceURL.String(), r.httpHeaders, &httpRange{
		start: offset,
		end:   offset + int64(len(buf)) - 1,
	})
	if err != nil {
		return 0, fmt.Errorf("read: %v", err)
	}

	res, err := r.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()

	err = validateServerResponse(res)
	if err != nil {
		return 0, fmt.Errorf("read server err: %v", err)
	}

	// return io.ReadAtLeast(res.Body, buf, len(buf))
	br, err := io.ReadAtLeast(res.Body, buf, len(buf))
	if errors.Is(err, io.ErrUnexpectedEOF) {
		// The server returned fewer bytes than requested. Treat it as a
		// normal EOF.
		return br, io.EOF
	}
	return br, err
}

func (r *Reader) Read(buf []byte) (br int, err error) {
	if r.offset >= r.resourceSize {
		return 0, io.EOF
	}

	br, err = r.read(buf, r.offset)
	r.offset += int64(br)
	return
}

func (r *Reader) ReadAt(buf []byte, offset int64) (br int, err error) {
	if offset < 0 || offset >= r.resourceSize {
		return 0, io.EOF
	}

	return r.read(buf, offset)
}

func (r *Reader) ResourceSize() int64 {
	return r.resourceSize
}

func (r *Reader) ResourceURL() *url.URL {
	u := *r.resourceURL
	return &u
}

func (r *Reader) init() (err error) {
	var req *http.Request
	var res *http.Response

	if req, err = newRangeRequest(
		r.resourceURL.String(), r.httpHeaders, &httpRange{start: 0, end: 1024},
	); err != nil {
		return
	}

	if res, err = r.httpClient.Do(req); err != nil {
		return
	}
	defer res.Body.Close()

	if err = validateServerResponse(res); err != nil {
		return
	}

	contentRange := strings.Split(res.Header.Get("Content-Range"), "/")
	if len(contentRange) == 2 {
		contentSize, err := strconv.ParseInt(contentRange[1], 10, 64)
		if err != nil {
			return fmt.Errorf("failed determine resource size from content-range: %v", err)
		}
		r.resourceSize = contentSize
	} else {
		return fmt.Errorf(
			"failed to parse content-range: \"%s\"", res.Header.Get("Content-Range"),
		)
	}
	return
}

func validateServerResponse(res *http.Response) error {
	if res.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("server returned an unexpected status code (%d)", res.StatusCode)
	}

	if res.Header.Get("Accept-Ranges") != "bytes" {
		return fmt.Errorf("%s does not appear to support byte-ranged requests", res.Request.URL.Host)
	}

	return nil
}

var defaultHTTPClient = &http.Client{
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
	},
}

func NewReader(resource string, options ...Option) (*Reader, error) {
	resourceURL, err := url.Parse(resource)
	if err != nil {
		return nil, err
	}

	reader := &Reader{
		resourceURL: resourceURL,
		httpClient:  defaultHTTPClient,
		httpHeaders: http.Header{
			"User-Agent": {"httpreader"},
		},
	}

	for _, option := range options {
		if err := option(reader); err != nil {
			return nil, err
		}
	}
	return reader, reader.init()
}
