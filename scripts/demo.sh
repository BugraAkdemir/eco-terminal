#!/usr/bin/env bash
# Local presentation API. Uses its own database and randomly generated login.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEMO_DIR="$ROOT/backend/data/presentation"
mkdir -p "$DEMO_DIR"
chmod 700 "$DEMO_DIR"
LOGIN="$DEMO_DIR/login.env"
if [ ! -f "$LOGIN" ]; then
  (umask 077; python3 - <<'PY' > "$LOGIN"
import secrets
print('ECO_ADMIN_EMAIL=presentation@ecoterminal.invalid')
print('ECO_ADMIN_PASSWORD='+secrets.token_urlsafe(18))
PY
  )
fi
set -a
source "$LOGIN"
set +a
export ECO_DB_PATH="$DEMO_DIR/eco.db" ECO_STORAGE_DIR="$DEMO_DIR/media"
export ECO_ADDR="${ECO_DEMO_ADDR:-127.0.0.1:8080}" ECO_SEED_DEMO=true ECO_RSS_ENABLED=false
printf 'Demo API: http://%s/api/v1\nLogin: %s\nPassword: %s\n' "$ECO_ADDR" "$ECO_ADMIN_EMAIL" "$ECO_ADMIN_PASSWORD"
printf '30 flagged demo records; RSS periodic fetch disabled. Credentials: %s\n' "$LOGIN"
cd "$ROOT/backend"
go build -tags 'sqlite_fts5 sqlite_rtree' -o bin/server ./cmd/server
exec ./bin/server
