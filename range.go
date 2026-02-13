package httpreader

import (
	"fmt"
	"net/http"
	"strings"
)

/*
httpRange represents a range of content bytes that can be used in HTTP
responses to specify partial content retrieval.
*/
type httpRange struct {
	start int64
	end   int64
}

/*
The String method converts the httpRange into a string that follows the
HTTP range header format.

Example:

	r1 := httpRange{start: 10, end: 20} // Represents bytes 10 to 20
	fmt.Println(r1.String()) // Output: "10-20"

	r2 := httpRange{start: 5, end: -1} // Represents bytes starting from 5 to the end
	fmt.Println(r2.String()) // Output: "5-"

	r3 := httpRange{start: -1, end: 15} // Represents the last 15 bytes
	fmt.Println(r3.String()) // Output: "-15"

In cases where the start or end are invalid or the range is not well-defined,
the String method returns an empty string.
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

func newRangeRequest(url string, headers http.Header, ranges ...*httpRange) (*http.Request, error) {
	if req, err := http.NewRequest(http.MethodGet, url, nil); err == nil {
		req.Header = headers.Clone()
		setRangeHeader(req.Header, ranges...)

		return req, err
	} else {
		return nil, err
	}
}
