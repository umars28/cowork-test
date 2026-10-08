package note

import (
	"errors"
	"strings"
	"time"
)

type Note struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

var (
	ErrNotFound   = errors.New("note not found")
	ErrEmptyTitle = errors.New("title must not be empty")
)

func Validate(title, body string) (string, string, error) {
	title = strings.TrimSpace(title)
	body = strings.TrimSpace(body)
	if title == "" {
		return "", "", ErrEmptyTitle
	}
	return title, body, nil
}
