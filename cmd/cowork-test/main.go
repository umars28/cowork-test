package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/umars28/cowork-test/internal/note"
	"github.com/umars28/cowork-test/internal/server"
)

const shutdownTimeout = 10 * time.Second

type config struct {
	addr string
	data string
}

func newFlagSet(cfg *config) *flag.FlagSet {
	fs := flag.NewFlagSet("cowork-test", flag.ContinueOnError)
	fs.StringVar(&cfg.addr, "addr", ":8080", "address to listen on")
	fs.StringVar(&cfg.data, "data", "notes.json", "path to the notes data file")
	return fs
}

func main() {
	err := run(os.Args[1:], os.Stderr, func(a net.Addr) {
		fmt.Fprintf(os.Stderr, "listening on %s\n", a)
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer, ready func(net.Addr)) error {
	var cfg config
	fs := newFlagSet(&cfg)
	fs.SetOutput(out)
	if err := fs.Parse(args); err != nil {
		return err
	}

	st, err := note.Open(cfg.data)
	if err != nil {
		return err
	}

	ln, err := net.Listen("tcp", cfg.addr)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := &http.Server{Handler: server.New(st)}
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()

	ready(ln.Addr())

	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
