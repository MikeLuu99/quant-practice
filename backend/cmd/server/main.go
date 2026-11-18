// quant-game backend server.
package main

import (
	"log"
	"os"
	"strconv"

	"quantgame/backend/internal/api"
)

func main() {
	port := 8080
	if p := os.Getenv("PORT"); p != "" {
		if v, err := strconv.Atoi(p); err == nil {
			port = v
		}
	}
	staticDir := os.Getenv("STATIC_DIR")
	if staticDir == "" {
		staticDir = "../frontend/dist"
	}
	srv := api.Server(port, staticDir)
	log.Printf("quant-game backend listening on %s", srv.Addr)
	log.Fatal(srv.ListenAndServe())
}
