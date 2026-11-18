// Replay a completed assessment or trainer using frozen scenarios and action times.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"quantgame/backend/internal/games"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/replay assessment-or-trainer-tape.json")
		os.Exit(2)
	}
	raw, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var header struct {
		RuntimeVersion string `json:"runtime_version"`
	}
	_ = json.Unmarshal(raw, &header)
	var results json.RawMessage
	if header.RuntimeVersion == "trainer-runtime-v1" {
		results, err = games.ReplayTrainer(raw)
	} else {
		results, err = games.ReplayAssessment(raw)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(string(results))
}
