package games

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"quantgame/backend/internal/engine"
)

type numberStep struct {
	Left     string `json:"left"`
	Right    string `json:"right"`
	Operator string `json:"operator"`
	ID       string `json:"id"`
}

func targetNumberPuzzle(rng *engine.RNG, index int) ([]trainerTile, int, []numberStep, int) {
	sources := []trainerTile{}
	for i := range 4 {
		v := rng.Range(1, 9)
		id := fmt.Sprintf("n%d", i)
		sources = append(sources, trainerTile{ID: id, Value: v, Expression: fmt.Sprint(v), Sources: []string{id}})
	}
	solutions := map[int]map[string]bool{}
	witnesses := map[int][]numberStep{}
	var solve func([]trainerTile, []numberStep)
	solve = func(tiles []trainerTile, path []numberStep) {
		if len(tiles) == 1 {
			v := tiles[0].Value
			if v < 5 || v > 200 {
				return
			}
			if solutions[v] == nil {
				solutions[v] = map[string]bool{}
			}
			solutions[v][tiles[0].Expression] = true
			if witnesses[v] == nil {
				witnesses[v] = append([]numberStep(nil), path...)
			}
			return
		}
		for i, left := range tiles {
			for j, right := range tiles {
				if i == j {
					continue
				}
				for _, op := range []string{"+", "-", "*", "/"} {
					if (op == "+" || op == "*") && i > j {
						continue
					}
					value, ok := numberCombine(left.Value, right.Value, op)
					if !ok || value < -10000 || value > 10000 {
						continue
					}
					id := fmt.Sprintf("r%d", len(path))
					expression := "(" + left.Expression + op + right.Expression + ")"
					remaining := []trainerTile{}
					for k, t := range tiles {
						if k != i && k != j {
							remaining = append(remaining, t)
						}
					}
					remaining = append(remaining, trainerTile{ID: id, Value: value, Expression: expression})
					next := append(append([]numberStep(nil), path...), numberStep{left.ID, right.ID, op, id})
					solve(remaining, next)
				}
			}
		}
	}
	solve(sources, nil)
	keys := []int{}
	for k := range solutions {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if len(solutions[keys[i]]) == len(solutions[keys[j]]) {
			return keys[i] < keys[j]
		}
		return len(solutions[keys[i]]) < len(solutions[keys[j]])
	})
	position := (len(keys) - 1) * (9 - min(index, 9)) / 9
	target := keys[position]
	return sources, target, witnesses[target], len(solutions[target])
}
func numberCombine(a, b int, op string) (int, bool) {
	switch op {
	case "+":
		if a > 1000000 || b > 1000000 {
			return 0, false
		}
		return a + b, true
	case "-":
		return a - b, true
	case "*":
		if a < -1000000 || a > 1000000 || b < -1000000 || b > 1000000 {
			return 0, false
		}
		v := int64(a) * int64(b)
		if v < -1000000000 || v > 1000000000 {
			return 0, false
		}
		return int(v), true
	case "/":
		if b == 0 || a%b != 0 {
			return 0, false
		}
		return a / b, true
	}
	return 0, false
}

type skylineMove struct {
	From int `json:"from"`
	To   int `json:"to"`
}

func skylineKey(stacks [][]string) string {
	parts := make([]string, len(stacks))
	for i, s := range stacks {
		parts[i] = strings.Join(s, "")
	}
	return strings.Join(parts, "|")
}
func skylineMoveState(stacks [][]string, capacity []int, from, to int) ([][]string, bool) {
	if from < 0 || from >= 3 || to < 0 || to >= 3 || from == to || len(stacks[from]) == 0 || len(stacks[to]) >= capacity[to] {
		return nil, false
	}
	next := trainerClone(stacks)
	last := len(next[from]) - 1
	v := next[from][last]
	next[from] = next[from][:last]
	next[to] = append(next[to], v)
	return next, true
}
func skylinePuzzle(rng *engine.RNG, goal [][]string, capacity []int, distance int) ([][]string, []skylineMove) {
	type node struct {
		stacks [][]string
		path   []skylineMove
	}
	queue := []node{{trainerClone(goal), nil}}
	seen := map[string]bool{skylineKey(goal): true}
	pool := []node{}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if len(current.path) == distance {
			pool = append(pool, current)
			continue
		}
		for from := range 3 {
			for to := range 3 {
				next, ok := skylineMoveState(current.stacks, capacity, from, to)
				if !ok || seen[skylineKey(next)] {
					continue
				}
				seen[skylineKey(next)] = true
				path := append(append([]skylineMove(nil), current.path...), skylineMove{from, to})
				queue = append(queue, node{next, path})
			}
		}
	}
	if len(pool) == 0 {
		return skylinePuzzle(rng, goal, capacity, distance-1)
	}
	selected := pool[rng.Int(len(pool))]
	solution := []skylineMove{}
	for i := len(selected.path) - 1; i >= 0; i-- {
		m := selected.path[i]
		solution = append(solution, skylineMove{m.To, m.From})
	}
	return selected.stacks, solution
}
func mysteryAll(attributes []map[string]any) [][]string {
	out := [][]string{{}}
	for _, a := range attributes {
		values := trainerDecode[[]string](a["values"])
		next := [][]string{}
		for _, prefix := range out {
			for _, v := range values {
				next = append(next, append(append([]string(nil), prefix...), v))
			}
		}
		out = next
	}
	return out
}
func mysteryCount(a, b []string) int {
	n := 0
	for i, v := range a {
		if i < len(b) && v == b[i] {
			n++
		}
	}
	return n
}
func mysteryConsistent(all, guesses [][]string, counts []int) [][]string {
	out := [][]string{}
	for _, v := range all {
		okay := true
		for i, q := range guesses {
			if mysteryCount(v, q) != counts[i] {
				okay = false
				break
			}
		}
		if okay {
			out = append(out, v)
		}
	}
	return out
}
func mysteryPartitionCost(candidates [][]string, guess []string) float64 {
	if len(candidates) == 0 {
		return 0
	}
	bins := make([]int, len(guess)+1)
	for _, v := range candidates {
		bins[mysteryCount(v, guess)]++
	}
	sum := 0
	for _, n := range bins {
		sum += n * n
	}
	return float64(sum) / float64(len(candidates))
}
func mysteryBestCost(candidates, all [][]string) float64 {
	best := float64(len(candidates))
	for _, q := range all {
		v := mysteryPartitionCost(candidates, q)
		if v < best {
			best = v
		}
	}
	return best
}
func sameStrings(a, b []string) bool { return slices.Equal(a, b) }
func sameJSON(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}
