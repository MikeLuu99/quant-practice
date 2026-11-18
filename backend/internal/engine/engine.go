// Package engine provides the shared deterministic game infrastructure:
// seeded PRNG, action envelopes, the Game interface, registry and session store.
package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"
)

var ErrNotFinished = errors.New("review and replay are available after the session ends")

// ---------------------------------------------------------------------------
// Deterministic RNG: SplitMix64. Fully portable, versioned as "splitmix64-v1".
// ---------------------------------------------------------------------------

type RNG struct{ s uint64 }

func NewRNG(seed uint64) *RNG { return &RNG{s: seed} }

func (r *RNG) Next() uint64 {
	r.s += 0x9E3779B97F4A7C15
	z := r.s
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31)
}

// Int returns uniform in [0,n).
func (r *RNG) Int(n int) int {
	if n <= 1 {
		return 0
	}
	return int(r.Next() % uint64(n))
}

// Range returns uniform integer in [lo,hi] inclusive.
func (r *RNG) Range(lo, hi int) int { return lo + r.Int(hi-lo+1) }

// Float returns uniform in [0,1).
func (r *RNG) Float() float64 { return float64(r.Next()>>11) / (1 << 53) }

// Exp returns an exponential sample with the given mean.
func (r *RNG) Exp(mean float64) float64 {
	u := r.Float()
	if u <= 0 {
		u = 1e-12
	}
	return -math.Log(1-u) * mean
}

func (r *RNG) Shuffle(n int, swap func(i, j int)) {
	for i := n - 1; i > 0; i-- {
		j := r.Int(i + 1)
		swap(i, j)
	}
}

// ---------------------------------------------------------------------------
// Context passed to every game.
// ---------------------------------------------------------------------------

type Ctx struct {
	Players []string
	Seed    uint64
	Start   time.Time
	// NowFn optionally overrides the wall clock (used by tests to drive
	// simulated time deterministically).
	NowFn func() time.Time
}

func (c *Ctx) now() time.Time {
	if c.NowFn != nil {
		return c.NowFn()
	}
	return time.Now()
}

// ElapsedMS returns server-authoritative elapsed milliseconds.
func (c *Ctx) ElapsedMS() int64 { return c.now().Sub(c.Start).Milliseconds() }

// ---------------------------------------------------------------------------
// Action envelope. Every accepted command gets a unique action_id; retries
// return the original receipt without re-executing.
// ---------------------------------------------------------------------------

type ActionRequest struct {
	ActionID      string          `json:"action_id"`
	PlayerID      string          `json:"player_id"`
	RoundID       int             `json:"round_id,omitempty"`
	MarketVersion int             `json:"market_version,omitempty"`
	Action        string          `json:"action"`
	Payload       json.RawMessage `json:"payload,omitempty"`
}

type Output struct {
	Accepted       bool            `json:"accepted"`
	Reason         string          `json:"reason,omitempty"`
	ServerSequence int64           `json:"server_sequence"`
	Execution      json.RawMessage `json:"execution,omitempty"`
	PublicChanges  json.RawMessage `json:"public_changes,omitempty"`
	PrivateChanges json.RawMessage `json:"private_changes,omitempty"`
	RoundReward    float64         `json:"round_reward,omitempty"`
	GameOver       bool            `json:"game_over"`
}

// Rejection reasons.
const (
	ReasonLate               = "LATE"
	ReasonStaleMarket        = "STALE_MARKET"
	ReasonInvalidValue       = "INVALID_VALUE"
	ReasonInsufficient       = "INSUFFICIENT_BALANCE"
	ReasonAlreadyActed       = "ALREADY_ACTED"
	ReasonNoCapacity         = "NO_CAPACITY"
	ReasonNotYourTurn        = "NOT_YOUR_TURN"
	ReasonRateLimit          = "RATE_LIMIT"
	ReasonUnknownPlayer      = "UNKNOWN_PLAYER"
	ReasonUnknownAction      = "UNKNOWN_ACTION"
	ReasonMarketClosed       = "MARKET_CLOSED"
	ReasonInsufficientStakes = "INSUFFICIENT_STAKES"
)

// ---------------------------------------------------------------------------
// Game interface.
// ---------------------------------------------------------------------------

type GameMeta struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Access      string `json:"access"`
	MinPlayers  int    `json:"min_players"`
	MaxPlayers  int    `json:"max_players"`
	DurationMS  int64  `json:"duration_ms"`
	Description string `json:"description"`
	Rules       string `json:"rules"`
}

type Game interface {
	Meta() GameMeta
	// Snapshot returns the merged public + per-player private observation.
	Snapshot(playerID string) (json.RawMessage, error)
	// Apply processes one command (already idempotency-filtered).
	Apply(req ActionRequest, seq int64) Output
	Finished() bool
	Results() (json.RawMessage, error)
	Tape() (json.RawMessage, error)
}

// Ticker is an optional interface for time-driven games (bots, timeouts).
type Ticker interface {
	Tick()
}

// ---------------------------------------------------------------------------
// Registry.
// ---------------------------------------------------------------------------

type Constructor func(cfg json.RawMessage, ctx *Ctx) (Game, error)

var registry = map[string]Constructor{}

func Register(slug string, c Constructor) { registry[slug] = c }

func Constructors() []string {
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	return out
}

func NewGame(slug string, cfg json.RawMessage, players []string, seed uint64, start time.Time) (Game, error) {
	c, ok := registry[slug]
	if !ok {
		return nil, fmt.Errorf("unknown game %q", slug)
	}
	return c(cfg, &Ctx{Players: players, Seed: seed, Start: start})
}

// NewGameWithContext supports an authoritative replay clock at the hosting boundary.
func NewGameWithContext(slug string, cfg json.RawMessage, ctx *Ctx) (Game, error) {
	c, ok := registry[slug]
	if !ok {
		return nil, fmt.Errorf("unknown game %q", slug)
	}
	return c(cfg, ctx)
}

// ---------------------------------------------------------------------------
// Session store with idempotent action processing.
// ---------------------------------------------------------------------------

type Session struct {
	ID        string    `json:"id"`
	Slug      string    `json:"game"`
	Players   []string  `json:"players"`
	CreatedAt time.Time `json:"created_at"`

	game     Game
	mu       sync.Mutex
	seq      int64
	receipts map[string]Output
}

func (s *Session) Apply(req ActionRequest) Output {
	s.mu.Lock()
	defer s.mu.Unlock()
	if req.ActionID != "" {
		if prev, ok := s.receipts[req.ActionID]; ok {
			prev.Reason = "DUPLICATE"
			return prev
		}
	}
	s.seq++
	out := s.game.Apply(req, s.seq)
	out.ServerSequence = s.seq
	if req.ActionID != "" && out.Accepted {
		s.receipts[req.ActionID] = out
	}
	return out
}

func (s *Session) Snapshot(playerID string) (json.RawMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.game.Snapshot(playerID)
}

func (s *Session) Finished() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.game.Finished()
}

func (s *Session) Results() (json.RawMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.game.Results()
}

func (s *Session) Tape() (json.RawMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.game.Tape()
}

func (s *Session) Tick() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t, ok := s.game.(Ticker); ok {
		t.Tick()
	}
}

type Store struct {
	mu       sync.Mutex
	sessions map[string]*Session
	next     int64
	nowFn    func() time.Time
	idFn     func() string
}

func NewStore() *Store { return NewStoreWithClock(time.Now, nil) }

// NewStoreWithClock is used by durable per-session hosts. The normal server retains
// its wall clock and sequential IDs; a durable host supplies one opaque room ID.
func NewStoreWithClock(now func() time.Time, newID func() string) *Store {
	return &Store{sessions: map[string]*Session{}, nowFn: now, idFn: newID}
}

func (st *Store) Create(slug string, cfg json.RawMessage, players []string, seed uint64) (*Session, error) {
	seen := make(map[string]bool, len(players))
	for _, player := range players {
		if strings.TrimSpace(player) == "" || seen[player] {
			return nil, fmt.Errorf("player names must be nonempty and unique")
		}
		seen[player] = true
	}
	started := st.nowFn()
	g, err := NewGameWithContext(slug, cfg, &Ctx{Players: players, Seed: seed, Start: started, NowFn: st.nowFn})
	if err != nil {
		return nil, err
	}
	meta := g.Meta()
	if len(players) < meta.MinPlayers || len(players) > meta.MaxPlayers {
		return nil, fmt.Errorf("%s supports %d–%d players", meta.Name, meta.MinPlayers, meta.MaxPlayers)
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	st.next++
	id := fmt.Sprintf("s-%d", st.next)
	if st.idFn != nil {
		id = st.idFn()
	}
	if _, exists := st.sessions[id]; exists {
		return nil, fmt.Errorf("session already exists")
	}
	s := &Session{ID: id, Slug: slug, Players: players, CreatedAt: started, game: g, receipts: map[string]Output{}}
	st.sessions[id] = s
	return s, nil
}

func (st *Store) Get(id string) (*Session, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	s, ok := st.sessions[id]
	return s, ok
}

// TickAll drives timeouts and bots for every live session.
func (st *Store) TickAll() {
	st.mu.Lock()
	list := make([]*Session, 0, len(st.sessions))
	for _, s := range st.sessions {
		list = append(list, s)
	}
	st.mu.Unlock()
	for _, s := range list {
		s.Tick()
	}
}

// ---------------------------------------------------------------------------
// Misc helpers.
// ---------------------------------------------------------------------------

func MustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func Round2(x float64) float64 { return math.Round(x*100) / 100 }

func Clamp(x, lo, hi float64) float64 {
	if x < lo {
		return lo
	}
	if x > hi {
		return hi
	}
	return x
}

// NextWake replaces a process-wide polling goroutine on request-scoped hosts.
// Game-specific absolute deadlines avoid waking untimed practice sessions.
func (s *Session) NextWake(now time.Time) time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	if wake, ok := s.game.(interface{ NextWake() time.Time }); ok {
		return wake.NextWake()
	}
	return now.Add(time.Second)
}
