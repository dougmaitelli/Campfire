// Package pagination defines the shared numbered-page contract for history APIs.
package pagination

import (
	"errors"
	"net/url"
	"strconv"

	"gorm.io/gorm"
)

// DefaultSize is the common history page size.
const DefaultSize = 20

// MaxSize bounds a single response without hiding collection totals.
const MaxSize = 100

// Request is validated before any database work. Pages are one-based.
type Request struct {
	Page int
	Size int
}

// Metadata describes the entire authorized, filtered collection, not just this page.
type Metadata struct {
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	TotalItems int64 `json:"total_items"`
	TotalPages int64 `json:"total_pages"`
}

// Page is the shared response envelope for all paginated history endpoints.
type Page[T any] struct {
	Items      []T      `json:"items"`
	Pagination Metadata `json:"pagination"`
}

// Parse rejects obsolete cursors and invalid sizes instead of silently ignoring
// them or truncating a caller's requested page size.
func Parse(values url.Values) (Request, error) {
	request := Request{Page: 1, Size: DefaultSize}

	for _, key := range []string{"before", "limit", "cursor"} {
		if values.Has(key) {
			return request, errors.New("use page and page_size for pagination")
		}
	}

	for key, target := range map[string]*int{"page": &request.Page, "page_size": &request.Size} {
		if raw, ok := values[key]; ok {
			if len(raw) != 1 {
				return request, errors.New("use each pagination parameter once")
			}

			value, err := strconv.Atoi(raw[0])
			if err != nil || value < 1 {
				return request, errors.New("page and page_size must be positive integers")
			}

			*target = value
		}
	}
	// Bound the offset to a portable signed SQL integer, without multiplying first.
	if request.Size > MaxSize || request.Page-1 > 2147483647/request.Size {
		return request, errors.New("page_size must be at most 100 and page offset at most 2147483647")
	}

	return request, nil
}

// New constructs page metadata and normalizes an empty collection to an array.
func New[T any](items []T, request Request, total int64) Page[T] {
	if items == nil {
		items = []T{}
	}

	return Page[T]{Items: items, Pagination: Metadata{Page: request.Page, PageSize: request.Size, TotalItems: total, TotalPages: (total + int64(request.Size) - 1) / int64(request.Size)}}
}

// Read counts and pages the same pre-authorized query. Find preserves preloads
// (notably event attachments). Callers supply a deterministic unique ordering.
// Totals reflect live data; inserts/deletes can shift numbered page boundaries.
func Read[T any](query *gorm.DB, request Request) (Page[T], error) {
	var total int64

	count := query.Session(&gorm.Session{}).Limit(-1).Offset(-1)
	// Limit clones the statement, so removing the count's ordering does not
	// mutate the original page query. Sorting cannot affect the total.
	delete(count.Statement.Clauses, "ORDER BY")

	if err := query.Session(&gorm.Session{NewDB: true}).Table("(?) AS page_count", count).Count(&total).Error; err != nil {
		return New[T](nil, request, 0), err
	}

	items := []T{}
	err := query.Offset((request.Page - 1) * request.Size).Limit(request.Size).Find(&items).Error

	return New(items, request, total), err
}
