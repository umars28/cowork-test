package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/umars28/cowork-test/internal/note"
)

func newTestStore(t *testing.T) *note.Store {
	t.Helper()
	st, err := note.Open(filepath.Join(t.TempDir(), "notes.json"))
	if err != nil {
		t.Fatalf("note.Open: %v", err)
	}
	return st
}

func do(t *testing.T, h http.Handler, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, target, nil)
	} else {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func decodeNote(t *testing.T, w *httptest.ResponseRecorder) note.Note {
	t.Helper()
	var n note.Note
	if err := json.Unmarshal(w.Body.Bytes(), &n); err != nil {
		t.Fatalf("decode note from %q: %v", w.Body.String(), err)
	}
	return n
}

func TestAPIListEmpty(t *testing.T) {
	h := New(newTestStore(t))

	w := do(t, h, http.MethodGet, "/api/notes", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var got struct {
		Notes []note.Note `json:"notes"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Notes == nil {
		t.Errorf("notes = null, want an empty array: %s", w.Body.String())
	}
	if len(got.Notes) != 0 {
		t.Errorf("notes = %d, want 0", len(got.Notes))
	}
}

func TestAPICreate(t *testing.T) {
	h := New(newTestStore(t))

	w := do(t, h, http.MethodPost, "/api/notes", `{"title":"  shopping  ","body":"  milk  "}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", w.Code, http.StatusCreated, w.Body.String())
	}
	created := decodeNote(t, w)
	if created.Title != "shopping" || created.Body != "milk" {
		t.Errorf("created = %+v, want trimmed title and body", created)
	}
	if want := "/api/notes/" + created.ID; w.Header().Get("Location") != want {
		t.Errorf("Location = %q, want %q", w.Header().Get("Location"), want)
	}

	w = do(t, h, http.MethodGet, "/api/notes", "")
	var list struct {
		Notes []note.Note `json:"notes"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list.Notes) != 1 || list.Notes[0].ID != created.ID {
		t.Errorf("list = %+v, want the created note", list.Notes)
	}
}

func TestAPIGetUpdateDelete(t *testing.T) {
	h := New(newTestStore(t))

	w := do(t, h, http.MethodPost, "/api/notes", `{"title":"shopping","body":"milk"}`)
	created := decodeNote(t, w)

	w = do(t, h, http.MethodGet, "/api/notes/"+created.ID, "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want %d", w.Code, http.StatusOK)
	}
	if got := decodeNote(t, w); got.ID != created.ID || got.Title != "shopping" {
		t.Errorf("GET = %+v, want %+v", got, created)
	}

	w = do(t, h, http.MethodPut, "/api/notes/"+created.ID, `{"title":"groceries","body":"milk and eggs"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}
	updated := decodeNote(t, w)
	if updated.Title != "groceries" || updated.Body != "milk and eggs" {
		t.Errorf("PUT = %+v, want the replaced fields", updated)
	}
	if updated.ID != created.ID {
		t.Errorf("PUT changed the id: %q, want %q", updated.ID, created.ID)
	}

	w = do(t, h, http.MethodDelete, "/api/notes/"+created.ID, "")
	if w.Code != http.StatusOK {
		t.Fatalf("DELETE status = %d, want %d", w.Code, http.StatusOK)
	}
	var del struct {
		ID      string `json:"id"`
		Deleted bool   `json:"deleted"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &del); err != nil {
		t.Fatalf("decode delete: %v", err)
	}
	if del.ID != created.ID || !del.Deleted {
		t.Errorf("DELETE body = %+v, want {%q true}", del, created.ID)
	}

	if w := do(t, h, http.MethodGet, "/api/notes/"+created.ID, ""); w.Code != http.StatusNotFound {
		t.Errorf("GET after DELETE status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestAPIErrorCodes(t *testing.T) {
	h := New(newTestStore(t))
	w := do(t, h, http.MethodPost, "/api/notes", `{"title":"shopping","body":"milk"}`)
	id := decodeNote(t, w).ID

	tests := []struct {
		name       string
		method     string
		target     string
		body       string
		wantStatus int
		wantCode   string
	}{
		{"bad json on create", http.MethodPost, "/api/notes", `{"title":`, http.StatusBadRequest, "invalid_json"},
		{"bad json on update", http.MethodPut, "/api/notes/" + id, `nope`, http.StatusBadRequest, "invalid_json"},
		{"empty title on create", http.MethodPost, "/api/notes", `{"title":"  ","body":"milk"}`, http.StatusBadRequest, "invalid_request"},
		{"empty title on update", http.MethodPut, "/api/notes/" + id, `{"title":"","body":"milk"}`, http.StatusBadRequest, "invalid_request"},
		{"unknown id on get", http.MethodGet, "/api/notes/ffffffffffffffff", "", http.StatusNotFound, "not_found"},
		{"unknown id on update", http.MethodPut, "/api/notes/ffffffffffffffff", `{"title":"t","body":"b"}`, http.StatusNotFound, "not_found"},
		{"unknown id on delete", http.MethodDelete, "/api/notes/ffffffffffffffff", "", http.StatusNotFound, "not_found"},
		{"unknown api path", http.MethodGet, "/api/nope", "", http.StatusNotFound, "not_found"},
		{"wrong method on collection", http.MethodPatch, "/api/notes", "", http.StatusMethodNotAllowed, "method_not_allowed"},
		{"wrong method on item", http.MethodPatch, "/api/notes/" + id, "", http.StatusMethodNotAllowed, "method_not_allowed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := do(t, h, tt.method, tt.target, tt.body)
			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d: %s", w.Code, tt.wantStatus, w.Body.String())
			}
			if got := errorCode(t, w); got != tt.wantCode {
				t.Errorf("error code = %q, want %q", got, tt.wantCode)
			}
		})
	}
}

func errorCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var env struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode error envelope from %q: %v", w.Body.String(), err)
	}
	if env.Error.Message == "" {
		t.Errorf("error message is empty: %s", w.Body.String())
	}
	return env.Error.Code
}

func TestAPIAlwaysJSON(t *testing.T) {
	h := New(newTestStore(t))
	w := do(t, h, http.MethodPost, "/api/notes", `{"title":"shopping","body":"milk"}`)
	id := decodeNote(t, w).ID

	cases := []struct {
		name   string
		method string
		target string
		body   string
	}{
		{"bad json", http.MethodPost, "/api/notes", `{`},
		{"invalid request", http.MethodPost, "/api/notes", `{"title":""}`},
		{"unknown id", http.MethodGet, "/api/notes/ffffffffffffffff", ""},
		{"wrong method", http.MethodPatch, "/api/notes/" + id, ""},
		{"unknown api path", http.MethodGet, "/api/whatever", ""},
		{"unknown api subpath", http.MethodPost, "/api/notes/" + id + "/extra", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := do(t, h, tc.method, tc.target, tc.body)
			if w.Code < 400 {
				t.Fatalf("status = %d, want a failure", w.Code)
			}
			assertJSONEnvelope(t, w)
		})
	}

	t.Run("panic", func(t *testing.T) {
		boom := recoverAPI(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			panic("boom")
		}))
		w := do(t, boom, http.MethodGet, "/api/notes", "")
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
		}
		assertJSONEnvelope(t, w)
		if got := errorCode(t, w); got != "internal" {
			t.Errorf("error code = %q, want %q", got, "internal")
		}
	})
}

func assertJSONEnvelope(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}
	if code := errorCode(t, w); code == "" {
		t.Errorf("error envelope has an empty code: %s", w.Body.String())
	}
}
