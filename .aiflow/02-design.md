# Design — notes app: JSON API + server-rendered UI

This is an empty repo (one `main.go` printing a string, zero tests), so there is no existing
feature to imitate; the sections below define the template every later slice follows.

**Template decisions (binding for all later slices):**

- Layout: `cmd/cowork-test/` wires flags and process lifecycle only; `internal/note/` owns the
  domain type and persistence; `internal/server/` owns HTTP and HTML. Nothing in `internal/note`
  imports `net/http`; nothing in `internal/server` touches the filesystem.
- Dependencies: **standard library only**. `go.mod` keeps an empty require block. HTML comes from
  `html/template` + `embed`, routing from `net/http.ServeMux` method patterns (Go 1.22+).
- Errors: packages export sentinel errors (`note.ErrNotFound`) and callers match with
  `errors.Is`; the HTTP layer is the only place that maps an error to a status code.
- Tests: table-driven `_test.go` next to the code, `net/http/httptest` for handlers, `t.TempDir()`
  for the store. No test helper framework.

## Public surface

### JSON API — every response under `/api/` is `Content-Type: application/json`

| Method & path | Request body | Success |
| --- | --- | --- |
| `GET /api/notes` | — | `200` `{"notes":[Note,…]}` |
| `POST /api/notes` | `{"title":"…","body":"…"}` | `201` `Note` + `Location: /api/notes/{id}` |
| `GET /api/notes/{id}` | — | `200` `Note` |
| `PUT /api/notes/{id}` | `{"title":"…","body":"…"}` | `200` `Note` (full replace of both fields) |
| `DELETE /api/notes/{id}` | — | `200` `{"id":"…","deleted":true}` |

`DELETE` returns `200` with a body rather than `204`, because `204` is defined to carry no body
and the requirement is that an `/api/` request always answers with JSON.

Error envelope, used for **every** non-2xx under `/api/`:

```json
{"error":{"code":"not_found","message":"note not found"}}
```

`code` is one of `invalid_json`, `invalid_request`, `not_found`, `method_not_allowed`, `internal`.

### HTML pages — never answer with JSON, forms are plain `<form method="post">`

| Method & path | Result |
| --- | --- |
| `GET /` | `303` → `/notes` |
| `GET /notes` | list page |
| `GET /notes/new` | create form |
| `POST /notes` | create, `303` → `/notes/{id}`; on validation failure re-renders the form with `422` |
| `GET /notes/{id}` | detail page |
| `GET /notes/{id}/edit` | edit form |
| `POST /notes/{id}/edit` | update, `303` → `/notes/{id}` |
| `POST /notes/{id}/delete` | delete, `303` → `/notes` |

HTML forms cannot issue `PUT`/`DELETE` without JavaScript, so update and delete are `POST` to
dedicated sub-paths. Every successful form post is Post/Redirect/Get, so a browser reload never
re-submits. Failures render an HTML error page with the right status — never JSON, never a
redirect that swallows the message.

### Go types

```go
package note

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

type Store struct{ /* path string; mu sync.RWMutex; notes []Note */ }

func Open(path string) (*Store, error)
func (s *Store) List() []Note
func (s *Store) Get(id string) (Note, error)
func (s *Store) Create(title, body string) (Note, error)
func (s *Store) Update(id, title, body string) (Note, error)
func (s *Store) Delete(id string) error
```

```go
package server

func New(st *note.Store) http.Handler
```

On-disk format at `-data` (default `notes.json`): `{"version":1,"notes":[Note,…]}`.
`List` returns notes sorted by `CreatedAt` descending, `ID` ascending as tiebreak, so page order
and API order are stable and identical. IDs are 16 hex chars from `crypto/rand`.

## Modules touched

| File | Why |
| --- | --- |
| `cmd/cowork-test/main.go` (modified) | Parse `-addr` (default `:8080`) and `-data`, open the store, serve `server.New`, shut down on SIGINT/SIGTERM. Replaces the placeholder print. |
| `internal/note/note.go` (new) | `Note`, sentinel errors, title/body validation and trimming — the one place that defines what a valid note is. |
| `internal/note/store.go` (new) | File-backed CRUD: load once at `Open`, serve reads from memory, persist the whole set atomically on every mutation. |
| `internal/note/store_test.go` (new) | CRUD round-trip, `ErrNotFound`, validation, reload-after-write, and a concurrent-writer race test (`go test -race` runs in CI). |
| `internal/server/router.go` (new) | Builds the `ServeMux`; registers API routes under `/api/` and page routes separately, and installs the two different fallback handlers. |
| `internal/server/api.go` (new) | The five JSON handlers plus `writeJSON` / `writeAPIError`. |
| `internal/server/page.go` (new) | The HTML handlers plus `renderPage`; every exit path writes HTML. |
| `internal/server/templates.go` + `internal/server/templates/*.html` (new) | `//go:embed` of layout, list, detail, form and error templates, parsed once at init so a broken template fails the build's first test rather than a live request. |
| `internal/server/api_test.go`, `internal/server/page_test.go` (new) | `httptest` coverage of each route, including the two contract tests below. |
| `go.mod` (unchanged) | Stays dependency-free; that is the point. |

The two contract requirements get dedicated tests rather than being left to review:
`TestAPIAlwaysJSON` walks every `/api/` failure path (bad JSON, unknown id, wrong method, unknown
`/api/` path, forced panic) and asserts `Content-Type: application/json` plus a parseable error
envelope. `TestPagesNeverJSON` does the same for every page route and form post and asserts
`text/html` on all of them.

## Failure mode

**A crash or a concurrent write during persistence truncates `notes.json` and every note is
gone.** This is the one that actually costs something: with a single file and no database, a
half-written file is total data loss, not a degraded request. The naive implementation —
`os.WriteFile(path, data, 0o644)` from each handler — fails here twice: `WriteFile` truncates
before it writes, so a crash mid-write leaves a truncated file, and two concurrent requests
interleave into a file that is neither version.

The design handles it in three parts:

1. **One writer.** `Store` holds the notes in memory behind a `sync.RWMutex`; every mutation takes
   the write lock for the whole read-modify-persist sequence. Concurrent requests serialize, which
   at this scale costs nothing.
2. **Atomic replace.** Persist writes the full set to `notes.json.tmp` in the *same directory*,
   `f.Sync()`, `f.Close()`, then `os.Rename` over the target and fsyncs the parent directory.
   Rename within a directory is atomic, so a reader (or the next `Open` after a crash) sees either
   the complete old file or the complete new one, never a partial one.
3. **Commit after the write.** The in-memory slice is only updated once `Rename` returns nil. A
   failed disk write returns `500` / an HTML error page and leaves memory and disk still agreeing,
   so a transient `ENOSPC` does not leave the running process serving notes it never saved.
   Symmetrically, `Open` treats a missing file as an empty set but returns an error on malformed
   JSON, so a corrupt file stops the process instead of being silently replaced by `[]` on the
   first write.

Not handled, deliberately: two *processes* pointing at the same file. There is no `flock`; the
store documents single-process ownership. Adding advisory locking is a later slice if it is ever
needed.

## Options rejected

**One handler per resource with `Accept`-header content negotiation.** Attractive because it
halves the handler count, and it is how many Go examples do it. Rejected because the requirement
is a guarantee, not a preference: a browser form post sends `Accept: text/html,…,*/*;q=0.8`, and
any proxy, prefetcher, or `fetch()` that drops or rewrites that header flips the response to JSON.
Splitting on path prefix makes "`/api/` is JSON, everything else is HTML" a structural property
you can test by enumerating routes, including the `/api/` 404 and 405 fallbacks, which a
negotiating handler never even reaches.

**Append-only JSONL log instead of rewriting the whole file.** Cheaper writes and crash-safe by
construction. Rejected because update and delete become tombstones that need compaction, and
every read has to replay the log — real complexity bought for a workload where the entire dataset
is a few KB and a full rewrite is one `os.Rename`.

**`PATCH` with optional fields instead of `PUT`.** The issue says "update" without saying which.
Rejected in favour of `PUT` with full replacement because the HTML edit form already submits both
fields on every save, so `PUT` gives the API and the UI identical semantics and avoids
`*string`-shaped request structs to tell "absent" from "set to empty". `PATCH` can be added later
without breaking `PUT`.

**Storing notes in a `map[string]Note`.** Rejected because JSON object key order is unspecified,
so the file would churn on every write and diffs would be unreadable; a slice plus an explicit
sort gives a stable file and a stable list order for free.
