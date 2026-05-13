package domain

import (
	"time"
)

type Tag struct {
	ID        int64
	ProjectID string
	Name      string
	Color     string
	CreatedAt time.Time
}

type TagWithCount struct {
	Tag
	PatentCount int
}

const (
	SortColumnTags = "tags"
)
