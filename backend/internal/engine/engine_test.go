package engine

import (
	"encoding/json"
	"testing"
	"time"
)

func TestRNGDeterministic(t *testing.T) {
	a, b := NewRNG(42), NewRNG(42)
	for i := 0; i < 1000; i++ {
		if a.Next() != b.Next() {
			t.Fatalf("same seed diverged at draw %d", i)
		}
	}
	c, d := NewRNG(43), NewRNG(44)
	if c.Next() == d.Next() && c.Next() == d.Next() {
		t.Fatal("different seeds must diverge")
	}
}

func TestRNGBounds(t *testing.T) {
	r := NewRNG(7)
	for i := 0; i < 10000; i++ {
		if v := r.Range(-3, 5); v < -3 || v > 5 {
			t.Fatalf("Range out of bounds: %d", v)
		}
		if v := r.Int(11); v < 0 || v >= 11 {
			t.Fatalf("Int out of bounds: %d", v)
		}
		if v := r.Float(); v < 0 || v >= 1 {
			t.Fatalf("Float out of bounds: %f", v)
		}
	}
}

func TestRNGExpMean(t *testing.T) {
	r := NewRNG(9)
	const n = 200000
	var sum float64
	for i := 0; i < n; i++ {
		sum += r.Exp(2.5)
	}
	mean := sum / n
	if mean < 2.3 || mean > 2.7 {
		t.Fatalf("exponential mean off: %f", mean)
	}
}

// stub game used to test session-level idempotency.
type stubGame struct{ calls int }

func (s *stubGame) Meta() GameMeta { return GameMeta{Slug: "stub"} }
func (s *stubGame) Snapshot(string) (json.RawMessage, error) { return nil, nil }
func (s *stubGame) Apply(req ActionRequest, seq int64) Output {
	s.calls++
	return Output{Accepted: true, RoundReward: 1}
}
func (s *stubGame) Finished() bool             { return false }
func (s *stubGame) Results() (json.RawMessage, error) { return nil, nil }
func (s *stubGame) Tape() (json.RawMessage, error)    { return nil, nil }

func TestSessionActionIdempotency(t *testing.T) {
	g := &stubGame{}
	s := &Session{ID: "t", game: g, receipts: map[string]Output{}}
	out1 := s.Apply(ActionRequest{ActionID: "a-1", PlayerID: "p", Action: "BUY"})
	if !out1.Accepted || out1.ServerSequence != 1 {
		t.Fatalf("first apply: %+v", out1)
	}
	out2 := s.Apply(ActionRequest{ActionID: "a-1", PlayerID: "p", Action: "BUY"})
	if out2.ServerSequence != 1 || out2.Reason != "DUPLICATE" {
		t.Fatalf("retry must return original receipt, got %+v", out2)
	}
	if g.calls != 1 {
		t.Fatalf("game applied %d times, want 1", g.calls)
	}
	out3 := s.Apply(ActionRequest{ActionID: "a-2", PlayerID: "p", Action: "BUY"})
	if out3.ServerSequence != 2 {
		t.Fatalf("new action must get new sequence, got %+v", out3)
	}
	if g.calls != 2 {
		t.Fatalf("game applied %d times, want 2", g.calls)
	}
}

func TestCtxFakeClock(t *testing.T) {
	ms := int64(0)
	ctx := &Ctx{Start: time.Unix(0, 0), NowFn: func() time.Time {
		return time.Unix(0, ms*int64(time.Millisecond))
	}}
	if ctx.ElapsedMS() != 0 {
		t.Fatal("expected 0 elapsed")
	}
	ms = 5500
	if ctx.ElapsedMS() != 5500 {
		t.Fatalf("expected 5500 elapsed, got %d", ctx.ElapsedMS())
	}
}
