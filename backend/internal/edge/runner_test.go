package edge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
)

func event(id string, at int64, method, path string, body any) Event {
	raw := []byte(nil)
	if body != nil {
		raw, _ = json.Marshal(body)
	}
	return Event{Version: Version, SessionID: id, AtMS: at, Method: method, URL: path, Body: string(raw)}
}
func execute(t *testing.T, r *Runner, events *[]Event, e Event) Response {
	t.Helper()
	*events = append(*events, e)
	out, err := r.Execute(e)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func TestEveryEngineRestoresAtOriginalClock(t *testing.T) {
	for _, slug := range []string{"fruit", "math", "cards", "marble", "etf", "nextcard", "prob", "fermi", "makeme", "assessment", "trainer"} {
		t.Run(slug, func(t *testing.T) {
			id := "s-test-" + slug
			r := New(id)
			events := []Event{}
			cfg := map[string]any{}
			if slug == "assessment" {
				cfg["profile_id"] = "mako-aptitude"
				cfg["mode"] = "preview"
			}
			if slug == "trainer" {
				cfg["trainer_id"] = "math"
			}
			created := execute(t, r, &events, event(id, 100000, "POST", "/api/sessions", map[string]any{"game": slug, "players": []string{"alice"}, "config": cfg}))
			if created.Status != 201 {
				t.Fatalf("create%d: %s", created.Status, created.Body)
			}
			var body map[string]any
			_ = json.Unmarshal(created.Body, &body)
			if body["session_id"] != id {
				t.Fatal("identity")
			}
			if body["seed"].(float64) > float64(1<<53-1) {
				t.Fatal("unsafe JS seed")
			}
			execute(t, r, &events, event(id, 100200, "GET", "/api/sessions/"+id+"/state?player=alice", nil))
			execute(t, r, &events, event(id, 101500, "ALARM", "/", nil))
			restored, err := Restore(id, events)
			if err != nil {
				t.Fatal(err)
			}
			read := event(id, 102000, "GET", "/api/sessions/"+id+"/state?player=alice", nil)
			a, err := r.Execute(read)
			if err != nil {
				t.Fatal(err)
			}
			b, err := restored.Execute(read)
			if err != nil {
				t.Fatal(err)
			}
			if a.Status != b.Status || !bytes.Equal(a.Body, b.Body) {
				t.Fatalf("restored state differs:\n%s\n%s", a.Body, b.Body)
			}
		})
	}
}
func TestTrainerExpiryAndDuplicateReceiptSurviveRestore(t *testing.T) {
	id := "s-math"
	r := New(id)
	events := []Event{}
	execute(t, r, &events, event(id, 1000, "POST", "/api/sessions", map[string]any{"game": "trainer", "players": []string{"alice"}, "seed": 42, "config": map[string]any{"trainer_id": "math", "duration_ms": 60000}}))
	action := func(at int64, name, actionID string, payload map[string]any) Event {
		return event(id, at, "POST", "/api/sessions/"+id+"/action", map[string]any{"player_id": "alice", "action_id": actionID, "action": name, "payload": payload})
	}
	begin := action(1004, "BEGIN", "begin", map[string]any{"elapsed_ms": 0})
	out := execute(t, r, &events, begin)
	if out.NextAlarm.UnixMilli() != 61004 {
		t.Fatal("deadline", out.NextAlarm)
	}
	restored, err := Restore(id, events)
	if err != nil {
		t.Fatal(err)
	}
	duplicate := action(1005, "BEGIN", "begin", map[string]any{"elapsed_ms": 0})
	out = execute(t, restored, &events, duplicate)
	if !bytes.Contains(out.Body, []byte("DUPLICATE")) {
		t.Fatal(string(out.Body))
	}
	out = execute(t, restored, &events, event(id, 61004, "GET", "/api/sessions/"+id+"/state?player=alice", nil))
	if !bytes.Contains(out.Body, []byte(`"finished":true`)) || !out.NextAlarm.IsZero() {
		t.Fatal("deadline lost", string(out.Body))
	}
	tape := execute(t, restored, &events, event(id, 61005, "GET", "/api/sessions/"+id+"/tape", nil))
	if tape.Status != 200 {
		t.Fatal(string(tape.Body))
	}
	again, err := Restore(id, events)
	if err != nil {
		t.Fatal(err)
	}
	same, err := again.Execute(event(id, 61005, "GET", "/api/sessions/"+id+"/tape", nil))
	if err != nil || !bytes.Equal(tape.Body, same.Body) {
		t.Fatal("tape changed after restoration", err)
	}
}
func TestInvalidJournalRefusesWrongIdentityVersionAndClock(t *testing.T) {
	for i, change := range []func(*Event){func(e *Event) { e.Version = "future" }, func(e *Event) { e.SessionID = "other" }, func(e *Event) { e.URL = "/api/sessions/other/state" }, func(e *Event) { e.AtMS = -1 }} {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			e := event("id", 100, "GET", "/api/sessions/id/state", nil)
			change(&e)
			if _, err := New("id").Execute(e); err == nil {
				t.Fatal("invalid event accepted")
			}
		})
	}
}
