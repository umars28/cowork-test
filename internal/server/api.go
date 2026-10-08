package server

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/umars28/cowork-test/internal/note"
)

type apiHandler struct {
	store *note.Store
}

type noteRequest struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

func (a *apiHandler) list(w http.ResponseWriter, r *http.Request) {
	notes := a.store.List()
	if notes == nil {
		notes = []note.Note{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"notes": notes})
}

func (a *apiHandler) create(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeNoteRequest(w, r)
	if !ok {
		return
	}

	n, err := a.store.Create(req.Title, req.Body)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	w.Header().Set("Location", "/api/notes/"+n.ID)
	writeJSON(w, http.StatusCreated, n)
}

func (a *apiHandler) get(w http.ResponseWriter, r *http.Request) {
	n, err := a.store.Get(r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func (a *apiHandler) update(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeNoteRequest(w, r)
	if !ok {
		return
	}

	n, err := a.store.Update(r.PathValue("id"), req.Title, req.Body)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func (a *apiHandler) delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := a.store.Delete(id); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "deleted": true})
}

func apiNotFound(w http.ResponseWriter, r *http.Request) {
	writeAPIError(w, http.StatusNotFound, "not_found", "resource not found")
}

func apiMethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	writeAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
}

func recoverAPI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				log.Printf("panic serving %s %s: %v", r.Method, r.URL.Path, v)
				writeAPIError(w, http.StatusInternalServerError, "internal", "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func decodeNoteRequest(w http.ResponseWriter, r *http.Request) (noteRequest, bool) {
	var req noteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "request body is not valid JSON")
		return noteRequest{}, false
	}
	return req, true
}

func writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, note.ErrNotFound):
		writeAPIError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, note.ErrEmptyTitle):
		writeAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
	default:
		log.Printf("store error: %v", err)
		writeAPIError(w, http.StatusInternalServerError, "internal", "internal server error")
	}
}

func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("encode response: %v", err)
		data = []byte(`{"error":{"code":"internal","message":"internal server error"}}`)
		status = http.StatusInternalServerError
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(append(data, '\n'))
}
