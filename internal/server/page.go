package server

import (
	"bytes"
	"errors"
	"log"
	"net/http"

	"github.com/umars28/cowork-test/internal/note"
)

type pageHandler struct {
	store *note.Store
}

type listData struct {
	Notes []note.Note
}

type detailData struct {
	Note note.Note
}

type formData struct {
	Heading string
	Action  string
	Cancel  string
	Title   string
	Body    string
	Error   string
}

type errorData struct {
	Status  string
	Message string
}

func (p *pageHandler) root(w http.ResponseWriter, r *http.Request) {
	redirect(w, r, "/notes")
}

func (p *pageHandler) list(w http.ResponseWriter, r *http.Request) {
	renderPage(w, r, "list", http.StatusOK, listData{Notes: p.store.List()})
}

func (p *pageHandler) newForm(w http.ResponseWriter, r *http.Request) {
	renderPage(w, r, "form", http.StatusOK, formData{
		Heading: "New note",
		Action:  "/notes",
		Cancel:  "/notes",
	})
}

func (p *pageHandler) create(w http.ResponseWriter, r *http.Request) {
	title, body := formFields(r)

	n, err := p.store.Create(title, body)
	if errors.Is(err, note.ErrEmptyTitle) {
		renderPage(w, r, "form", http.StatusUnprocessableEntity, formData{
			Heading: "New note",
			Action:  "/notes",
			Cancel:  "/notes",
			Title:   title,
			Body:    body,
			Error:   err.Error(),
		})
		return
	}
	if err != nil {
		writePageError(w, r, err)
		return
	}
	redirect(w, r, "/notes/"+n.ID)
}

func (p *pageHandler) detail(w http.ResponseWriter, r *http.Request) {
	n, err := p.store.Get(r.PathValue("id"))
	if err != nil {
		writePageError(w, r, err)
		return
	}
	renderPage(w, r, "detail", http.StatusOK, detailData{Note: n})
}

func (p *pageHandler) editForm(w http.ResponseWriter, r *http.Request) {
	n, err := p.store.Get(r.PathValue("id"))
	if err != nil {
		writePageError(w, r, err)
		return
	}
	renderPage(w, r, "form", http.StatusOK, formData{
		Heading: "Edit note",
		Action:  "/notes/" + n.ID + "/edit",
		Cancel:  "/notes/" + n.ID,
		Title:   n.Title,
		Body:    n.Body,
	})
}

func (p *pageHandler) update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	title, body := formFields(r)

	n, err := p.store.Update(id, title, body)
	if errors.Is(err, note.ErrEmptyTitle) {
		renderPage(w, r, "form", http.StatusUnprocessableEntity, formData{
			Heading: "Edit note",
			Action:  "/notes/" + id + "/edit",
			Cancel:  "/notes/" + id,
			Title:   title,
			Body:    body,
			Error:   err.Error(),
		})
		return
	}
	if err != nil {
		writePageError(w, r, err)
		return
	}
	redirect(w, r, "/notes/"+n.ID)
}

func (p *pageHandler) destroy(w http.ResponseWriter, r *http.Request) {
	if err := p.store.Delete(r.PathValue("id")); err != nil {
		writePageError(w, r, err)
		return
	}
	redirect(w, r, "/notes")
}

func (p *pageHandler) notFound(w http.ResponseWriter, r *http.Request) {
	renderPage(w, r, "error", http.StatusNotFound, errorData{
		Status:  "404 Not Found",
		Message: "that page does not exist",
	})
}

func recoverPage(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				log.Printf("panic serving %s %s: %v", r.Method, r.URL.Path, v)
				renderPage(w, r, "error", http.StatusInternalServerError, errorData{
					Status:  "500 Internal Server Error",
					Message: "something went wrong",
				})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func formFields(r *http.Request) (string, string) {
	return r.PostFormValue("title"), r.PostFormValue("body")
}

func redirect(w http.ResponseWriter, r *http.Request, target string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func writePageError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, note.ErrNotFound) {
		renderPage(w, r, "error", http.StatusNotFound, errorData{
			Status:  "404 Not Found",
			Message: err.Error(),
		})
		return
	}
	log.Printf("store error: %v", err)
	renderPage(w, r, "error", http.StatusInternalServerError, errorData{
		Status:  "500 Internal Server Error",
		Message: "something went wrong",
	})
}

func renderPage(w http.ResponseWriter, r *http.Request, name string, status int, data any) {
	var buf bytes.Buffer
	if err := pageTemplates[name].ExecuteTemplate(&buf, "layout", data); err != nil {
		log.Printf("render %s for %s %s: %v", name, r.Method, r.URL.Path, err)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("<!doctype html><title>500</title><p>something went wrong</p>"))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	buf.WriteTo(w)
}
