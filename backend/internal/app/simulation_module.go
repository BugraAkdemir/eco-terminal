package app

import (
	"context"
	"net/http"

	"github.com/BugraAkdemir/eco-terminal/backend/internal/simulation"
)

// simulationModule adapts simulation.Module (which cannot import this package) to Module + Worker.
// Plug the narrative provider in with simulation.WithExplainer(...) when the AI text provider exists.
type simulationModule struct{ m *simulation.Module }

func newSimulationModule(d Deps, opts ...simulation.Option) *simulationModule {
	return &simulationModule{m: simulation.NewModule(simulation.Deps{DB: d.DB, Config: d.Config, Logger: d.Logger}, opts...)}
}

func (s *simulationModule) Register(mux *http.ServeMux, _ Deps) { s.m.Register(mux) }
func (s *simulationModule) Start(ctx context.Context) error     { return s.m.Start(ctx) }
func (s *simulationModule) Stop()                               { s.m.Stop() }
