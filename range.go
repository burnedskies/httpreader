package httpreader

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

/*
httpRange represents a range of content bytes that can be used in HTTP
responses to specify partial content retrieval.

Example:

	httpRange{start: 10, end: 20} // Represents "Range: bytes=10-20"
	httpRange{start: 5, end: -1}  // Represents "Range: bytes=5-"
	httpRange{start: -1, end: 15} // Represents "Range: bytes=-15"
*/
type httpRange struct {
	start int64
	end   int64
}

/*
The String method converts the httpRange into a string that follows the
HTTP range header format. In cases where the start or end are invalid or
the range is not well-defined, String() returns an empty string.
*/
func (r httpRange) String() string {
	if r.start < 0 && r.end >= 0 {
		return fmt.Sprintf("-%d", r.end) // -<suffix-length>
	}
	if r.start >= 0 && r.end < 0 {
		return fmt.Sprintf("%d-", r.start) // <start>-
	}
	if r.start >= 0 && r.end >= 0 && r.start <= r.end {
		return fmt.Sprintf("%d-%d", r.start, r.end) // <start>-<end>
	}
	return ""
}

func setRangeHeader(header http.Header, ranges ...*httpRange) {
	var r []string
	for _, httpRange := range ranges {
		if s := httpRange.String(); s != "" {
			r = append(r, s)
		}
	}
	header.Set("Range", "bytes="+strings.Join(r, ","))
}

func newRangeRequest(u *url.URL, headers http.Header, ranges ...*httpRange) (*http.Request, error) {
	if req, err := http.NewRequest(http.MethodGet, u.String(), nil); err == nil {
		req.Header = headers.Clone()
		setRangeHeader(req.Header, ranges...)

		return req, err
	} else {
		return nil, err
	}
}
