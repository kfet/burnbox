// Package server wires burnbox's HTTP surface: a blind blob store plus
// static page serving. It contains no cryptography — it stores and
// returns opaque ciphertext and serves the embedded frontend.
package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/kfet/burnbox/internal/store"
	"github.com/kfet/burnbox/internal/ui"
)

// Server holds the blob store and serves the HTTP API + frontend.
type Server struct {
	store *store.Store
	mux   *http.ServeMux
}

// New constructs a Server backed by st and registers all routes.
func New(st *store.Store) *Server {
	s := &Server{store: st, mux: http.NewServeMux()}
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("POST /s", s.handlePut)
	s.mux.HandleFunc("GET /s/{id}", s.handleGet)
	s.mux.HandleFunc("GET /r/{id}", s.handleRecipe)
	s.mux.HandleFunc("GET /burnbox.js", s.handleScript)
	s.mux.HandleFunc("GET /", s.handleIndex)
	return s
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	_, _ = io.WriteString(w, "ok")
}

// handlePut stores a posted ciphertext blob and returns its id.
func (s *Server) handlePut(w http.ResponseWriter, r *http.Request) {
	// Cap the read at maxSize+1 so we can distinguish "too large".
	max := s.store.MaxSize()
	body, err := io.ReadAll(io.LimitReader(r.Body, int64(max)+1))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read failed"})
		return
	}
	if len(body) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "empty body"})
		return
	}
	if len(body) > max {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "blob too large"})
		return
	}

	ttl := parseTTL(r.URL.Query().Get("ttl"))
	id, err := s.store.Put(body, ttl)
	if err != nil {
		// Size is already enforced above, so any error here is internal.
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "store failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id})
}

// parseTTL converts a ?ttl= seconds string to a duration. Invalid or
// missing values yield 0, which the store clamps to its default.
func parseTTL(s string) time.Duration {
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0
	}
	return time.Duration(n) * time.Second
}

// handleGet atomically returns and burns a blob.
func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	blob, err := s.store.GetDel(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found or already viewed"})
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(blob)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	// Only the canonical root serves the SPA; unknown paths 404 (but
	// still as the SPA-less JSON to avoid leaking a file tree).
	if r.URL.Path != "/" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	serveHTML(w, ui.Index)
}

func (s *Server) handleRecipe(w http.ResponseWriter, _ *http.Request) {
	serveHTML(w, ui.Recipe)
}

func (s *Server) handleScript(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(ui.Script)
}

func serveHTML(w http.ResponseWriter, body []byte) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(body)
}
