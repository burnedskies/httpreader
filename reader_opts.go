package httpreader

import "net/http"

type Option func(*Reader) error // functional option pattern

type Options interface {
	WithClient(*http.Client) Option
	WithHeaders(http.Header) Option
}

func WithClient(client *http.Client) Option {
	return func(r *Reader) error {
		if client == nil {
			client = defaultHTTPClient
		}
		r.httpClient = client
		return nil
	}
}

// WithHeaders sets the default HTTP headers that will be sent in all
// requests made by the Reader.
func WithHeaders(header http.Header) Option {
	return func(r *Reader) error {
		r.httpHeaders = header.Clone()
		return nil
	}
}
