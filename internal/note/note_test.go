package note_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/umars28/cowork-test/internal/note"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name      string
		title     string
		body      string
		wantTitle string
		wantBody  string
		wantErr   error
	}{
		{name: "empty title", title: "", body: "something", wantErr: note.ErrEmptyTitle},
		{name: "whitespace only title", title: "   \t\n ", body: "something", wantErr: note.ErrEmptyTitle},
		{name: "surrounding whitespace trimmed", title: "  shopping  ", body: "\n milk \t", wantTitle: "shopping", wantBody: "milk"},
		{name: "valid pair accepted", title: "shopping", body: "milk", wantTitle: "shopping", wantBody: "milk"},
		{name: "empty body allowed", title: "shopping", body: "", wantTitle: "shopping", wantBody: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			title, body, err := note.Validate(tt.title, tt.body)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Validate(%q, %q) error = %v, want %v", tt.title, tt.body, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate(%q, %q) unexpected error: %v", tt.title, tt.body, err)
			}
			if title != tt.wantTitle {
				t.Errorf("title = %q, want %q", title, tt.wantTitle)
			}
			if body != tt.wantBody {
				t.Errorf("body = %q, want %q", body, tt.wantBody)
			}
		})
	}
}

func TestNoteJSONRoundTrip(t *testing.T) {
	created := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	updated := created.Add(time.Hour)
	in := note.Note{
		ID:        "0123456789abcdef",
		Title:     "shopping",
		Body:      "milk",
		CreatedAt: created,
		UpdatedAt: updated,
	}

	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("Unmarshal into map: %v", err)
	}
	for _, name := range []string{"id", "title", "body", "created_at", "updated_at"} {
		if _, ok := fields[name]; !ok {
			t.Errorf("marshalled note is missing field %q: %s", name, data)
		}
	}
	if len(fields) != 5 {
		t.Errorf("marshalled note has %d fields, want 5: %s", len(fields), data)
	}

	var out note.Note
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("Unmarshal into Note: %v", err)
	}
	if !out.CreatedAt.Equal(in.CreatedAt) || !out.UpdatedAt.Equal(in.UpdatedAt) {
		t.Errorf("timestamps did not round-trip: got %v/%v, want %v/%v", out.CreatedAt, out.UpdatedAt, in.CreatedAt, in.UpdatedAt)
	}
	out.CreatedAt, out.UpdatedAt = in.CreatedAt, in.UpdatedAt
	if out != in {
		t.Errorf("round-trip = %+v, want %+v", out, in)
	}
}

func TestErrNotFoundIsDistinct(t *testing.T) {
	if errors.Is(note.ErrNotFound, note.ErrEmptyTitle) {
		t.Error("ErrNotFound and ErrEmptyTitle must be distinct sentinels")
	}
}
