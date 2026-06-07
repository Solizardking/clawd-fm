// Package api exposes the CLAWD FM station over HTTP with REST endpoints
// and Server-Sent Events (SSE) for real-time frontends.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"

	"clawdamp/radio"
)

// Server serves the CLAWD FM HTTP API wired to a station.
type Server struct {
	station *radio.Station
	mu      sync.RWMutex
	subs    map[chan []byte]struct{}
	srv     *http.Server
}

// New creates an API server backed by the given station.
func New(st *radio.Station) *Server {
	s := &Server{
		station: st,
		subs:    make(map[chan []byte]struct{}),
	}
	go s.fanout()
	return s
}

// Start begins listening on addr (e.g. ":8080") and blocks until the
// server shuts down. Callers should invoke Start in a goroutine.
func (s *Server) Start(addr string) error {
	s.mu.Lock()
	s.srv = &http.Server{Addr: addr, Handler: s.mux()}
	s.mu.Unlock()
	log.Printf("[api] listening on %s", addr)
	return s.srv.ListenAndServe()
}

// Shutdown gracefully stops the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.RLock()
	srv := s.srv
	s.mu.RUnlock()
	if srv == nil {
		return nil
	}
	return srv.Shutdown(ctx)
}

func (s *Server) mux() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/stats", s.handleStats)
	mux.HandleFunc("/api/queue", s.handleQueue)
	mux.HandleFunc("/api/chat", s.handleChat)
	mux.HandleFunc("/api/tip", s.handleTip)
	mux.HandleFunc("/api/playlist", s.handlePlaylist)
	mux.HandleFunc("/events", s.handleSSE)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})

	return corsMiddleware(mux)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "CLAWD FM ONLINE")
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	stats := s.station.Stats()
	writeJSON(w, http.StatusOK, stats)
}

func (s *Server) handleQueue(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		queue := s.station.GetQueue()
		writeJSON(w, http.StatusOK, map[string]interface{}{"queue": queue})
	case http.MethodPost:
		var req struct {
			Title  string `json:"title"`
			Artist string `json:"artist"`
			Source string `json:"source"`
			CID    string `json:"cid"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if req.Title == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "title required"})
			return
		}
		_ = s.station.QueueTrackFromAPI(req.Title, req.Artist, req.Source, req.CID)
		queue := s.station.GetQueue()
		writeJSON(w, http.StatusOK, map[string]interface{}{"queue": queue})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "use POST"})
		return
	}
	var req struct {
		Name string `json:"name"`
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if req.Text == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "text required"})
		return
	}
	s.station.SendChat(s.station.Identity, req.Text)
	writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}

func (s *Server) handleTip(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "use POST"})
		return
	}
	var req struct {
		CID      string `json:"cid"`
		Lamports uint64 `json:"lamports"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if req.CID == "" || req.Lamports == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cid and lamports required"})
		return
	}
	if err := s.station.TipTrack(req.CID, req.Lamports); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "tipped"})
}

func (s *Server) handlePlaylist(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "use POST"})
		return
	}
	var req struct {
		Name  string   `json:"name"`
		CIDs  []string `json:"cids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name required"})
		return
	}
	if err := s.station.CreatePlaylistOnChain(req.Name, req.CIDs); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "created"})
}

// handleSSE is a Server-Sent Events endpoint that streams station events
// to connected clients.
func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := make(chan []byte, 64)
	s.mu.Lock()
	s.subs[ch] = struct{}{}
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.subs, ch)
		s.mu.Unlock()
	}()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case data := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

func (s *Server) fanout() {
	for ev := range s.station.Events() {
		data, _ := json.Marshal(ev)
		s.mu.RLock()
		for ch := range s.subs {
			select {
			case ch <- data:
			default:
			}
		}
		s.mu.RUnlock()
	}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		// Normalise trailing slash.
		if r.URL.Path != "/" {
			r.URL.Path = strings.TrimRight(r.URL.Path, "/")
		}
		next.ServeHTTP(w, r)
	})
}