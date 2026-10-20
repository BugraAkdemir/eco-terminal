// Package rag implements the geo-aware hybrid retrieval-augmented answering used by /ai/chat:
// chunking and indexing (SQLite FTS5 + optional sqlite-vec), access-filtered hybrid retrieval, rule-based question
// understanding, SQL-computed facts, and answer assembly with validated citations. See docs/architecture/rag.md.
//
// Invariants:
//   - Access filters (visibility/owner/role, region, date) are part of the retrieval SQL, evaluated BEFORE ranking and
//     re-applied when hydrating results. A restricted chunk is never returned, whatever the query text says.
//   - Numbers in answers come from SQL facts; the LLM only explains. Retrieved text is untrusted data.
//   - Everything works without any AI key (FTS5 retrieval + retrieval_only answers).
package rag

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"

	"github.com/BugraAkdemir/eco-terminal/backend/internal/ai"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/config"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/storage"
)

// Caller identifies who is asking; it drives the access filter. The zero value is an anonymous visitor.
type Caller struct {
	UserID string
	Role   string // citizen | researcher | reviewer | admin | "" (anonymous)
}

// IsStaff reports reviewer/admin.
func (c Caller) IsStaff() bool { return c.Role == "reviewer" || c.Role == "admin" }

// Authenticated reports whether the caller is a signed-in user.
func (c Caller) Authenticated() bool { return c.UserID != "" }

// Engine is the RAG facade (indexer + retriever + answerer). Safe for concurrent use.
type Engine struct {
	DB  *sql.DB
	AI  *ai.Providers
	Cfg config.Config
	Now func() time.Time
	Log *slog.Logger

	vecMu sync.Mutex
}

// New builds an Engine. providers may be nil (no AI).
func New(db *sql.DB, cfg config.Config, providers *ai.Providers) *Engine {
	return &Engine{DB: db, AI: providers, Cfg: cfg, Now: time.Now, Log: slog.Default()}
}

func (e *Engine) now() time.Time { return e.Now().UTC() }

// ─── vector index management ───────────────────────────────────────

const vecTable = "rag_chunks_vec"

func (e *Engine) vecTableExists(ctx context.Context) bool {
	var n int
	_ = e.DB.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE name = ?`, vecTable).Scan(&n)
	return n > 0
}

// EnsureVectorIndex makes sure rag_chunks_vec exists with the configured model/dimension, rebuilding (drop +
// create; vectors re-created by EmbedBackfill) when embedding_index_metadata differs. It returns true when vector
// search is usable. Without an embedding provider it returns false and leaves any existing table untouched.
func (e *Engine) EnsureVectorIndex(ctx context.Context) (bool, error) {
	if !e.AI.HasEmbedding() {
		return false, nil
	}
	st := e.AI.Embedding.Status()
	dim := e.AI.Embedding.Dimensions()
	if dim <= 0 {
		return false, errors.New("rag: embedding dimension must be > 0")
	}
	e.vecMu.Lock()
	defer e.vecMu.Unlock()
	var model string
	var curDim int
	err := e.DB.QueryRowContext(ctx, `SELECT model, dimensions FROM embedding_index_metadata WHERE id = 1`).Scan(&model, &curDim)
	exists := e.vecTableExists(ctx)
	if err == nil && exists && model == st.Model && curDim == dim {
		return true, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	tx, err := e.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DROP TABLE IF EXISTS `+vecTable); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`CREATE VIRTUAL TABLE %s USING vec0(embedding float[%d] distance_metric=cosine)`, vecTable, dim)); err != nil {
		return false, fmt.Errorf("rag: creating vector table: %w", err)
	}
	now := storage.Time(e.now())
	if _, err := tx.ExecContext(ctx, `INSERT INTO embedding_index_metadata(id,provider,model,dimensions,vec_table,created_at,updated_at)
		VALUES (1,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET provider=excluded.provider, model=excluded.model,
		dimensions=excluded.dimensions, vec_table=excluded.vec_table, updated_at=excluded.updated_at`,
		st.Provider, st.Model, dim, vecTable, now, now); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	e.Log.Info("rag: vector index (re)built; run EmbedBackfill to embed existing chunks", "model", st.Model, "dim", dim)
	return true, nil
}

func vecBlob(v []float32) ([]byte, error) { return sqlite_vec.SerializeFloat32(v) }

// EmbedBackfill embeds up to batch chunks that have no vector yet. Returns the number embedded; call repeatedly
// (the embed_backfill job does) until it returns 0.
func (e *Engine) EmbedBackfill(ctx context.Context, batch int) (int, error) {
	ok, err := e.EnsureVectorIndex(ctx)
	if err != nil || !ok {
		return 0, err
	}
	if batch <= 0 {
		batch = 64
	}
	rows, err := e.DB.QueryContext(ctx, `SELECT c.id, c.text FROM rag_document_chunks c
		WHERE c.id NOT IN (SELECT rowid FROM `+vecTable+`) ORDER BY c.id LIMIT ?`, batch)
	if err != nil {
		return 0, err
	}
	var ids []int64
	var texts []string
	for rows.Next() {
		var id int64
		var t string
		if err := rows.Scan(&id, &t); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
		texts = append(texts, t)
	}
	rows.Close()
	if len(ids) == 0 {
		return 0, rows.Err()
	}
	res, err := e.AI.Embedding.Embed(ctx, texts)
	if err != nil {
		return 0, err
	}
	tx, err := e.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	n := 0
	for i, id := range ids {
		blob, err := vecBlob(res.Vectors[i])
		if err != nil {
			return 0, err
		}
		// the chunk may have been replaced meanwhile; only embed chunks that still exist
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM rag_document_chunks WHERE id = ?`, id).Scan(&exists); err != nil {
			return 0, err
		}
		if exists == 0 {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO `+vecTable+`(rowid, embedding) VALUES (?, ?)`, id, blob); err != nil {
			return 0, err
		}
		n++
	}
	return n, tx.Commit()
}

// VectorSearchEnabled reports whether hybrid (vector) retrieval is available right now.
func (e *Engine) VectorSearchEnabled(ctx context.Context) bool {
	if !e.AI.HasEmbedding() {
		return false
	}
	ok, err := e.EnsureVectorIndex(ctx)
	return err == nil && ok
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}
