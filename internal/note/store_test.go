package note_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/umars28/cowork-test/internal/note"
)

func openStore(t *testing.T) (*note.Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "notes.json")
	st, err := note.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return st, path
}

func seed(t *testing.T, notes []note.Note) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "notes.json")
	data, err := json.Marshal(struct {
		Version int         `json:"version"`
		Notes   []note.Note `json:"notes"`
	}{Version: 1, Notes: notes})
	if err != nil {
		t.Fatalf("Marshal seed: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile seed: %v", err)
	}
	return path
}

func TestOpenMissingFileIsEmpty(t *testing.T) {
	st, _ := openStore(t)
	if got := st.List(); len(got) != 0 {
		t.Fatalf("List() on a fresh store = %d notes, want 0", len(got))
	}
}

func TestOpenMalformedJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := note.Open(path); err == nil {
		t.Fatal("Open on malformed JSON returned nil error, want an error")
	}
}

func TestCRUDRoundTrip(t *testing.T) {
	st, _ := openStore(t)

	created, err := st.Create("  shopping  ", "  milk  ")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Title != "shopping" || created.Body != "milk" {
		t.Errorf("Create did not trim: %+v", created)
	}
	if matched, _ := regexp.MatchString(`^[0-9a-f]{16}$`, created.ID); !matched {
		t.Errorf("ID = %q, want 16 hex characters", created.ID)
	}
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Errorf("Create left a zero timestamp: %+v", created)
	}

	got, err := st.Get(created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != created {
		t.Errorf("Get = %+v, want %+v", got, created)
	}

	updated, err := st.Update(created.ID, "groceries", "milk and eggs")
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Title != "groceries" || updated.Body != "milk and eggs" {
		t.Errorf("Update = %+v, want the new title and body", updated)
	}
	if !updated.CreatedAt.Equal(created.CreatedAt) {
		t.Errorf("Update changed CreatedAt: %v, want %v", updated.CreatedAt, created.CreatedAt)
	}
	if updated.UpdatedAt.Before(created.UpdatedAt) {
		t.Errorf("Update moved UpdatedAt backwards: %v, want >= %v", updated.UpdatedAt, created.UpdatedAt)
	}

	if err := st.Delete(created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := st.Get(created.ID); !errors.Is(err, note.ErrNotFound) {
		t.Errorf("Get after Delete error = %v, want ErrNotFound", err)
	}
	if got := st.List(); len(got) != 0 {
		t.Errorf("List after Delete = %d notes, want 0", len(got))
	}
}

func TestUnknownIDIsNotFound(t *testing.T) {
	st, _ := openStore(t)

	if _, err := st.Get("ffffffffffffffff"); !errors.Is(err, note.ErrNotFound) {
		t.Errorf("Get error = %v, want ErrNotFound", err)
	}
	if _, err := st.Update("ffffffffffffffff", "title", "body"); !errors.Is(err, note.ErrNotFound) {
		t.Errorf("Update error = %v, want ErrNotFound", err)
	}
	if err := st.Delete("ffffffffffffffff"); !errors.Is(err, note.ErrNotFound) {
		t.Errorf("Delete error = %v, want ErrNotFound", err)
	}
}

func TestCreateAndUpdateValidate(t *testing.T) {
	st, _ := openStore(t)

	if _, err := st.Create("   ", "body"); !errors.Is(err, note.ErrEmptyTitle) {
		t.Errorf("Create with blank title error = %v, want ErrEmptyTitle", err)
	}
	if got := st.List(); len(got) != 0 {
		t.Errorf("a rejected Create stored %d notes, want 0", len(got))
	}

	created, err := st.Create("shopping", "milk")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := st.Update(created.ID, "  ", "body"); !errors.Is(err, note.ErrEmptyTitle) {
		t.Errorf("Update with blank title error = %v, want ErrEmptyTitle", err)
	}
	got, err := st.Get(created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Title != "shopping" {
		t.Errorf("a rejected Update changed the note: %+v", got)
	}
}

func TestReopenSeesPreviousWrites(t *testing.T) {
	st, path := openStore(t)

	first, err := st.Create("shopping", "milk")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	second, err := st.Create("todo", "write tests")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := st.Delete(first.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	reopened, err := note.Open(path)
	if err != nil {
		t.Fatalf("Open again: %v", err)
	}
	got := reopened.List()
	if len(got) != 1 {
		t.Fatalf("reopened List = %d notes, want 1", len(got))
	}
	if got[0].ID != second.ID || got[0].Title != "todo" || got[0].Body != "write tests" {
		t.Errorf("reopened note = %+v, want %+v", got[0], second)
	}
	if !got[0].CreatedAt.Equal(second.CreatedAt) {
		t.Errorf("reopened CreatedAt = %v, want %v", got[0].CreatedAt, second.CreatedAt)
	}
}

func TestListOrder(t *testing.T) {
	older := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)
	path := seed(t, []note.Note{
		{ID: "bbbbbbbbbbbbbbbb", Title: "b", CreatedAt: older, UpdatedAt: older},
		{ID: "cccccccccccccccc", Title: "c", CreatedAt: newer, UpdatedAt: newer},
		{ID: "aaaaaaaaaaaaaaaa", Title: "a", CreatedAt: newer, UpdatedAt: newer},
	})

	st, err := note.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	want := []string{"aaaaaaaaaaaaaaaa", "cccccccccccccccc", "bbbbbbbbbbbbbbbb"}
	got := st.List()
	if len(got) != len(want) {
		t.Fatalf("List = %d notes, want %d", len(got), len(want))
	}
	for i, id := range want {
		if got[i].ID != id {
			t.Errorf("List[%d].ID = %q, want %q", i, got[i].ID, id)
		}
	}
}

func TestListIsACopy(t *testing.T) {
	st, _ := openStore(t)
	if _, err := st.Create("shopping", "milk"); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got := st.List()
	got[0].Title = "mutated"

	if again := st.List(); again[0].Title != "shopping" {
		t.Errorf("mutating the List result changed the store: %q", again[0].Title)
	}
}

func TestConcurrentCreate(t *testing.T) {
	st, path := openStore(t)

	const writers = 50
	var wg sync.WaitGroup
	wg.Add(writers)
	errs := make(chan error, writers)
	for i := range writers {
		go func() {
			defer wg.Done()
			if _, err := st.Create(fmt.Sprintf("note %d", i), "body"); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent Create: %v", err)
	}

	if got := st.List(); len(got) != writers {
		t.Errorf("List = %d notes, want %d", len(got), writers)
	}

	reopened, err := note.Open(path)
	if err != nil {
		t.Fatalf("Open after concurrent writes: %v", err)
	}
	persisted := reopened.List()
	if len(persisted) != writers {
		t.Fatalf("persisted %d notes, want %d", len(persisted), writers)
	}
	titles := make(map[string]bool, writers)
	ids := make(map[string]bool, writers)
	for _, n := range persisted {
		titles[n.Title] = true
		ids[n.ID] = true
	}
	if len(ids) != writers {
		t.Errorf("got %d distinct ids, want %d", len(ids), writers)
	}
	for i := range writers {
		if !titles[fmt.Sprintf("note %d", i)] {
			t.Errorf("note %d is missing from the persisted file", i)
		}
	}
}
