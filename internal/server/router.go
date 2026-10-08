package server

import (
	"net/http"

	"github.com/umars28/cowork-test/internal/note"
)

func New(st *note.Store) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/api/", recoverAPI(apiRoutes(st)))
	mux.Handle("/", recoverPage(pageRoutes(st)))
	return mux
}

func pageRoutes(st *note.Store) http.Handler {
	p := &pageHandler{store: st}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", p.root)
	mux.HandleFunc("GET /notes", p.list)
	mux.HandleFunc("POST /notes", p.create)
	mux.HandleFunc("GET /notes/new", p.newForm)
	mux.HandleFunc("GET /notes/{id}", p.detail)
	mux.HandleFunc("GET /notes/{id}/edit", p.editForm)
	mux.HandleFunc("POST /notes/{id}/edit", p.update)
	mux.HandleFunc("POST /notes/{id}/delete", p.destroy)
	mux.HandleFunc("/", p.notFound)
	return mux
}

func apiRoutes(st *note.Store) http.Handler {
	a := &apiHandler{store: st}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/notes", a.list)
	mux.HandleFunc("POST /api/notes", a.create)
	mux.HandleFunc("GET /api/notes/{id}", a.get)
	mux.HandleFunc("PUT /api/notes/{id}", a.update)
	mux.HandleFunc("DELETE /api/notes/{id}", a.delete)
	mux.HandleFunc("/api/notes", apiMethodNotAllowed)
	mux.HandleFunc("/api/notes/{id}", apiMethodNotAllowed)
	mux.HandleFunc("/api/", apiNotFound)
	return mux
}
