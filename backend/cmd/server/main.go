// Command server is the Eco Terminal API server. All wiring lives in internal/app.
package main

import (
	"log"

	"github.com/BugraAkdemir/eco-terminal/backend/internal/app"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/config"
)

func main() {
	if err := app.Run(config.Load()); err != nil {
		log.Fatalf("server: %v", err)
	}
}
