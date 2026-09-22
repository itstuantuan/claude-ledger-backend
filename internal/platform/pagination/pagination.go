package pagination

import (
	"net/url"
	"strconv"
)

type Params struct {
	Page     int
	PageSize int
}
type Page[T any] struct {
	Items    []T   `json:"items"`
	Page     int   `json:"page"`
	PageSize int   `json:"pageSize"`
	Total    int64 `json:"total"`
}

func Parse(values url.Values) Params {
	page, _ := strconv.Atoi(values.Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(values.Get("pageSize"))
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 50 {
		pageSize = 50
	}
	return Params{Page: page, PageSize: pageSize}
}
func (p Params) Offset() int { return (p.Page - 1) * p.PageSize }
