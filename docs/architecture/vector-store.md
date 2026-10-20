# Decision: SQLite driver + vector store

**Chosen:** `github.com/mattn/go-sqlite3` (CGO) + `github.com/asg017/sqlite-vec-go-bindings/cgo` (sqlite-vec v0.1.6), build tags `sqlite_fts5 sqlite_rtree`.

Spike result (2026-10-20, Go 1.27.1, gcc 16, SQLite 3.53.4): `vec0` KNN query, FTS5 `MATCH` and RTree range query all work in one WAL database.

**Rejected / why**
- `ncruces/go-sqlite3` + sqlite-vec WASM binding: the binding (v0.1.6) targets ncruces v0.17.1; with v0.17.1 on Go 1.27 the embedded wasm fails (`go_busy_timeout` import signature mismatch); newer ncruces removed `sqlite3.Binary` so the binding does not compile. Revisit if the binding is updated.
- `chromem-go`: kept as the documented fallback; not needed since sqlite-vec works.

**Consequences:** a C compiler is required to build the backend (Linux/macOS: gcc/clang; Windows: MinGW-w64). Flutter clients do not embed the backend, so this only affects server builds. `vec0` tables have a fixed dimension, so `rag_chunks_vec` is created at runtime from `embedding_index_metadata` (not in the SQL migration). If the embedding model/dimension changes the vec table is dropped and rebuilt.
