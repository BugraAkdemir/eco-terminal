package observations

import (
	"net/http"

	"github.com/BugraAkdemir/eco-terminal/backend/internal/auth"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/httpx"
)

// Routes registers the observation endpoints and /regions/{id}/observations (paths include /api/v1).
func (s *Service) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/observations", s.handleList)
	mux.Handle("POST /api/v1/observations", auth.RequireFunc(s.handleCreate))
	mux.HandleFunc("GET /api/v1/observations/{id}", s.handleGet)
	mux.Handle("PATCH /api/v1/observations/{id}", auth.RequireFunc(s.handlePatch))
	mux.Handle("DELETE /api/v1/observations/{id}", auth.RequireFunc(s.handleDelete))
	mux.HandleFunc("GET /api/v1/regions/{id}/observations", s.handleRegionList)
}

func (s *Service) handleList(w http.ResponseWriter, r *http.Request) {
	viewer, _ := auth.UserFrom(r.Context())
	f, err := ParseFilter(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	s.writeList(w, r, viewer, f)
}

func (s *Service) writeList(w http.ResponseWriter, r *http.Request, viewer *auth.User, f Filter) {
	page, err := httpx.ParsePage(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	items, total, err := s.List(r.Context(), viewer, f, page)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSONMeta(w, http.StatusOK, items, page.Meta(total))
}

func (s *Service) handleRegionList(w http.ResponseWriter, r *http.Request) {
	viewer, _ := auth.UserFrom(r.Context())
	f, err := ParseFilter(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	id := r.PathValue("id")
	ok, err := s.geo.Exists(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	if !ok {
		httpx.WriteError(w, httpx.NotFound("region not found"))
		return
	}
	f.RegionID = id
	s.writeList(w, r, viewer, f)
}

func (s *Service) handleCreate(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	var in CreateInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, err)
		return
	}
	obs, created, err := s.Create(r.Context(), u, auth.ActorFrom(r), in, CreateOptions{})
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	status := http.StatusCreated
	if !created {
		status = http.StatusOK
	}
	httpx.WriteJSON(w, status, obs)
}

func (s *Service) handleGet(w http.ResponseWriter, r *http.Request) {
	viewer, _ := auth.UserFrom(r.Context())
	d, err := s.GetDetail(r.Context(), viewer, r.PathValue("id"))
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, d)
}

func (s *Service) handlePatch(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	var in PatchInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, err)
		return
	}
	obs, err := s.Patch(r.Context(), u, auth.ActorFrom(r), r.PathValue("id"), in)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, obs)
}

func (s *Service) handleDelete(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	if err := s.Delete(r.Context(), u, auth.ActorFrom(r), r.PathValue("id")); err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.NoContent(w)
}
