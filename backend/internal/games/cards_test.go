package games

import (
	"encoding/json"
	"testing"

	"quantgame/backend/internal/engine"
)

func cardsForTest(t *testing.T, cfg string, seed uint64, players ...string) (*cardsGame, *fakeClock) {
	t.Helper()
	fc := &fakeClock{}
	var cfgRaw json.RawMessage
	if cfg != "" {
		cfgRaw = json.RawMessage(cfg)
	}
	g, err := newCards(cfgRaw, fc.ctx(players, seed))
	if err != nil {
		t.Fatalf("newCards: %v", err)
	}
	return g.(*cardsGame), fc
}

func cardsCfg() string {
	return `{"prep_ms":100,"quote_timer_ms":200,"response_timer_ms":200,"center_count":1,"max_spread":20}`
}

func cardsQuote(g *cardsGame, p string, bid, ask int) engine.Output {
	return g.Apply(engine.ActionRequest{ActionID: p + "q", PlayerID: p, Action: "QUOTE",
		Payload: engine.MustJSON(map[string]any{"bid": bid, "ask": ask})}, 1)
}

func cardsTrade(g *cardsGame, p, side string, id string) engine.Output {
	if id == "" {
		id = p + side
	}
	return g.Apply(engine.ActionRequest{ActionID: id, PlayerID: p, MarketVersion: 1, Action: side,
		Payload: engine.MustJSON(map[string]any{"quantity": 1})}, 1)
}

func TestCardsDealConservation(t *testing.T) {
	g, _ := cardsForTest(t, cardsCfg(), 1, "p1", "p2", "p3")
	seen := map[int]bool{}
	n := len(g.playerOrder)
	for i, p := range g.playerOrder {
		hand := g.tape.Hands[p]
		if len(hand) != 2 {
			t.Fatalf("hand size %d", len(hand))
		}
		if hand[0] != g.tape.Deck[2*i] || hand[1] != g.tape.Deck[2*i+1] {
			t.Fatalf("hand of %s not dealt from deck order", p)
		}
		for _, c := range hand {
			key := c.Rank*4 + c.Suit
			if seen[key] {
				t.Fatalf("card %v dealt twice", c)
			}
			seen[key] = true
		}
	}
	for i, c := range g.tape.Center {
		if c != g.tape.Deck[2*n+i] {
			t.Fatalf("center card %d not from deck order", i)
		}
		if seen[c.Rank*4+c.Suit] {
			t.Fatalf("center card %v duplicated", c)
		}
	}
	if len(g.tape.Deck) != 52 {
		t.Fatal("deck must be 52 cards")
	}
}

func TestCardsFirstResponderGetsFill(t *testing.T) {
	g, fc := cardsForTest(t, cardsCfg(), 2, "p1", "p2", "p3")
	fc.advance(150) // PREP over, maker p1 (sorted order)
	if out := cardsQuote(g, "p1", 50, 55); !out.Accepted {
		t.Fatalf("quote rejected: %s", out.Reason)
	}
	if g.phase != "RESPONSE" {
		t.Fatalf("phase %s, want RESPONSE", g.phase)
	}
	out := cardsTrade(g, "p2", "BUY", "")
	if !out.Accepted {
		t.Fatalf("first responder BUY rejected: %s", out.Reason)
	}
	// Second concurrent responder must get NO_CAPACITY (market closed).
	out = cardsTrade(g, "p3", "BUY", "")
	if out.Accepted || out.Reason != engine.ReasonNoCapacity {
		t.Fatalf("second responder must get NO_CAPACITY, got %+v", out)
	}
	// Bookkeeping: p2 long 1 at 55; p1 (maker) short 1.
	if b := g.books["p2"]; b.C != -55 || b.Q != 1 {
		t.Fatalf("p2 book %+v", b)
	}
	if b := g.books["p1"]; b.C != 55 || b.Q != -1 {
		t.Fatalf("p1 book %+v", b)
	}
}

func TestCardsMakerCannotRespondAndValidation(t *testing.T) {
	g, fc := cardsForTest(t, cardsCfg(), 3, "p1", "p2")
	fc.advance(150)
	if out := cardsQuote(g, "p2", 50, 55); out.Accepted {
		t.Fatal("non-maker quote must be rejected")
	}
	if out := cardsQuote(g, "p1", 55, 50); out.Accepted || out.Reason != engine.ReasonInvalidValue {
		t.Fatalf("crossed quote must fail INVALID_VALUE, got %+v", out)
	}
	if out := cardsQuote(g, "p1", 50, 80); out.Accepted || out.Reason != engine.ReasonInvalidValue {
		t.Fatalf("over-wide quote must fail INVALID_VALUE, got %+v", out)
	}
	if out := cardsQuote(g, "p1", 50, 55); !out.Accepted {
		t.Fatalf("valid quote rejected: %s", out.Reason)
	}
	// Maker cannot respond to their own quote.
	out := cardsTrade(g, "p1", "BUY", "")
	if out.Accepted || out.Reason != engine.ReasonNotYourTurn {
		t.Fatalf("maker response must fail NOT_YOUR_TURN, got %+v", out)
	}
	// A pass consumes only that responder's action; no later trade for them.
	if out := cardsTrade(g, "p2", "PASS", ""); !out.Accepted {
		t.Fatalf("pass rejected: %s", out.Reason)
	}
	if out := cardsTrade(g, "p2", "PASS", ""); out.Accepted || out.Reason != engine.ReasonAlreadyActed {
		t.Fatalf("double pass must fail ALREADY_ACTED, got %+v", out)
	}
	if out := cardsTrade(g, "p2", "BUY", ""); out.Accepted || out.Reason != engine.ReasonAlreadyActed {
		t.Fatalf("trade after pass must fail ALREADY_ACTED, got %+v", out)
	}
}

func TestCardsRotationRevealAndZeroSumSettlement(t *testing.T) {
	g, fc := cardsForTest(t, cardsCfg(), 4, "p1", "p2")
	fc.advance(150)
	// Maker p1 quotes 50/55; p2 buys at 55.
	_ = cardsQuote(g, "p1", 50, 55)
	_ = cardsTrade(g, "p2", "BUY", "")
	// Maker p2 quotes 40/45; p1 sells at 40.
	fc.advance(10)
	if out := cardsQuote(g, "p2", 40, 45); !out.Accepted {
		t.Fatalf("p2 quote rejected: %s", out.Reason)
	}
	if out := cardsTrade(g, "p1", "SELL", ""); !out.Accepted {
		t.Fatalf("p1 sell rejected: %s", out.Reason)
	}
	// Rotation complete (2 players, 1 rotation, center_count 1): revealed.
	if g.revealed != 1 {
		t.Fatalf("revealed %d, want 1 after full rotation", g.revealed)
	}
	if !g.Finished() {
		t.Fatal("game must finish after the last reveal")
	}
	res, err := g.Results()
	if err != nil {
		t.Fatalf("zero-sum check failed: %v", err)
	}
	var r map[string]any
	_ = json.Unmarshal(res, &r)
	V := int(r["value"].(float64))
	if V != g.totalDealtValue() {
		t.Fatal("settlement value mismatch")
	}
	rows := r["players"].([]any)
	sum := 0
	for _, row := range rows {
		p := row.(map[string]any)
		sum += int(p["profit"].(float64))
	}
	if sum != 0 {
		t.Fatalf("profits not zero-sum: %d", sum)
	}
	// Revealed hands in results.
	if _, ok := r["hands"].(map[string]any)["p1"]; !ok {
		t.Fatal("results must reveal all hands")
	}
}

func TestCardsBotsAutoPlayZeroSum(t *testing.T) {
	g, fc := cardsForTest(t, cardsCfg(), 5, "p1")
	if len(g.playerOrder) != 3 {
		t.Fatalf("player order %v, want p1 + 2 bots", g.playerOrder)
	}
	for i := 0; i < 60 && !g.Finished(); i++ {
		fc.advance(250)
		g.Snapshot("p1")
	}
	if !g.Finished() {
		t.Fatal("bots should complete the game")
	}
	res, err := g.Results()
	if err != nil {
		t.Fatalf("results error (zero-sum?): %v", err)
	}
	var r map[string]any
	_ = json.Unmarshal(res, &r)
	sum := 0
	for _, row := range r["players"].([]any) {
		p := row.(map[string]any)
		sum += int(p["profit"].(float64))
		if p["player"] == "bot1" && p["is_bot"] != true {
			t.Fatal("bot classification missing")
		}
	}
	if sum != 0 {
		t.Fatalf("bot game not zero-sum: %d", sum)
	}
}

func TestCardsSnapshotPrivacy(t *testing.T) {
	g, _ := cardsForTest(t, cardsCfg(), 6, "p1", "p2")
	snap, _ := g.Snapshot("p1")
	var s map[string]any
	_ = json.Unmarshal(snap, &s)
	// Own two cards visible...
	if len(s["my_hand"].([]any)) != 2 {
		t.Fatal("own hand missing")
	}
	// No center cards may be revealed before a rotation completes.
	if len(s["revealed_center"].([]any)) != 0 {
		t.Fatal("center cards leaked before reveal")
	}
	// Opponents' private cards must never appear: check every p2 label.
	for _, c := range g.tape.Hands["p2"] {
		if containsSubstr(string(snap), c.label()) {
			t.Fatalf("snapshot leaks opponent card %s", c.label())
		}
	}
}

func containsSubstr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
