// Command server is the Eco Terminal API server. Wiring lives in internal/app (added by the backend-core work).
package main

import (
	"log"
	"net/http"

	"github.com/BugraAkdemir/eco-terminal/backend/internal/config"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/storage"
)

func main() {
	cfg := config.Load()
	db, err := storage.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"status":"ok"}}`))
	})
	log.Printf("listening on %s", cfg.Addr)
	log.Fatal(http.ListenAndServe(cfg.Addr, mux))
}
