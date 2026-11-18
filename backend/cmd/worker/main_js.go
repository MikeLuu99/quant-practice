//go:build js && wasm

// Cloudflare-specific adapter. Native server and all engine tests stay portable.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"syscall/js"
	"time"

	"github.com/syumai/workers-go"
	"github.com/syumai/workers-go/exp/cloudflare/durableobjects"
	"quantgame/backend/internal/api"
	"quantgame/backend/internal/edge"
)

type Room struct {
	state  *durableobjects.DurableObjectState
	mu     sync.Mutex
	runner *edge.Runner
	id     string
}

func newRoom(state *durableobjects.DurableObjectState, _ js.Value) (durableobjects.Object, error) {
	_, err := state.Storage().SQL().Exec(`CREATE TABLE IF NOT EXISTS session_events(seq INTEGER PRIMARY KEY,event TEXT NOT NULL)`)
	if err != nil {
		return nil, err
	}
	return &Room{state: state}, nil
}
func (g *Room) load(id string) error {
	if g.runner != nil {
		if id != "" && g.id != id {
			return fmt.Errorf("session identity mismatch")
		}
		return nil
	}
	cursor, err := g.state.Storage().SQL().Exec(`SELECT event FROM session_events ORDER BY seq`)
	if err != nil {
		return err
	}
	rows, err := cursor.Rows()
	if err != nil {
		return err
	}
	events := make([]edge.Event, len(rows))
	for i, row := range rows {
		raw, ok := row["event"].(string)
		if !ok {
			return fmt.Errorf("invalid journal row")
		}
		if err = json.Unmarshal([]byte(raw), &events[i]); err != nil {
			return err
		}
		if id == "" {
			id = events[i].SessionID
		}
	}
	if id == "" {
		return nil
	}
	g.runner, err = edge.Restore(id, events)
	g.id = id
	return err
}
func (g *Room) commit(e edge.Event) (edge.Response, error) {
	e.Version = edge.Version
	e.SessionID = g.id
	e.AtMS = max(time.Now().UnixMilli(), g.runner.LastMS)
	raw, err := json.Marshal(e)
	if err != nil {
		return edge.Response{}, err
	}
	// Persist first. A crash between this insert and execution replays the same
	// request at exactly the original time, including its idempotency receipt.
	if _, err = g.state.Storage().SQL().Exec(`INSERT INTO session_events(event) VALUES(?)`, string(raw)); err != nil {
		return edge.Response{}, err
	}
	response, err := g.runner.Execute(e)
	if err != nil {
		g.runner = nil
		return edge.Response{}, err
	}
	if response.NextAlarm.IsZero() {
		err = g.state.Storage().DeleteAlarm(durableobjects.DurableObjectSetAlarmOptions{})
	} else {
		err = g.state.Storage().SetAlarm(response.NextAlarm, durableobjects.DurableObjectSetAlarmOptions{})
	}
	return response, err
}
func (g *Room) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	defer g.mu.Unlock()
	id := r.Header.Get("X-Quant-Session")
	if id == "" {
		http.Error(w, "missing session identity", 400)
		return
	}
	if err := g.load(id); err != nil {
		log.Print(err)
		http.Error(w, "session could not be restored", 500)
		return
	}
	body, err := edge.ReadBody(r)
	if err != nil {
		http.Error(w, err.Error(), 413)
		return
	}
	response, err := g.commit(edge.Event{Method: r.Method, URL: r.URL.RequestURI(), Body: body})
	if err != nil {
		log.Print(err)
		http.Error(w, "session storage unavailable; retry the same action ID", 503)
		return
	}
	edge.WriteResponse(w, response)
}
func (g *Room) Alarm(_ context.Context, _ *durableobjects.AlarmInvocationInfo) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.load(""); err != nil {
		return err
	}
	if g.runner == nil {
		return nil
	}
	_, err := g.commit(edge.Event{Method: "ALARM", URL: "/"})
	return err
}
func main() {
	durableobjects.Register("GameSession", newRoom)
	// Catalogue and replay routes are stateless. Session routes are dispatched
	// to GameSession by worker/index.mjs, before this ordinary fetch handler.
	h := api.NewHandler()
	mux := http.NewServeMux()
	h.Register(mux)
	workers.Serve(mux)
}
