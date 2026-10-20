package app

import "github.com/BugraAkdemir/eco-terminal/backend/internal/simulation"

// extensions is THE place where other agents' modules plug in. Add one line per module:
//
//	mods = append(mods, ai.NewModule(d))          // registers /ai/* routes (implements Module)
//	mods = append(mods, rss.NewModule(d))         // /feeds, /admin/feeds; if it also has Start/Stop it runs as a worker
//	workers = append(workers, jobs.NewWorker(d))  // pure background worker (Start(ctx) error / Stop())
//
// A Module that also implements Worker is started/stopped automatically; do not add it twice.
// d.DB, d.Config, d.Logger, d.Auth, d.Require(...) and d.Observations are available for wiring.
func extensions(d Deps) (mods []Module, workers []Worker) {
	intelligence := newIntelligenceModule(d)
	mods = append(mods, intelligence)
	mods = append(mods, newSimulationModule(d, simulation.WithExplainer(simulationTextExplainer{provider: intelligence.providers.Text})))
	return mods, workers
}
