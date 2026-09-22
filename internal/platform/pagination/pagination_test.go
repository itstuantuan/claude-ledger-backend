package pagination

import (
	"net/url"
	"testing"
)

func TestParseBounds(t *testing.T) {
	params := Parse(url.Values{"page": {"0"}, "pageSize": {"999"}})
	if params.Page != 1 || params.PageSize != 50 || params.Offset() != 0 {
		t.Fatalf("unexpected params: %+v", params)
	}
}
