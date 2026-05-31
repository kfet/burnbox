// Command burnbox is the single-binary entry point.
//
// Subcommands:
//
//	burnbox serve [-listen ADDR] [-max-size BYTES] [-max-ttl DUR] [-min-ttl DUR]
//	burnbox version
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kfet/burnbox"
	"github.com/kfet/burnbox/internal/server"
	"github.com/kfet/burnbox/internal/store"
)

const usage = `burnbox — server-blind one-time secrets

usage:
  burnbox serve [flags]
  burnbox version

serve flags:
  -listen ADDR     address to listen on (default ":8080")
  -max-size BYTES  maximum ciphertext blob size (default 262144)
  -max-ttl DUR     ceiling for requested TTLs (default 168h)
  -min-ttl DUR     floor / default TTL (default 1h)
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "version":
		fmt.Fprintf(stdout, "burnbox %s (commit %s, built %s)\n",
			burnbox.Version, burnbox.Commit, burnbox.BuildDate)
		return 0
	case "serve":
		return cmdServe(rest, stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n%s", cmd, usage)
		return 2
	}
}

func cmdServe(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	listen := fs.String("listen", ":8080", "address to listen on")
	maxSize := fs.Int("max-size", 256<<10, "maximum ciphertext blob size in bytes")
	maxTTL := fs.Duration("max-ttl", 7*24*time.Hour, "ceiling for requested TTLs")
	minTTL := fs.Duration("min-ttl", time.Hour, "floor / default TTL")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	st := store.New(store.Options{
		MaxSize: *maxSize,
		MaxTTL:  *maxTTL,
		MinTTL:  *minTTL,
	})
	defer st.Close()

	srv := &http.Server{
		Addr:              *listen,
		Handler:           server.New(st),
		ReadHeaderTimeout: 10 * time.Second,
	}

	return serveUntilSignal(srv, stdout, stderr)
}

// serveUntilSignal runs srv until SIGINT/SIGTERM, then drains with a
// bounded shutdown. Kept separate from cmdServe to keep flag-parsing and
// run-loop concerns apart.
func serveUntilSignal(srv *http.Server, stdout, stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	fmt.Fprintf(stdout, "burnbox listening on %s\n", srv.Addr)

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(stderr, "serve error: %v\n", err)
			return 1
		}
		return 0
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		fmt.Fprintln(stdout, "shutting down")
		return 0
	}
}
