package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/umars28/cowork-test/internal/note"
)

func post(t *testing.T, h http.Handler, target string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, target, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func seedNote(t *testing.T, st *note.Store, title, body string) note.Note {
	t.Helper()
	n, err := st.Create(title, body)
	if err != nil {
		t.Fatalf("seed Create: %v", err)
	}
	return n
}

func assertHTML(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
}

func assertContains(t *testing.T, w *httptest.ResponseRecorder, want string) {
	t.Helper()
	if !strings.Contains(w.Body.String(), want) {
		t.Errorf("body does not contain %q:\n%s", want, w.Body.String())
	}
}

func TestPageRoot(t *testing.T) {
	h := New(newTestStore(t))

	w := do(t, h, http.MethodGet, "/", "")
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusSeeOther)
	}
	if got := w.Header().Get("Location"); got != "/notes" {
		t.Errorf("Location = %q, want %q", got, "/notes")
	}
}

func TestPageList(t *testing.T) {
	st := newTestStore(t)
	h := New(st)

	w := do(t, h, http.MethodGet, "/notes", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	assertHTML(t, w)
	assertContains(t, w, "/notes/new")

	seedNote(t, st, "shopping", "milk")
	seedNote(t, st, "todo", "write tests")

	w = do(t, h, http.MethodGet, "/notes", "")
	assertContains(t, w, "shopping")
	assertContains(t, w, "todo")
}

func TestPageNewForm(t *testing.T) {
	h := New(newTestStore(t))

	w := do(t, h, http.MethodGet, "/notes/new", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	assertHTML(t, w)
	assertContains(t, w, `method="post"`)
	assertContains(t, w, `action="/notes"`)
	assertContains(t, w, `name="title"`)
	assertContains(t, w, `name="body"`)
}

func TestPageCreate(t *testing.T) {
	st := newTestStore(t)
	h := New(st)

	w := post(t, h, "/notes", url.Values{"title": {"shopping"}, "body": {"milk"}})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d:\n%s", w.Code, http.StatusSeeOther, w.Body.String())
	}
	notes := st.List()
	if len(notes) != 1 {
		t.Fatalf("store holds %d notes, want 1", len(notes))
	}
	if want := "/notes/" + notes[0].ID; w.Header().Get("Location") != want {
		t.Errorf("Location = %q, want %q", w.Header().Get("Location"), want)
	}
}

func TestPageCreateValidationFailure(t *testing.T) {
	st := newTestStore(t)
	h := New(st)

	w := post(t, h, "/notes", url.Values{"title": {"   "}, "body": {"milk"}})
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnprocessableEntity)
	}
	assertHTML(t, w)
	assertContains(t, w, note.ErrEmptyTitle.Error())
	assertContains(t, w, "milk")
	if got := st.List(); len(got) != 0 {
		t.Errorf("store holds %d notes, want 0", len(got))
	}
}

func TestPageDetail(t *testing.T) {
	st := newTestStore(t)
	h := New(st)
	n := seedNote(t, st, "shopping", "milk and eggs")

	w := do(t, h, http.MethodGet, "/notes/"+n.ID, "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	assertHTML(t, w)
	assertContains(t, w, "shopping")
	assertContains(t, w, "milk and eggs")
	assertContains(t, w, "/notes/"+n.ID+"/edit")
	assertContains(t, w, "/notes/"+n.ID+"/delete")
}

func TestPageDetailUnknown(t *testing.T) {
	h := New(newTestStore(t))

	w := do(t, h, http.MethodGet, "/notes/ffffffffffffffff", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
	assertHTML(t, w)
}

func TestPageEditForm(t *testing.T) {
	st := newTestStore(t)
	h := New(st)
	n := seedNote(t, st, "shopping", "milk")

	w := do(t, h, http.MethodGet, "/notes/"+n.ID+"/edit", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	assertHTML(t, w)
	assertContains(t, w, `action="/notes/`+n.ID+`/edit"`)
	assertContains(t, w, "shopping")
	assertContains(t, w, "milk")
}

func TestPageUpdate(t *testing.T) {
	st := newTestStore(t)
	h := New(st)
	n := seedNote(t, st, "shopping", "milk")

	w := post(t, h, "/notes/"+n.ID+"/edit", url.Values{"title": {"groceries"}, "body": {"milk and eggs"}})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d:\n%s", w.Code, http.StatusSeeOther, w.Body.String())
	}
	if want := "/notes/" + n.ID; w.Header().Get("Location") != want {
		t.Errorf("Location = %q, want %q", w.Header().Get("Location"), want)
	}

	got, err := st.Get(n.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Title != "groceries" || got.Body != "milk and eggs" {
		t.Errorf("stored note = %+v, want the updated fields", got)
	}
}

func TestPageUpdateValidationFailure(t *testing.T) {
	st := newTestStore(t)
	h := New(st)
	n := seedNote(t, st, "shopping", "milk")

	w := post(t, h, "/notes/"+n.ID+"/edit", url.Values{"title": {""}, "body": {"milk"}})
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnprocessableEntity)
	}
	assertHTML(t, w)
	assertContains(t, w, note.ErrEmptyTitle.Error())

	got, err := st.Get(n.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Title != "shopping" {
		t.Errorf("a rejected update changed the note: %+v", got)
	}
}

func TestPageDelete(t *testing.T) {
	st := newTestStore(t)
	h := New(st)
	n := seedNote(t, st, "shopping", "milk")

	w := post(t, h, "/notes/"+n.ID+"/delete", nil)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d:\n%s", w.Code, http.StatusSeeOther, w.Body.String())
	}
	if got := w.Header().Get("Location"); got != "/notes" {
		t.Errorf("Location = %q, want %q", got, "/notes")
	}
	if got := st.List(); len(got) != 0 {
		t.Errorf("store holds %d notes, want 0", len(got))
	}
}

func TestPageDeleteUnknown(t *testing.T) {
	h := New(newTestStore(t))

	w := post(t, h, "/notes/ffffffffffffffff/delete", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
	assertHTML(t, w)
}

func TestPageEscapesContent(t *testing.T) {
	st := newTestStore(t)
	h := New(st)
	n := seedNote(t, st, "<script>alert(1)</script>", "body")

	w := do(t, h, http.MethodGet, "/notes/"+n.ID, "")
	if strings.Contains(w.Body.String(), "<script>alert(1)</script>") {
		t.Errorf("note title was not escaped:\n%s", w.Body.String())
	}
}

func TestPagesNeverJSON(t *testing.T) {
	st := newTestStore(t)
	h := New(st)
	n := seedNote(t, st, "shopping", "milk")

	gets := []string{
		"/",
		"/notes",
		"/notes/new",
		"/notes/" + n.ID,
		"/notes/" + n.ID + "/edit",
		"/notes/ffffffffffffffff",
		"/nope",
		"/notes/" + n.ID + "/nope",
	}
	for _, target := range gets {
		t.Run("GET "+target, func(t *testing.T) {
			assertHTML(t, do(t, h, http.MethodGet, target, ""))
		})
	}

	posts := []struct {
		name   string
		target string
		form   url.Values
	}{
		{"create validation failure", "/notes", url.Values{"title": {""}}},
		{"update validation failure", "/notes/" + n.ID + "/edit", url.Values{"title": {""}}},
		{"update unknown", "/notes/ffffffffffffffff/edit", url.Values{"title": {"t"}}},
		{"delete unknown", "/notes/ffffffffffffffff/delete", nil},
		{"unknown path", "/nope", nil},
		{"create", "/notes", url.Values{"title": {"fresh"}, "body": {"b"}}},
		{"update", "/notes/" + n.ID + "/edit", url.Values{"title": {"renamed"}, "body": {"b"}}},
		{"delete", "/notes/" + n.ID + "/delete", nil},
	}
	for _, tc := range posts {
		t.Run("POST "+tc.name, func(t *testing.T) {
			assertHTML(t, post(t, h, tc.target, tc.form))
		})
	}

	t.Run("panic", func(t *testing.T) {
		boom := recoverPage(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			panic("boom")
		}))
		w := do(t, boom, http.MethodGet, "/notes", "")
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
		}
		assertHTML(t, w)
	})
}
