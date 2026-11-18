// Package api exposes the quant-game HTTP API: game catalog, session
// lifecycle, action processing, results and reproducibility tapes.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"quantgame/backend/internal/engine"
	"quantgame/backend/internal/games"
)

type Handler struct {
	Store *engine.Store
	Now   func() time.Time
}

func NewHandler() *Handler {
	return &Handler{Store: engine.NewStore(), Now: time.Now}
}

// StartTicking drives timers and bots for all live sessions.
func (h *Handler) StartTicking() {
	go func() {
		for {
			time.Sleep(200 * time.Millisecond)
			h.Store.TickAll()
		}
	}()
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg})
}

// cors allows the Astro dev server and any local origin.
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Register mounts all routes on the mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/games", h.listGames)
	mux.HandleFunc("GET /api/trainers", h.listTrainers)
	mux.HandleFunc("GET /api/trainers/library", h.trainerLibrary)
	mux.HandleFunc("POST /api/trainers/replay", h.replayTrainer)
	mux.HandleFunc("GET /api/assessments", h.listAssessments)
	mux.HandleFunc("POST /api/assessments/replay", h.replayAssessment)
	mux.HandleFunc("POST /api/sessions", h.createSession)
	mux.HandleFunc("GET /api/sessions/{id}/state", h.getState)
	mux.HandleFunc("POST /api/sessions/{id}/action", h.postAction)
	mux.HandleFunc("GET /api/sessions/{id}/results", h.getResults)
	mux.HandleFunc("GET /api/sessions/{id}/tape", h.getTape)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
	})
}

func (h *Handler) listAssessments(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"profiles": games.AssessmentProfiles(), "runtime_version": "assessment-runtime-v1"})
}

func (h *Handler) replayAssessment(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 5<<20))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "replay tape exceeds 5 MiB or could not be read")
		return
	}
	results, err := games.ReplayAssessment(raw)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"verified": true, "results": results})
}

func (h *Handler) listGames(w http.ResponseWriter, _ *http.Request) {
	// Construct one throwaway game per slug to read its meta.
	games := []engine.GameMeta{}
	for _, slug := range []string{"fruit", "math", "cards", "marble", "etf", "nextcard", "prob", "fermi", "makeme"} {
		g, err := engine.NewGame(slug, nil, []string{"x"}, 1, time.Now())
		if err != nil {
			continue
		}
		games = append(games, g.Meta())
	}
	writeJSON(w, http.StatusOK, map[string]any{"games": games})
}

type createReq struct {
	Game    string          `json:"game"`
	Players []string        `json:"players"`
	Config  json.RawMessage `json:"config"`
	Seed    uint64          `json:"seed"`
}

func (h *Handler) createSession(w http.ResponseWriter, r *http.Request) {
	var req createReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad json body")
		return
	}
	if req.Game == "" {
		writeErr(w, http.StatusBadRequest, "missing game")
		return
	}
	if len(req.Players) == 0 {
		req.Players = []string{"p1"}
	}
	if req.Seed == 0 {
		req.Seed = uint64(h.Now().UnixNano()) & ((1 << 53) - 1)
		if req.Seed == 0 {
			req.Seed = 1
		}
	}
	s, err := h.Store.Create(req.Game, req.Config, req.Players, req.Seed)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	snaps := map[string]json.RawMessage{}
	for _, p := range s.Players {
		if snap, err := s.Snapshot(p); err == nil {
			snaps[p] = snap
		}
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"session_id": s.ID,
		"game":       s.Slug,
		"players":    s.Players,
		"seed":       req.Seed,
		"snapshots":  snaps,
	})
}

func (h *Handler) session(w http.ResponseWriter, r *http.Request) (*engine.Session, bool) {
	id := r.PathValue("id")
	s, ok := h.Store.Get(id)
	if !ok {
		writeErr(w, http.StatusNotFound, "unknown session "+id)
		return nil, false
	}
	return s, true
}

func (h *Handler) getState(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	player := r.URL.Query().Get("player")
	if player == "" && len(s.Players) > 0 {
		player = s.Players[0]
	}
	snap, err := s.Snapshot(player)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"session_id": s.ID,
		"game":       s.Slug,
		"players":    s.Players,
		"player":     player,
		"finished":   s.Finished(),
		"state":      snap,
	})
}

type actionReq struct {
	engine.ActionRequest
}

func (h *Handler) postAction(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	var req actionReq
	if err := json.NewDecoder(r.Body).Decode(&req.ActionRequest); err != nil {
		writeErr(w, http.StatusBadRequest, "bad json body")
		return
	}
	out := s.Apply(req.ActionRequest)
	writeJSON(w, http.StatusOK, map[string]any{"output": out})
}

func (h *Handler) getResults(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	res, err := s.Results()
	if err != nil {
		if errors.Is(err, engine.ErrNotFinished) {
			writeErr(w, http.StatusConflict, err.Error())
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"session_id": s.ID,
		"finished":   s.Finished(),
		"results":    res,
	})
}

func (h *Handler) getTape(w http.ResponseWriter, r *http.Request) {
	s, ok := h.session(w, r)
	if !ok {
		return
	}
	tape, err := s.Tape()
	if err != nil {
		if errors.Is(err, engine.ErrNotFinished) {
			writeErr(w, http.StatusConflict, err.Error())
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(tape)
}

// ---------------------------------------------------------------------------
// Server bootstrap.
// ---------------------------------------------------------------------------

// Server builds the HTTP server. If staticDir exists on disk, the built
// Astro frontend is served from it (single-origin deployment); API routes
// take precedence. Pass "" to skip static serving.
func Server(port int, staticDir string) *http.Server {
	h := NewHandler()
	h.StartTicking()
	mux := http.NewServeMux()
	h.Register(mux)
	if staticDir != "" {
		if st, err := os.Stat(staticDir); err == nil && st.IsDir() {
			fs := http.FileServer(http.Dir(staticDir))
			mux.Handle("GET /", fs)
			log.Printf("serving static frontend from %s", staticDir)
		}
	}
	addr := fmt.Sprintf(":%d", port)
	return &http.Server{Addr: addr, Handler: cors(mux), ReadHeaderTimeout: 5 * time.Second}
}

func (h *Handler) listTrainers(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"profiles": games.TrainerProfiles(), "runtime_version": "trainer-runtime-v1"})
}
func (h *Handler) trainerLibrary(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"items": games.TrainerFermiLibrary()})
}
func (h *Handler) replayTrainer(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8<<20))
	if err != nil {
		writeErr(w, 400, "trainer tape exceeds 8 MiB")
		return
	}
	results, err := games.ReplayTrainer(raw)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"verified": true, "results": results})
}
