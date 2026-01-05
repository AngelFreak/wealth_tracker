// Package repository provides data access layer for the wealth tracker.
package repository

// Pagination holds pagination parameters.
type Pagination struct {
	Limit  int
	Offset int
}

// PaginatedResult wraps results with pagination info.
type PaginatedResult[T any] struct {
	Items      []T   `json:"items"`
	Total      int64 `json:"total"`
	Limit      int   `json:"limit"`
	Offset     int   `json:"offset"`
	HasMore    bool  `json:"has_more"`
	TotalPages int   `json:"total_pages"`
	Page       int   `json:"page"`
}

// DefaultLimit is the default number of items per page.
const DefaultLimit = 50

// MaxLimit is the maximum allowed items per page.
const MaxLimit = 500

// NewPagination creates pagination with validated limits.
func NewPagination(limit, offset int) Pagination {
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}
	if offset < 0 {
		offset = 0
	}
	return Pagination{Limit: limit, Offset: offset}
}

// PageToPagination converts page number to offset-based pagination.
func PageToPagination(page, perPage int) Pagination {
	if page < 1 {
		page = 1
	}
	if perPage <= 0 {
		perPage = DefaultLimit
	}
	if perPage > MaxLimit {
		perPage = MaxLimit
	}
	return Pagination{
		Limit:  perPage,
		Offset: (page - 1) * perPage,
	}
}

// NewPaginatedResult creates a paginated result.
func NewPaginatedResult[T any](items []T, total int64, p Pagination) PaginatedResult[T] {
	totalPages := int(total) / p.Limit
	if int(total)%p.Limit > 0 {
		totalPages++
	}

	page := (p.Offset / p.Limit) + 1
	hasMore := p.Offset+len(items) < int(total)

	return PaginatedResult[T]{
		Items:      items,
		Total:      total,
		Limit:      p.Limit,
		Offset:     p.Offset,
		HasMore:    hasMore,
		TotalPages: totalPages,
		Page:       page,
	}
}
