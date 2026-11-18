package games

import (
	"fmt"
	"math"
	"slices"
	"strconv"
)

func (g *trainerGame) firstMove() {
	if g.move.FirstMoveMS == nil {
		v := g.simMS - g.responseOnset
		g.move.FirstMoveMS = &v
	}
}
func (g *trainerGame) specialAction(action string, p trainerPayload, result map[string]any) string {
	t := g.current()
	result["points"] = 0
	correct := func(points float64) {
		result["correct"] = true
		result["status"] = "correct"
		result["points"] = points
		g.completeTrial(result)
	}
	switch t.Kind {
	case "target-number":
		if action == "UNDO" {
			if len(g.undo) == 0 {
				return "NO_UNDO"
			}
			g.move = g.undo[len(g.undo)-1]
			g.undo = g.undo[:len(g.undo)-1]
			return ""
		}
		if action == "RESET" {
			g.move.Tiles = trainerDecode[[]trainerTile](t.Public["sources"])
			g.undo = nil
			return ""
		}
		if action != "COMBINE" {
			return "UNKNOWN_ACTION"
		}
		a, b := -1, -1
		for i, tile := range g.move.Tiles {
			if tile.ID == p.Left {
				a = i
			}
			if tile.ID == p.Right {
				b = i
			}
		}
		if a < 0 || b < 0 || a == b {
			return "INVALID_TILES"
		}
		left, right := g.move.Tiles[a], g.move.Tiles[b]
		value, ok := numberCombine(left.Value, right.Value, p.Operator)
		if !ok {
			return "ILLEGAL_ARITHMETIC"
		}
		g.firstMove()
		g.undo = append(g.undo, trainerClone(g.move))
		g.move.Moves++
		next := []trainerTile{}
		for i, tile := range g.move.Tiles {
			if i != a && i != b {
				next = append(next, tile)
			}
		}
		id := fmt.Sprintf("r%d", len(g.undo)-1)
		next = append(next, trainerTile{ID: id, Value: value, Expression: "(" + left.Expression + p.Operator + right.Expression + ")", Sources: append(append([]string{}, left.Sources...), right.Sources...)})
		g.move.Tiles = next
		if len(next) == 1 {
			result["expression"] = next[0].Expression
			if value == trainerInt(t.Public["target"]) && len(next[0].Sources) == 4 {
				correct(1)
			} else {
				result["status"] = "incorrect_final"
				result["points"] = 0
				g.completeTrial(result)
			}
		}
	case "skyline":
		if action == "RESET" {
			g.move.Stacks = trainerDecode[[][]string](t.Public["start"])
			g.undo = nil
			return ""
		}
		if action == "UNDO" {
			if len(g.undo) == 0 {
				return "NO_UNDO"
			}
			previous := g.undo[len(g.undo)-1]
			g.undo = g.undo[:len(g.undo)-1]
			g.move.Stacks = previous.Stacks
			g.move.Moves++
			return ""
		}
		if action != "MOVE" {
			return "UNKNOWN_ACTION"
		}
		next, ok := skylineMoveState(g.move.Stacks, trainerDecode[[]int](t.Public["capacities"]), p.From, p.To)
		if !ok {
			return "ILLEGAL_MOVE"
		}
		g.firstMove()
		g.undo = append(g.undo, trainerClone(g.move))
		g.move.Stacks = next
		g.move.Moves++
		if skylineKey(next) == skylineKey(trainerDecode[[][]string](t.Public["goal"])) {
			opt := trainerInt(t.Public["minimum_moves"])
			result["minimum_moves"], result["excess_moves"], result["efficiency"] = opt, g.move.Moves-opt, float64(opt)/float64(g.move.Moves)
			points := 0.0
			if g.move.Moves == opt {
				points = 1
			}
			correct(points)
		}
	case "mystery":
		if action != "GUESS" {
			return "UNKNOWN_ACTION"
		}
		attrs := trainerDecode[[]map[string]any](t.Public["attributes"])
		if len(p.Values) != len(attrs) {
			return "INVALID_FORMAT"
		}
		for i, v := range p.Values {
			if !slices.Contains(trainerDecode[[]string](attrs[i]["values"]), v) {
				return "INVALID_FORMAT"
			}
		}
		all := mysteryAll(attrs)
		candidates := mysteryConsistent(all, g.move.Guesses, g.move.Counts)
		cost := mysteryPartitionCost(candidates, p.Values)
		best := mysteryBestCost(candidates, all)
		surviving := false
		for _, v := range candidates {
			if sameStrings(v, p.Values) {
				surviving = true
				break
			}
		}
		g.firstMove()
		count := mysteryCount(p.Values, trainerDecode[[]string](t.Secret["secret"]))
		g.move.Guesses = append(g.move.Guesses, p.Values)
		g.move.Counts = append(g.move.Counts, count)
		g.move.Moves++
		g.feedback = map[string]any{"matching_attributes": count, "expected_remaining": cost, "reference_expected_remaining": best, "surviving_candidate_probe": surviving, "remaining_candidates": len(mysteryConsistent(all, g.move.Guesses, g.move.Counts))}
		if count == len(attrs) {
			result["guesses_to_solve"] = g.move.Moves
			result["last_probe"] = g.feedback
			correct(1)
		}
	case "pump":
		k := trainerInt(t.Secret["threshold"])
		cents := trainerInt(t.Public["cents_per_pump"])
		cash := func() {
			amount := g.move.Pumps * cents
			g.bank += amount
			g.metrics["cashed"]++
			g.metrics["cashed_pumps"] += float64(g.move.Pumps)
			result["status"] = "cashed"
			result["correct"] = true
			result["points"] = 0
			result["bank_delta_cents"] = amount
			result["threshold_observation"] = fmt.Sprintf("K > %d", g.move.Pumps)
			g.completeTrial(result)
		}
		pop := func() {
			penalty := 0
			if g.cfg.Mode == "stakes" {
				penalty = g.move.Pumps * cents / 2
				g.bank -= penalty
			}
			g.metrics["pops"]++
			result["status"] = "popped"
			result["points"] = 0
			result["bank_delta_cents"] = -penalty
			result["threshold"] = k
			g.completeTrial(result)
		}
		if action == "COMMIT_TARGET" {
			if g.cfg.Mode != "stakes" || p.Target < 0 || p.Target > 127 {
				return "INVALID_TARGET"
			}
			g.firstMove()
			if p.Target >= k {
				g.move.Pumps = k - 1
				pop()
			} else {
				g.move.Pumps = p.Target
				cash()
			}
			return ""
		}
		if g.cfg.Mode != "manual" {
			return "UNKNOWN_ACTION"
		}
		if action == "CASH_IN" {
			cash()
		} else if action == "PUMP" {
			g.firstMove()
			if g.move.Pumps+1 >= k {
				pop()
			} else {
				g.move.Pumps++
			}
		} else {
			return "UNKNOWN_ACTION"
		}
	case "sweet-spot":
		if action != "CLICK" || !finiteTrainer(p.X) || !finiteTrainer(p.Y) || p.X < 0 || p.X > 800 || p.Y < 0 || p.Y > 500 {
			return "INVALID_COORDINATES"
		}
		g.metrics["clicks"]++
		result["points"] = -1
		hit := -1
		for i, d := range g.dials {
			if math.Hypot(p.X-d.X, p.Y-d.Y) <= d.Radius {
				hit = i
				break
			}
		}
		row := map[string]any{"at_ms": g.simMS, "x": p.X, "y": p.Y, "status": "background_miss"}
		if hit < 0 {
			g.metrics["wrong_clicks"]++
			g.score--
		} else {
			d := g.dials[hit]
			error := math.Abs(trainerSignedAngle(d.Theta + d.Omega*float64(g.simMS-d.Spawn)/1000 - d.Target))
			row["dial_id"], row["angular_error"] = d.ID, error
			if error <= d.Width/2 {
				row["status"] = "hit"
				result["points"] = 1
				g.metrics["hits"]++
				g.metrics["angular_error_sum"] += error
				g.score++
				g.dials = append(g.dials[:hit], g.dials[hit+1:]...)
				g.fillDials(g.simMS)
			} else {
				row["status"] = "angular_miss"
				g.metrics["wrong_clicks"]++
				g.score--
			}
		}
		row["points"] = result["points"]
		g.history = append(g.history, row)
	default:
		if action == "QUOTE" && g.profile.Category == "Trading" {
			return g.applyTrainerQuote(p, result)
		}
		return "UNKNOWN_ACTION"
	}
	return ""
}
func (g *trainerGame) fillDials(at int64) {
	if at >= g.cfg.DurationMS {
		return
	}
	target := min(6, 2+int(at/24000))
	slots := [][2]float64{{140, 140}, {400, 140}, {660, 140}, {140, 370}, {400, 370}, {660, 370}}
	for len(g.dials) < target {
		slot := 0
		for ; slot < 6; slot++ {
			used := false
			for _, d := range g.dials {
				if d.Slot == slot {
					used = true
				}
			}
			if !used {
				break
			}
		}
		rng := trainerRNG(g.ctx.Seed, "dial", g.dialSerial, 0)
		g.dialSerial++
		d := trainerDial{ID: "dial-" + strconv.Itoa(g.dialSerial), Slot: slot, Spawn: at, Expires: at + int64(rng.Range(4000, 9000)), X: slots[slot][0], Y: slots[slot][1], Radius: 87, Theta: rng.Float() * 360, Omega: 180 + rng.Float()*180, Target: rng.Float() * 360, Width: 25 + rng.Float()*20}
		if rng.Int(2) == 0 {
			d.Omega = -d.Omega
		}
		g.dials = append(g.dials, d)
		g.metrics["spawned"]++
	}
}
func (g *trainerGame) advanceDials(to int64) {
	// Process every expiry and concurrency step chronologically, even after a slow frame.
	for {
		next := int64(math.MaxInt64)
		for _, d := range g.dials {
			next = min(next, d.Expires)
		}
		if g.nextDialStep > 0 && g.nextDialStep < g.cfg.DurationMS {
			next = min(next, g.nextDialStep)
		}
		if next > to {
			break
		}
		remaining := []trainerDial{}
		for _, d := range g.dials {
			if d.Expires <= next {
				g.metrics["expiries"]++
				g.score--
				g.history = append(g.history, map[string]any{"dial_id": d.ID, "status": "expired", "at_ms": d.Expires, "points": -1})
			} else {
				remaining = append(remaining, d)
			}
		}
		g.dials = remaining
		if next == g.nextDialStep {
			g.nextDialStep += 24000
		}
		g.fillDials(next)
	}
}
