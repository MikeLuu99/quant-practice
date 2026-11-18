// Package edge hosts the existing HTTP API in one durable, event-sourced room.
// Every input is persisted before execution; replay uses its original server clock.
package edge

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"quantgame/backend/internal/api"
	"quantgame/backend/internal/engine"
)

const Version = "quant-edge-session-v1"
const MaxRequestBytes = 1 << 20
const MaxResponseBytes = 16 << 20

type Event struct {
	Version   string `json:"version"`
	SessionID string `json:"session_id"`
	AtMS      int64  `json:"at_ms"`
	Method    string `json:"method"`
	URL       string `json:"url"`
	Body      string `json:"body,omitempty"`
}
type Response struct {
	Status    int
	Header    http.Header
	Body      []byte
	NextAlarm time.Time
}
type Runner struct {
	id      string
	clock   time.Time
	handler *api.Handler
	mux     *http.ServeMux
	LastMS  int64
}

func New(id string) *Runner {
	r := &Runner{id: id}
	store := engine.NewStoreWithClock(func() time.Time { return r.clock }, func() string { return id })
	r.handler = &api.Handler{Store: store, Now: func() time.Time { return r.clock }}
	r.mux = http.NewServeMux()
	r.handler.Register(r.mux)
	return r
}
func (r *Runner) Execute(e Event) (Response, error) {
	if e.Version != Version || e.SessionID != r.id || e.AtMS < r.LastMS || e.AtMS < 0 || len(e.Body) > MaxRequestBytes {
		return Response{}, fmt.Errorf("invalid durable session event")
	}
	u, err := url.ParseRequestURI(e.URL)
	if err != nil {
		return Response{}, err
	}
	if e.Method != "ALARM" && !((e.Method == "POST" && u.Path == "/api/sessions") || strings.HasPrefix(u.Path, "/api/sessions/"+r.id+"/")) {
		return Response{}, fmt.Errorf("event targets another session")
	}
	r.clock = time.UnixMilli(e.AtMS)
	r.LastMS = e.AtMS
	r.handler.Store.TickAll()
	out := &recorder{header: make(http.Header)}
	if e.Method != "ALARM" {
		req, err := http.NewRequest(e.Method, "http://session"+e.URL, strings.NewReader(e.Body))
		if err != nil {
			return Response{}, err
		}
		req.Header.Set("Content-Type", "application/json")
		r.mux.ServeHTTP(out, req)
	}
	if out.err != nil {
		return Response{}, out.err
	}
	if out.status == 0 {
		out.status = 200
	}
	result := Response{Status: out.status, Header: out.header, Body: out.body.Bytes()}
	if s, ok := r.handler.Store.Get(r.id); ok && !s.Finished() {
		result.NextAlarm = s.NextWake(r.clock)
	}
	return result, nil
}

// Restoring never substitutes the current clock for recorded presentation/action times.
func Restore(id string, events []Event) (*Runner, error) {
	r := New(id)
	for i, e := range events {
		if _, err := r.Execute(e); err != nil {
			return nil, fmt.Errorf("restore event %d: %w", i, err)
		}
	}
	return r, nil
}

type recorder struct {
	header http.Header
	status int
	body   bytes.Buffer
	err    error
}

func (w *recorder) Header() http.Header { return w.header }
func (w *recorder) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}
func (w *recorder) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = 200
	}
	if w.body.Len()+len(p) > MaxResponseBytes {
		w.err = fmt.Errorf("session response exceeds size limit")
		return 0, w.err
	}
	return w.body.Write(p)
}
func WriteResponse(w http.ResponseWriter, res Response) {
	for k, vs := range res.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(res.Status)
	_, _ = w.Write(res.Body)
}
func ReadBody(r *http.Request) (string, error) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, MaxRequestBytes+1))
	if err != nil {
		return "", err
	}
	if len(raw) > MaxRequestBytes {
		return "", fmt.Errorf("request exceeds 1 MiB")
	}
	return string(raw), nil
}
