#!/usr/bin/env bash
# Bio Radar: starts an isolated local demo API and the Linux application.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT"
PORT="${BIO_RADAR_PORT:-8080}"
if [[ ! "$PORT" =~ ^[0-9]+$ ]] || ((PORT<1024 || PORT>65535)); then
  echo 'BIO_RADAR_PORT must be between 1024 and 65535.' >&2; exit 2
fi
export ECO_DEMO_ADDR="127.0.0.1:$PORT"
BASE="http://127.0.0.1:$PORT/api/v1"
case "${1:-}" in
  --help|-h) echo 'Bio Radar: ./run.sh [--api-only]. Optional: BIO_RADAR_PORT=8081 ./run.sh'; exit 0 ;;
  --api-only) exec "$ROOT/scripts/demo.sh" ;;
  '') ;;
  *) echo 'Usage: ./run.sh [--api-only|--help]' >&2; exit 2 ;;
esac
command -v curl >/dev/null || { echo 'curl is required.' >&2; exit 1; }
command -v flutter >/dev/null || { echo 'Flutter SDK is required and must be on PATH.' >&2; exit 1; }
if curl -fsS --max-time 1 "$BASE/health" >/dev/null 2>&1; then
  echo "Port $PORT is already in use. Try BIO_RADAR_PORT=8081 ./run.sh" >&2; exit 1
fi
mkdir -p backend/data/presentation
LOG="$ROOT/backend/data/presentation/api.log"
"$ROOT/scripts/demo.sh" >"$LOG" 2>&1 &
API_PID=$!
cleanup() { kill "$API_PID" 2>/dev/null || true; wait "$API_PID" 2>/dev/null || true; }
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
READY=false
for ((i=0;i<1200;i++)); do
  if ! kill -0 "$API_PID" 2>/dev/null; then cat "$LOG" >&2; exit 1; fi
  if curl -fsS --max-time 1 "$BASE/health" >/dev/null 2>&1; then READY=true; break; fi
  sleep 0.1
done
if [ "$READY" != true ]; then tail -30 "$LOG" >&2; echo 'API did not become ready.' >&2; exit 1; fi
source "$ROOT/backend/data/presentation/login.env"
printf '\nBio Radar hazır · 30 demo kayıt\nGiriş: %s\nŞifre: %s\n\n' "$ECO_ADMIN_EMAIL" "$ECO_ADMIN_PASSWORD"
# Server configuration stays in the API process, never in the Flutter process.
unset ECO_ADMIN_EMAIL ECO_ADMIN_PASSWORD ECO_AI_API_KEY ECO_AI_BASE_URL ECO_AI_TEXT_MODEL ECO_AI_VISION_MODEL ECO_AI_EMBEDDING_MODEL ECO_AI_EMBEDDING_DIM
cd "$ROOT/apps/flutter_app"
flutter pub get
flutter build linux --dart-define="API_BASE_URL=$BASE"
./build/linux/x64/release/bundle/eco_terminal
