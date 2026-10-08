package note

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

const fileVersion = 1

type fileFormat struct {
	Version int    `json:"version"`
	Notes   []Note `json:"notes"`
}

type Store struct {
	path  string
	mu    sync.RWMutex
	notes []Note
}

func Open(path string) (*Store, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &Store{path: path}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var f fileFormat
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if f.Version != fileVersion {
		return nil, fmt.Errorf("parse %s: unsupported version %d", path, f.Version)
	}

	sortNotes(f.Notes)
	return &Store{path: path, notes: f.Notes}, nil
}

func (s *Store) List() []Note {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.notes)
}

func (s *Store) Get(id string) (Note, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	i := indexOf(s.notes, id)
	if i < 0 {
		return Note{}, ErrNotFound
	}
	return s.notes[i], nil
}

func (s *Store) Create(title, body string) (Note, error) {
	title, body, err := Validate(title, body)
	if err != nil {
		return Note{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	id, err := newID()
	if err != nil {
		return Note{}, err
	}
	now := time.Now().UTC()
	n := Note{ID: id, Title: title, Body: body, CreatedAt: now, UpdatedAt: now}

	next := append(slices.Clone(s.notes), n)
	sortNotes(next)
	if err := s.persist(next); err != nil {
		return Note{}, err
	}
	s.notes = next
	return n, nil
}

func (s *Store) Update(id, title, body string) (Note, error) {
	title, body, err := Validate(title, body)
	if err != nil {
		return Note{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	i := indexOf(s.notes, id)
	if i < 0 {
		return Note{}, ErrNotFound
	}

	next := slices.Clone(s.notes)
	next[i].Title = title
	next[i].Body = body
	next[i].UpdatedAt = time.Now().UTC()
	if err := s.persist(next); err != nil {
		return Note{}, err
	}
	s.notes = next
	return next[i], nil
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	i := indexOf(s.notes, id)
	if i < 0 {
		return ErrNotFound
	}

	next := slices.Delete(slices.Clone(s.notes), i, i+1)
	if err := s.persist(next); err != nil {
		return err
	}
	s.notes = next
	return nil
}

func (s *Store) persist(notes []Note) error {
	data, err := json.Marshal(fileFormat{Version: fileVersion, Notes: notes})
	if err != nil {
		return fmt.Errorf("encode %s: %w", s.path, err)
	}

	dir := filepath.Dir(s.path)
	tmp := s.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("create %s: %w", tmp, err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("sync %s: %w", tmp, err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("close %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename %s: %w", tmp, err)
	}

	d, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open %s: %w", dir, err)
	}
	defer d.Close()
	if err := d.Sync(); err != nil {
		return fmt.Errorf("sync %s: %w", dir, err)
	}
	return nil
}

func indexOf(notes []Note, id string) int {
	return slices.IndexFunc(notes, func(n Note) bool { return n.ID == id })
}

func sortNotes(notes []Note) {
	slices.SortFunc(notes, func(a, b Note) int {
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return b.CreatedAt.Compare(a.CreatedAt)
		}
		return strings.Compare(a.ID, b.ID)
	})
}

func newID() (string, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return hex.EncodeToString(buf[:]), nil
}
