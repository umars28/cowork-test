package main

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestFlagDefaults(t *testing.T) {
	var cfg config
	fs := newFlagSet(&cfg)
	if err := fs.Parse(nil); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.addr != ":8080" {
		t.Errorf("addr = %q, want %q", cfg.addr, ":8080")
	}
	if cfg.data != "notes.json" {
		t.Errorf("data = %q, want %q", cfg.data, "notes.json")
	}
}

func TestFlagOverrides(t *testing.T) {
	var cfg config
	fs := newFlagSet(&cfg)
	if err := fs.Parse([]string{"-addr", "127.0.0.1:0", "-data", "/tmp/other.json"}); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.addr != "127.0.0.1:0" {
		t.Errorf("addr = %q, want %q", cfg.addr, "127.0.0.1:0")
	}
	if cfg.data != "/tmp/other.json" {
		t.Errorf("data = %q, want %q", cfg.data, "/tmp/other.json")
	}
}

func TestRunServesAndShutsDownOnSignal(t *testing.T) {
	args := []string{"-addr", "127.0.0.1:0", "-data", filepath.Join(t.TempDir(), "notes.json")}

	addrs := make(chan net.Addr, 1)
	done := make(chan error, 1)
	go func() {
		done <- run(args, io.Discard, func(a net.Addr) { addrs <- a })
	}()

	var addr net.Addr
	select {
	case addr = <-addrs:
	case err := <-done:
		t.Fatalf("run returned before serving: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("run never started listening")
	}

	base := "http://" + addr.String()
	resp, err := http.Get(base + "/api/notes")
	if err != nil {
		t.Fatalf("GET /api/notes: %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", resp.StatusCode, http.StatusOK, body)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}
	var payload struct {
		Notes []json.RawMessage `json:"notes"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode %q: %v", body, err)
	}

	if err := syscall.Kill(syscall.Getpid(), syscall.SIGINT); err != nil {
		t.Fatalf("send SIGINT: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run returned %v, want nil after a clean shutdown", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("run did not return after SIGINT")
	}

	if _, err := http.Get(base + "/api/notes"); err == nil {
		t.Error("server still accepting requests after shutdown")
	}
}

func TestRunRejectsUnusableDataPath(t *testing.T) {
	dir := t.TempDir()
	args := []string{"-addr", "127.0.0.1:0", "-data", filepath.Join(dir, "notes.json")}

	if err := os.WriteFile(filepath.Join(dir, "notes.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := run(args, io.Discard, func(net.Addr) {}); err == nil {
		t.Fatal("run returned nil on a malformed data file, want an error")
	}
}
