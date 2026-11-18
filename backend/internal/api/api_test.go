package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func newTestServer() *httptest.Server {
	h := NewHandler()
	mux := http.NewServeMux()
	h.Register(mux)
	return httptest.NewServer(cors(mux))
}

func post(t *testing.T, url string, body any) (int, map[string]any) {
	t.Helper()
	b, _ := json.Marshal(body)
	resp, err := http.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func get(t *testing.T, url string) (int, map[string]any) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func TestCatalogListsAllNineGames(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	code, body := get(t, ts.URL+"/api/games")
	if code != 200 {
		t.Fatalf("catalog status %d", code)
	}
	games := body["games"].([]any)
	if len(games) != 9 {
		t.Fatalf("catalog has %d games, want 9", len(games))
	}
	want := map[string]bool{"fruit": false, "math": false, "cards": false, "marble": false,
		"etf": false, "nextcard": false, "prob": false, "fermi": false, "makeme": false}
	for _, g := range games {
		want[g.(map[string]any)["slug"].(string)] = true
	}
	for slug, ok := range want {
		if !ok {
			t.Fatalf("missing game %s", slug)
		}
	}
}

func TestUnknownSessionAndGame(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	code, _ := get(t, ts.URL+"/api/sessions/nope/state")
	if code != 404 {
		t.Fatalf("unknown session status %d, want 404", code)
	}
	code, body := post(t, ts.URL+"/api/sessions", map[string]any{"game": "nope"})
	if code != 400 {
		t.Fatalf("unknown game status %d, want 400", code)
	}
	if body["error"] == nil {
		t.Fatal("error message missing")
	}
}

// fruitPlaythrough exercises: state, idempotent action retry, stale version,
// results and tape endpoints over HTTP.
func TestFruitPlaythroughHTTP(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	code, body := post(t, ts.URL+"/api/sessions", map[string]any{
		"game": "fruit", "players": []string{"p1"}, "seed": 42,
		"config": map[string]any{"duration_ms": 4000, "min_trade_interval_ms": 100},
	})
	if code != 201 {
		t.Fatalf("create status %d: %v", code, body)
	}
	id := body["session_id"].(string)

	_, st := get(t, ts.URL+"/api/sessions/"+id+"/state?player=p1")
	state := st["state"].(map[string]any)
	ver := int(state["market_version"].(float64))

	// Accepted BUY.
	code, out := post(t, ts.URL+"/api/sessions/"+id+"/action", map[string]any{
		"action_id": "a-1", "player_id": "p1", "market_version": ver, "action": "BUY",
		"payload": map[string]any{"quantity": 1},
	})
	if code != 200 {
		t.Fatalf("action status %d", code)
	}
	output := out["output"].(map[string]any)
	if output["accepted"] != true {
		t.Fatalf("BUY rejected: %v", output)
	}
	// Retry the same action id: original receipt, no double fill.
	_, out2 := post(t, ts.URL+"/api/sessions/"+id+"/action", map[string]any{
		"action_id": "a-1", "player_id": "p1", "market_version": ver, "action": "BUY",
		"payload": map[string]any{"quantity": 1},
	})
	o2 := out2["output"].(map[string]any)
	if o2["reason"] != "DUPLICATE" {
		t.Fatalf("retry reason %v, want DUPLICATE", o2["reason"])
	}
	// Stale version (wait out the rate limit first).
	time.Sleep(150 * time.Millisecond)
	_, out3 := post(t, ts.URL+"/api/sessions/"+id+"/action", map[string]any{
		"action_id": "a-2", "player_id": "p1", "market_version": ver - 100, "action": "SELL",
	})
	o3 := out3["output"].(map[string]any)
	if o3["accepted"] != false || o3["reason"] != "STALE_MARKET" {
		t.Fatalf("stale action %+v", o3)
	}
	// Tape endpoint.
	code, tape := get(t, ts.URL+"/api/sessions/"+id+"/tape")
	if code != 200 {
		t.Fatalf("tape status %d", code)
	}
	if tape["prng"] != "splitmix64-v1" {
		t.Fatalf("tape prng %v", tape["prng"])
	}
	// Results endpoint.
	time.Sleep(4500 * time.Millisecond) // game is 4s
	_, res := get(t, ts.URL+"/api/sessions/"+id+"/results")
	if res["finished"] != true {
		t.Fatalf("results not finished: %v", res)
	}
}

// Every game must be creatable and produce a readable state and tape.
func TestAllGamesCreateStateResults(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	cases := []struct {
		game    string
		players []string
		config  map[string]any
	}{
		{"fruit", []string{"p1"}, map[string]any{"duration_ms": 60000}},
		{"math", []string{"p1"}, map[string]any{"duration_ms": 60000, "count": 50}},
		{"cards", []string{"p1"}, map[string]any{"prep_ms": 100, "quote_timer_ms": 200, "response_timer_ms": 200, "center_count": 1}},
		{"marble", []string{"p1"}, map[string]any{"sim_duration_ms": 300000, "playback_x": 100, "side_count": 2, "window_step_ms": 1000, "window_len_ms": 800}},
		{"etf", []string{"p1"}, map[string]any{"duration_ms": 30000, "quote_step_ms": 1000}},
		{"nextcard", []string{"p1"}, map[string]any{"suits": 1, "cycles": 1, "betting_timer_ms": 500, "result_timer_ms": 200}},
		{"prob", []string{"p1"}, map[string]any{"rounds": 3, "betting_timer_ms": 500, "result_timer_ms": 200}},
		{"fermi", []string{"p1"}, map[string]any{"rounds": 2, "question_timer_ms": 500, "reveal_ms": 200}},
		{"makeme", []string{"p1"}, map[string]any{"rounds": 1, "quote_timer_ms": 500, "solo_pause_ms": 100, "question_timer_ms": 300}},
	}
	for _, c := range cases {
		code, body := post(t, ts.URL+"/api/sessions", map[string]any{
			"game": c.game, "players": c.players, "seed": uint64(1000), "config": c.config,
		})
		if code != 201 {
			t.Fatalf("create %s: %d %v", c.game, code, body)
		}
		id := body["session_id"].(string)
		code, st := get(t, ts.URL+"/api/sessions/"+id+"/state?player=p1")
		if code != 200 || st["state"] == nil {
			t.Fatalf("state %s: %d %v", c.game, code, st)
		}
		// Tape endpoint.
		code, _ = get(t, ts.URL+"/api/sessions/"+id+"/tape")
		if code != 200 {
			t.Fatalf("tape %s: %d", c.game, code)
		}
		code, res := get(t, ts.URL+"/api/sessions/"+id+"/results")
		if code != 200 {
			t.Fatalf("results %s: %d", c.game, code)
		}
		_ = res
	}
}

// Racing two trades for one remaining lot: exactly one fill.
func TestRaceForOneLot(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	_, body := post(t, ts.URL+"/api/sessions", map[string]any{
		"game": "cards", "players": []string{"p1", "p2", "p3"}, "seed": 7,
		"config": map[string]any{"prep_ms": 50, "quote_timer_ms": 5000, "response_timer_ms": 5000, "center_count": 1},
	})
	id := body["session_id"].(string)
	time.Sleep(120 * time.Millisecond) // let PREP elapse (50ms)
	// p1 (alphabetically first maker) quotes 50/55 size 1.
	_, out := post(t, ts.URL+"/api/sessions/"+id+"/action", map[string]any{
		"action_id": "q", "player_id": "p1", "action": "QUOTE",
		"payload": map[string]any{"bid": 50, "ask": 55},
	})
	if out["output"].(map[string]any)["accepted"] != true {
		t.Fatalf("quote %+v", out)
	}
	// p2 and p3 race to buy the single lot.
	var wg sync.WaitGroup
	results := make([]map[string]any, 2)
	for i, p := range []string{"p2", "p3"} {
		wg.Add(1)
		go func(i int, p string) {
			defer wg.Done()
			_, o := post(t, ts.URL+"/api/sessions/"+id+"/action", map[string]any{
				"action_id": "buy-" + p, "player_id": p, "action": "BUY",
				"payload": map[string]any{"quantity": 1},
			})
			results[i] = o["output"].(map[string]any)
		}(i, p)
	}
	wg.Wait()
	accepted, rejected := 0, 0
	for _, o := range results {
		if o["accepted"] == true {
			accepted++
		} else {
			rejected++
			if o["reason"] != "NO_CAPACITY" {
				t.Fatalf("race loser reason %v", o["reason"])
			}
		}
	}
	if accepted != 1 || rejected != 1 {
		t.Fatalf("race accepted=%d rejected=%d, want 1/1", accepted, rejected)
	}
}

func TestSessionRestoreMetadataAndPlayerValidation(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	code, created := post(t, ts.URL+"/api/sessions", map[string]any{"game": "math", "players": []string{"alice", "bob"}, "seed": 42})
	if code != http.StatusCreated {
		t.Fatalf("create: %d %v", code, created)
	}
	id := created["session_id"].(string)
	code, state := get(t, ts.URL+"/api/sessions/"+id+"/state?player=bob")
	if code != http.StatusOK || state["game"] != "math" || len(state["players"].([]any)) != 2 {
		t.Fatalf("restore metadata missing: %d %v", code, state)
	}
	for _, players := range [][]string{{"alice", "alice"}, {" "}, {""}} {
		code, _ := post(t, ts.URL+"/api/sessions", map[string]any{"game": "math", "players": players})
		if code != http.StatusBadRequest {
			t.Fatalf("accepted invalid players: %v", players)
		}
	}
	code, body := post(t, ts.URL+"/api/sessions", map[string]any{})
	if code != http.StatusBadRequest || body["error"] != "missing game" {
		t.Fatalf("missing game: %d %v", code, body)
	}
}

func TestEveryGameAcceptsItsBoardAction(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	for _, slug := range []string{"fruit", "math", "cards", "marble", "etf", "nextcard", "prob", "fermi", "makeme"} {
		t.Run(slug, func(t *testing.T) {
			cfg := map[string]any{}
			if slug == "cards" {
				cfg["prep_ms"] = 1
			}
			code, created := post(t, ts.URL+"/api/sessions", map[string]any{"game": slug, "players": []string{"p1"}, "seed": 42, "config": cfg})
			if code != 201 {
				t.Fatalf("create: %d %v", code, created)
			}
			id := created["session_id"].(string)
			if slug == "cards" {
				time.Sleep(3 * time.Millisecond)
			}
			_, stateBody := get(t, ts.URL+"/api/sessions/"+id+"/state?player=p1")
			state := stateBody["state"].(map[string]any)
			action := "BUY"
			payload := map[string]any{"quantity": 1}
			switch slug {
			case "math":
				action = "ANSWER"
				payload = map[string]any{"question_id": state["question"].(map[string]any)["question_id"], "answer": "0"}
			case "cards":
				action = "QUOTE"
				payload = map[string]any{"bid": 50, "ask": 55}
			case "marble":
				payload["contract_id"] = "MAIN"
			case "etf":
				action = "BUY_ETF"
				payload["etf_id"] = "E1"
			case "nextcard":
				action = "BET"
				payload = map[string]any{"prediction": "HIGHER", "stake": 10}
			case "prob":
				action = "SUBMIT_BETS"
				event := state["events"].([]any)[0].(map[string]any)["event_id"]
				payload = map[string]any{"bets": []map[string]any{{"event_id": event, "stake": 10}}}
			case "fermi":
				action = "ESTIMATE"
				payload = map[string]any{"question_id": state["question"].(map[string]any)["question_id"], "value": 100}
			case "makeme":
				action = "QUOTE"
				payload = map[string]any{"bid": 1, "ask": 1, "bid_size": 1, "ask_size": 1}
			}
			code, body := post(t, ts.URL+"/api/sessions/"+id+"/action", map[string]any{"player_id": "p1", "action_id": "ui-action", "market_version": state["market_version"], "action": action, "payload": payload})
			if code != 200 || body["output"].(map[string]any)["accepted"] != true {
				t.Fatalf("board action rejected: %d %v", code, body)
			}
			code, body = get(t, ts.URL+"/api/sessions/"+id+"/state?player=p1")
			if code != 200 || body["state"] == nil {
				t.Fatalf("post-action state: %d %v", code, body)
			}
		})
	}
}

func TestAssessmentHTTPPrivacyIdempotencyAndReplay(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	code, catalog := get(t, ts.URL+"/api/assessments")
	if code != 200 || len(catalog["profiles"].([]any)) != 25 {
		t.Fatalf("catalog: %d %v", code, catalog)
	}
	code, created := post(t, ts.URL+"/api/sessions", map[string]any{"game": "assessment", "players": []string{"alice"}, "seed": 42, "config": map[string]any{"profile_id": "drw-assessment", "mode": "preview"}})
	if code != 201 {
		t.Fatalf("create %d %v", code, created)
	}
	id := created["session_id"].(string)
	for _, path := range []string{"results", "tape"} {
		code, body := get(t, ts.URL+"/api/sessions/"+id+"/"+path)
		if code != 409 || body["error"] == nil {
			t.Fatalf("live %s exposed: %d %v", path, code, body)
		}
	}
	_, body := get(t, ts.URL+"/api/sessions/"+id+"/state?player=alice")
	question := body["state"].(map[string]any)["question"].(map[string]any)
	if question["answer"] != nil || question["explanation"] != nil || question["model"] != nil {
		t.Fatal("answer key leaked")
	}
	qid := question["question_id"]
	request := map[string]any{"action_id": "save-1", "player_id": "alice", "action": "SAVE_ANSWER", "payload": map[string]any{"question_id": qid, "response": map[string]any{"answer": "a"}}}
	_, receipt := post(t, ts.URL+"/api/sessions/"+id+"/action", request)
	if receipt["output"].(map[string]any)["accepted"] != true {
		t.Fatal(receipt)
	}
	_, receipt = post(t, ts.URL+"/api/sessions/"+id+"/action", request)
	if receipt["output"].(map[string]any)["reason"] != "DUPLICATE" {
		t.Fatal("not idempotent")
	}
	_, restored := get(t, ts.URL+"/api/sessions/"+id+"/state?player=alice")
	if restored["state"].(map[string]any)["saved_response"].(map[string]any)["answer"] != "a" {
		t.Fatal("saved response not restored")
	}
	_, receipt = post(t, ts.URL+"/api/sessions/"+id+"/action", map[string]any{"action_id": "finish", "player_id": "alice", "action": "SUBMIT_TEST"})
	if receipt["output"].(map[string]any)["accepted"] != true {
		t.Fatal(receipt)
	}
	code, tape := get(t, ts.URL+"/api/sessions/"+id+"/tape")
	if code != 200 {
		t.Fatalf("tape: %d %v", code, tape)
	}
	code, replay := post(t, ts.URL+"/api/assessments/replay", tape)
	if code != 200 || replay["verified"] != true {
		t.Fatalf("replay: %d %v", code, replay)
	}
}

func TestTrainerHTTPPrivacyCatalogAndReplay(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	code, catalog := get(t, ts.URL+"/api/trainers")
	if code != 200 || len(catalog["profiles"].([]any)) != 22 {
		t.Fatal(code, catalog)
	}
	code, library := get(t, ts.URL+"/api/trainers/library")
	if code != 200 || len(library["items"].([]any)) != 24 {
		t.Fatal(code, library)
	}
	for _, item := range library["items"].([]any) {
		if item.(map[string]any)["answer"] != nil {
			t.Fatal("library answer leak")
		}
	}
	code, created := post(t, ts.URL+"/api/sessions", map[string]any{"game": "trainer", "players": []string{"alice"}, "seed": 42, "config": map[string]any{"trainer_id": "pincode", "mode": "repeat"}})
	if code != 201 {
		t.Fatal(code, created)
	}
	id := created["session_id"].(string)
	for _, path := range []string{"results", "tape"} {
		code, _ = get(t, ts.URL+"/api/sessions/"+id+"/"+path)
		if code != 409 {
			t.Fatal("early review", path, code)
		}
	}
	for i, action := range []string{"BEGIN", "END_SESSION"} {
		code, body := post(t, ts.URL+"/api/sessions/"+id+"/action", map[string]any{"player_id": "alice", "action_id": fmt.Sprintf("trainer-action-%d", i), "action": action, "payload": map[string]any{"elapsed_ms": 0}})
		if code != 200 || body["output"].(map[string]any)["accepted"] != true {
			t.Fatal(code, body)
		}
	}
	code, tape := get(t, ts.URL+"/api/sessions/"+id+"/tape")
	if code != 200 {
		t.Fatal(code, tape)
	}
	code, replayed := post(t, ts.URL+"/api/trainers/replay", tape)
	if code != 200 || replayed["verified"] != true {
		t.Fatal(code, replayed)
	}
}
