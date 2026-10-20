// Package media handles observation image uploads (magic-byte validation, EXIF/GPS stripping by
// re-encoding, thumbnails, random file names) and serves images with the same visibility rules as
// the parent observation.
package media

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BugraAkdemir/eco-terminal/backend/internal/audit"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/auth"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/httpx"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/observations"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/storage"
)

// MaxPerObservation is the maximum number of images per observation.
const MaxPerObservation = 8

// Service manages media files and rows.
type Service struct {
	db           *sql.DB
	obs          *observations.Service
	root         string
	maxBytes     int64
	aiConfigured bool
	log          *slog.Logger
}

// NewService creates the service. storageDir is ECO_STORAGE_DIR; maxUploadMB is ECO_MAX_UPLOAD_MB.
func NewService(db *sql.DB, obs *observations.Service, storageDir string, maxUploadMB int, aiConfigured bool, log *slog.Logger) (*Service, error) {
	if log == nil {
		log = slog.Default()
	}
	if maxUploadMB < 1 {
		maxUploadMB = 10
	}
	root, err := filepath.Abs(storageDir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("create storage dir: %w", err)
	}
	return &Service{db: db, obs: obs, root: root, maxBytes: int64(maxUploadMB) << 20, aiConfigured: aiConfigured, log: log}, nil
}

// Routes registers the media endpoints (paths include /api/v1).
func (s *Service) Routes(mux *http.ServeMux) {
	mux.Handle("POST /api/v1/observations/{id}/media", auth.RequireFunc(s.handleUpload))
	mux.HandleFunc("GET /api/v1/media/{id}", s.handleServe)
}

// BodyLimit is the largest request body the upload endpoint accepts (file + multipart overhead).
func (s *Service) BodyLimit() int64 { return s.maxBytes + (1 << 20) }

func randomName() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

func thumbPath(rel string) string {
	ext := filepath.Ext(rel)
	return strings.TrimSuffix(rel, ext) + "_t" + ext
}

// safePath resolves a stored relative path under root and refuses anything that escapes it.
func (s *Service) safePath(rel string) (string, error) {
	if rel == "" || filepath.IsAbs(rel) || strings.Contains(rel, "..") || strings.ContainsRune(rel, 0) {
		return "", errors.New("unsafe media path")
	}
	full := filepath.Join(s.root, filepath.Clean(rel))
	if !strings.HasPrefix(full, s.root+string(filepath.Separator)) {
		return "", errors.New("unsafe media path")
	}
	return full, nil
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(path)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(path)
		return err
	}
	return nil
}

type mediaRow struct {
	id, mime string
	w, h     int
	size     int64
}

func (s *Service) existingBySHA(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, obsID, sha string) (*observations.Media, error) {
	var m mediaRow
	err := q.QueryRowContext(ctx, `SELECT id, mime_type, COALESCE(width,0), COALESCE(height,0), size_bytes
		FROM observation_media WHERE observation_id = ? AND sha256 = ?`, obsID, sha).Scan(&m.id, &m.mime, &m.w, &m.h, &m.size)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := observations.NewMedia(m.id, m.mime, m.w, m.h, m.size)
	return &out, nil
}

// Upload validates and stores one image for an observation. created=false means an identical image
// (same sha256 of the re-encoded bytes) was already attached; the existing Media is returned.
func (s *Service) Upload(ctx context.Context, user *auth.User, actor audit.Actor, obsID string, data []byte) (m *observations.Media, created bool, err error) {
	acc, err := s.obs.AccessOf(ctx, s.db, obsID)
	if err != nil {
		return nil, false, err
	}
	if !observations.CanView(user, acc.OwnerID, acc.Status) {
		return nil, false, httpx.NotFound("observation not found")
	}
	if acc.OwnerID != user.ID {
		return nil, false, httpx.Forbidden("only the owner can add photos")
	}
	if !observations.OwnerEditable(acc.Status) {
		return nil, false, httpx.Conflict("photos cannot be added in status " + acc.Status)
	}
	mime := SniffMIME(data)
	if mime == "" {
		return nil, false, httpx.UnsupportedMedia("only JPEG, PNG and WebP images are accepted")
	}
	p, err := Process(data, mime)
	switch {
	case errors.Is(err, ErrDimensions):
		return nil, false, httpx.TooLarge("image dimensions are too large")
	case err != nil:
		return nil, false, httpx.UnsupportedMedia("the file is not a valid image")
	}
	sum := sha256.Sum256(p.Data)
	sha := hex.EncodeToString(sum[:])

	if ex, err := s.existingBySHA(ctx, s.db, obsID, sha); err != nil || ex != nil {
		return ex, false, err
	}
	now := time.Now().UTC()
	rel := filepath.Join(fmt.Sprintf("%04d", now.Year()), fmt.Sprintf("%02d", int(now.Month())), randomName()+p.Ext)
	full, err := s.safePath(rel)
	if err != nil {
		return nil, false, err
	}
	thumbFull, _ := s.safePath(thumbPath(rel))
	if err := writeFile(full, p.Data); err != nil {
		return nil, false, err
	}
	if err := writeFile(thumbFull, p.Thumb); err != nil {
		os.Remove(full)
		return nil, false, err
	}
	cleanup := func() { os.Remove(full); os.Remove(thumbFull) }

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		cleanup()
		return nil, false, err
	}
	defer tx.Rollback()
	// re-check inside the write transaction (serialized): duplicates and the per-observation limit
	if ex, err := s.existingBySHA(ctx, tx, obsID, sha); err != nil || ex != nil {
		cleanup()
		return ex, false, err
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM observation_media WHERE observation_id = ?`, obsID).Scan(&count); err != nil {
		cleanup()
		return nil, false, err
	}
	if count >= MaxPerObservation {
		cleanup()
		return nil, false, httpx.Conflict(fmt.Sprintf("at most %d photos per observation", MaxPerObservation))
	}
	id, ts := storage.NewID(), storage.Time(now)
	if _, err := tx.ExecContext(ctx, `INSERT INTO observation_media(id, observation_id, storage_path, mime_type, size_bytes, width, height, sha256,
		exif_stripped, position, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,1,?,?,?)`,
		id, obsID, filepath.ToSlash(rel), p.Mime, len(p.Data), p.Width, p.Height, sha, count, ts, ts); err != nil {
		cleanup()
		return nil, false, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE observations SET updated_at = ? WHERE id = ?`, ts, obsID); err != nil {
		cleanup()
		return nil, false, err
	}
	if err := audit.Log(ctx, tx, audit.Entry{Actor: actor, Action: "media.upload", EntityType: "observation", EntityID: obsID,
		After: map[string]any{"media_id": id, "mime_type": p.Mime, "size_bytes": len(p.Data), "sha256": sha}}); err != nil {
		cleanup()
		return nil, false, err
	}
	if acc.Status != "draft" {
		if err := observations.EnqueueIndex(ctx, tx, obsID); err != nil {
			cleanup()
			return nil, false, err
		}
		if s.aiConfigured {
			var queued int
			if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM jobs WHERE kind = ? AND status = 'queued' AND payload_json = ?`,
				observations.JobAnalyzeObservation, `{"observation_id":"`+obsID+`"}`).Scan(&queued); err != nil {
				cleanup()
				return nil, false, err
			}
			if queued == 0 {
				if err := observations.EnqueueAnalyze(ctx, tx, obsID, now); err != nil {
					cleanup()
					return nil, false, err
				}
			}
		}
	}
	if err := tx.Commit(); err != nil {
		cleanup()
		return nil, false, err
	}
	out := observations.NewMedia(id, p.Mime, p.Width, p.Height, int64(len(p.Data)))
	return &out, true, nil
}

func (s *Service) handleUpload(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	r.Body = http.MaxBytesReader(w, r.Body, s.BodyLimit())
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			httpx.WriteError(w, httpx.TooLarge(fmt.Sprintf("upload exceeds %d MB", s.maxBytes>>20)))
			return
		}
		httpx.WriteError(w, httpx.Validation("expected a multipart/form-data body with a 'file' field", nil))
		return
	}
	defer r.MultipartForm.RemoveAll()
	f, _, err := r.FormFile("file")
	if err != nil {
		httpx.WriteError(w, httpx.Validation("multipart field 'file' is required", map[string]any{"field": "file"}))
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, s.maxBytes+1))
	if err != nil {
		httpx.WriteError(w, httpx.Validation("could not read the uploaded file", nil))
		return
	}
	if int64(len(data)) > s.maxBytes {
		httpx.WriteError(w, httpx.TooLarge(fmt.Sprintf("upload exceeds %d MB", s.maxBytes>>20)))
		return
	}
	m, created, err := s.Upload(r.Context(), u, auth.ActorFrom(r), r.PathValue("id"), data)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	status := http.StatusCreated
	if !created {
		status = http.StatusOK
	}
	httpx.WriteJSON(w, status, m)
}

func (s *Service) handleServe(w http.ResponseWriter, r *http.Request) {
	viewer, _ := auth.UserFrom(r.Context())
	var rel, mime, owner, status string
	err := s.db.QueryRowContext(r.Context(), `SELECT m.storage_path, m.mime_type, o.user_id, o.status
		FROM observation_media m JOIN observations o ON o.id = m.observation_id
		WHERE m.id = ? AND o.deleted_at IS NULL`, r.PathValue("id")).Scan(&rel, &mime, &owner, &status)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !observations.CanView(viewer, owner, status)) {
		httpx.WriteError(w, httpx.NotFound("media not found"))
		return
	}
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	if httpx.QueryBool(r, "thumb") {
		rel = thumbPath(rel)
	}
	full, err := s.safePath(rel)
	if err != nil {
		s.log.Error("refusing unsafe media path", "path", rel)
		httpx.WriteError(w, httpx.NotFound("media not found"))
		return
	}
	f, err := os.Open(full)
	if err != nil {
		httpx.WriteError(w, httpx.NotFound("media not found"))
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		httpx.WriteError(w, httpx.NotFound("media not found"))
		return
	}
	h := w.Header()
	h.Set("Content-Type", mime)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "default-src 'none'")
	h.Set("Content-Disposition", "inline")
	if status == "verified" || status == "resolved" {
		h.Set("Cache-Control", "public, max-age=86400")
	} else {
		h.Set("Cache-Control", "private, max-age=300")
	}
	h.Add("Vary", "Authorization")
	http.ServeContent(w, r, "", st.ModTime(), f)
}
