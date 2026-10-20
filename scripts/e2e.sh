#!/usr/bin/env bash
# End-to-end smoke test against a RUNNING server (starts nothing).
#
#   BASE_URL=http://localhost:8080/api/v1 ADMIN_EMAIL=admin@example.org ADMIN_PASSWORD=... ./scripts/e2e.sh
#
# ADMIN_EMAIL / ADMIN_PASSWORD default to ECO_ADMIN_EMAIL / ECO_ADMIN_PASSWORD (the bootstrap admin acts as reviewer).
# Flow: register citizen -> create observation -> upload PNG -> admin login -> review queue -> verify ->
#       citizen sees verified + notification -> anonymous map list shows it -> region analytics counts it.
# Needs: bash, curl and (jq or python3). Exits non-zero on the first failed check.
set -u
BASE_URL="${BASE_URL:-http://localhost:8080/api/v1}"
ADMIN_EMAIL="${ADMIN_EMAIL:-${ECO_ADMIN_EMAIL:-}}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-${ECO_ADMIN_PASSWORD:-}}"
if [ -z "$ADMIN_EMAIL" ] || [ -z "$ADMIN_PASSWORD" ]; then
  echo "FAIL: set ADMIN_EMAIL and ADMIN_PASSWORD (or ECO_ADMIN_EMAIL/ECO_ADMIN_PASSWORD)" >&2
  exit 2
fi
command -v curl >/dev/null || { echo "FAIL: curl is required" >&2; exit 2; }
if [ -z "${E2E_NO_JQ:-}" ] && command -v jq >/dev/null; then JSON=jq; elif command -v python3 >/dev/null; then JSON=python3; else
  echo "FAIL: jq or python3 is required" >&2; exit 2; fi

TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT
STEP=0
fail() { echo "FAIL [step $STEP]: $*" >&2; [ -f "$TMP/last.json" ] && { echo "last response:" >&2; head -c 1500 "$TMP/last.json" >&2; echo >&2; }; exit 1; }
step() { STEP=$((STEP+1)); echo "[$STEP] $*"; }

# jget '<json>' 'data.user.id'  (dotted path; numeric segments index arrays) -> prints scalar or empty
jget() {
  if [ "$JSON" = jq ]; then
    printf '%s' "$1" | jq -r --arg p "$2" 'getpath($p | split(".") | map(tonumber? // .)) // empty | if type=="boolean" or type=="number" or type=="string" then tostring else tojson end' 2>/dev/null
  else
    printf '%s' "$1" | python3 -c '
import json,sys
try: v=json.load(sys.stdin)
except Exception: sys.exit(0)
for k in sys.argv[1].split("."):
    try: v=v[int(k)] if isinstance(v,list) else v[k]
    except Exception: v=None; break
if v is None: pass
elif isinstance(v,bool): print("true" if v else "false")
elif isinstance(v,(dict,list)): print(json.dumps(v))
else: print(v)' "$2"
  fi
}
# jlen '<json>' 'data'  -> array length
jlen() {
  if [ "$JSON" = jq ]; then printf '%s' "$1" | jq -r --arg p "$2" 'getpath($p | split(".") | map(tonumber? // .)) | length' 2>/dev/null
  else printf '%s' "$1" | python3 -c '
import json,sys
v=json.load(sys.stdin)
for k in sys.argv[1].split("."):
    v=v[int(k)] if isinstance(v,list) else v[k]
print(len(v))' "$2"; fi
}

# api METHOD PATH [TOKEN] [JSON_BODY]  -> sets $CODE and $BODY
api() {
  local method="$1" path="$2" token="${3:-}" body="${4:-}"
  local args=(-sS -m 30 -o "$TMP/last.json" -w '%{http_code}' -X "$method")
  [ -n "$token" ] && args+=(-H "Authorization: Bearer $token")
  [ -n "$body" ] && args+=(-H 'Content-Type: application/json' --data "$body")
  CODE="$(curl "${args[@]}" "$BASE_URL$path")" || fail "curl failed for $method $path"
  BODY="$(cat "$TMP/last.json")"
}
expect() { [ "$CODE" = "$1" ] || fail "$2: expected HTTP $1, got $CODE"; }

# 1x1 PNG
printf '%s' 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==' | base64 -d > "$TMP/tiny.png" || fail "cannot decode test PNG"

step "health"
api GET /health; expect 200 "health"
[ "$(jget "$BODY" data.status)" = ok ] || fail "health status"

SUFFIX="$(date +%s)$RANDOM"
EMAIL="e2e-citizen-$SUFFIX@example.org"
PASS="e2e-password-$SUFFIX"

step "register citizen $EMAIL"
api POST /auth/register "" "{\"email\":\"$EMAIL\",\"password\":\"$PASS\",\"display_name\":\"E2E Citizen\"}"; expect 201 "register"
CTOKEN="$(jget "$BODY" data.token)"; CID="$(jget "$BODY" data.user.id)"
[ -n "$CTOKEN" ] && [ -n "$CID" ] || fail "no token/user id in register response"

step "create observation (Kastamonu Merkez)"
OBSERVED="$(date -u -d '-1 hour' +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -v-1H +%Y-%m-%dT%H:%M:%SZ)"
api POST /observations "$CTOKEN" "{\"record_type\":\"observation\",\"category\":\"animal\",\"title\":\"E2E sighting\",\"description\":\"created by e2e.sh\",\"species_guess\":\"Cervus elaphus\",\"count_observed\":2,\"lat\":41.3887,\"lon\":33.7827,\"observed_at\":\"$OBSERVED\",\"client_ref\":\"e2e-$SUFFIX-ref\"}"; expect 201 "create observation"
OID="$(jget "$BODY" data.id)"; REF="$(jget "$BODY" data.reference_no)"
[ -n "$OID" ] || fail "no observation id"
case "$REF" in ET-*) ;; *) fail "unexpected reference_no '$REF'";; esac
[ "$(jget "$BODY" data.status)" = submitted ] || fail "status should be submitted"

step "idempotent retry with the same client_ref returns the existing record (200)"
api POST /observations "$CTOKEN" "{\"record_type\":\"observation\",\"category\":\"animal\",\"lat\":41.3887,\"lon\":33.7827,\"observed_at\":\"$OBSERVED\",\"client_ref\":\"e2e-$SUFFIX-ref\"}"; expect 200 "idempotent create"
[ "$(jget "$BODY" data.id)" = "$OID" ] || fail "retry created a different record"

step "upload PNG"
CODE="$(curl -sS -m 30 -o "$TMP/last.json" -w '%{http_code}' -H "Authorization: Bearer $CTOKEN" -F "file=@$TMP/tiny.png;type=image/png" "$BASE_URL/observations/$OID/media")" || fail "curl upload"
BODY="$(cat "$TMP/last.json")"; expect 201 "upload"
MID="$(jget "$BODY" data.id)"; [ -n "$MID" ] || fail "no media id"

step "upload a text file pretending to be a PNG is rejected (415)"
printf 'not an image' > "$TMP/fake.png"
CODE="$(curl -sS -m 30 -o "$TMP/last.json" -w '%{http_code}' -H "Authorization: Bearer $CTOKEN" -F "file=@$TMP/fake.png;type=image/png" "$BASE_URL/observations/$OID/media")"; expect 415 "fake png"

step "anonymous cannot see the unverified record"
api GET "/observations/$OID"; expect 404 "anonymous detail of unverified"
CODE="$(curl -sS -m 30 -o /dev/null -w '%{http_code}' "$BASE_URL/media/$MID")"; [ "$CODE" = 404 ] || fail "anonymous media of unverified record should be 404, got $CODE"

step "admin (reviewer) login"
api POST /auth/login "" "{\"email\":\"$ADMIN_EMAIL\",\"password\":\"$ADMIN_PASSWORD\"}"; expect 200 "admin login"
ATOKEN="$(jget "$BODY" data.token)"; [ -n "$ATOKEN" ] || fail "no admin token"

step "review queue lists the record with media"
api GET "/reviews/queue?category=animal&page_size=200" "$ATOKEN"; expect 200 "queue"
FOUND=""; N="$(jlen "$BODY" data)"; i=0
while [ "$i" -lt "$N" ]; do [ "$(jget "$BODY" data.$i.id)" = "$OID" ] && FOUND=1 && [ "$(jlen "$BODY" data.$i.media)" = 1 ] && break; i=$((i+1)); done
[ -n "$FOUND" ] || fail "record (with 1 media item) not found in the review queue"

step "citizen cannot use the review endpoints (403)"
api GET /reviews/queue "$CTOKEN"; expect 403 "citizen queue"

step "verify"
api POST "/reviews/$OID/decision" "$ATOKEN" '{"decision":"verify","corrected_species":"Cervus elaphus","note":"e2e verification"}'; expect 200 "verify"
[ "$(jget "$BODY" data.status)" = verified ] || fail "status should be verified"
api POST "/reviews/$OID/decision" "$ATOKEN" '{"decision":"verify"}'; expect 409 "second verify must conflict"

step "citizen sees verified status + notification"
api GET "/observations/$OID" "$CTOKEN"; expect 200 "citizen detail"
[ "$(jget "$BODY" data.status)" = verified ] || fail "citizen does not see verified"
[ "$(jget "$BODY" data.species_verified)" = "Cervus elaphus" ] || fail "species_verified missing"
api GET "/notifications?unread=true" "$CTOKEN"; expect 200 "notifications"
[ "$(jlen "$BODY" data)" -ge 1 ] || fail "citizen has no notification"
[ "$(jget "$BODY" data.0.observation_id)" = "$OID" ] || fail "notification does not reference the observation"
NID="$(jget "$BODY" data.0.id)"
api POST "/notifications/$NID/read" "$CTOKEN"; expect 204 "mark read"

step "anonymous map list (bbox over Kastamonu) shows it"
api GET "/observations?bbox=32.5,40.8,35.5,42.2&page_size=200"; expect 200 "anonymous list"
FOUND=""; N="$(jlen "$BODY" data)"; i=0
while [ "$i" -lt "$N" ]; do [ "$(jget "$BODY" data.$i.id)" = "$OID" ] && FOUND=1 && break; i=$((i+1)); done
[ -n "$FOUND" ] || fail "verified record missing from anonymous bbox list"
CODE="$(curl -sS -m 30 -o /dev/null -w '%{http_code}' "$BASE_URL/media/$MID?thumb=1")"; [ "$CODE" = 200 ] || fail "anonymous thumbnail of verified record should be 200, got $CODE"

step "region lookup + analytics count the verified record"
api GET "/regions?lon=33.7827&lat=41.3887"; expect 200 "region lookup"
RID="$(jget "$BODY" data.0.id)"; [ -n "$RID" ] || fail "no region contains the point (are regions seeded?)"
api GET "/regions/$RID/analytics"; expect 200 "analytics"
OBS="$(jget "$BODY" data.totals.observations)"; VER="$(jget "$BODY" data.totals.verified)"
[ "${OBS:-0}" -ge 1 ] && [ "${VER:-0}" -ge 1 ] || fail "analytics totals do not count the record (observations=$OBS verified=$VER)"
case "$BODY" in *"popülasyon"*) ;; *) fail "analytics lacks the sampling-bias warning";; esac

step "audit log has the verification (admin)"
api GET "/admin/audit-logs?entity_id=$OID" "$ATOKEN"; expect 200 "audit"
case "$BODY" in *observation.status_change*) ;; *) fail "audit log lacks observation.status_change";; esac

echo "PASS: e2e flow ok ($REF, observation $OID)"

step "authenticated assistant uses SQL facts for the selected region"
api POST /ai/chat "$ATOKEN" "{\"message\":\"Bu bölgede kaç gözlem var?\",\"region_id\":\"$RID\"}"; expect 200 "chat"
[ "$(jlen "$BODY" data.facts)" -ge 1 ] || fail "chat returned no SQL facts"
[ "$(jlen "$BODY" data.sources)" -ge 1 ] || fail "chat returned no citations"
api POST /ai/chat "" '{"message":"kaç gözlem var?"}'; expect 401 "anonymous chat"

step "public feeds and provider capabilities"
api GET /feeds; expect 200 "feeds"
api GET /feeds/articles; expect 200 "feed articles"
api GET /ai/providers/status "$ATOKEN"; expect 200 "provider status"

step "real-climate scenario and comparison (admin/researcher)"
api GET '/regions?kind=province'; expect 200 "province"
PROVINCE="$(jget "$BODY" data.0.id)"; [ -n "$PROVINCE" ] || fail "province missing"
RUN="{\"name\":\"E2E rainfall -20 temperature +2\",\"region_id\":\"$PROVINCE\",\"reference_from\":\"1991-01\",\"reference_to\":\"2020-12\",\"precip_change_pct\":-20,\"temp_change_c\":2}"
api POST /simulations/run "$ATOKEN" "$RUN"; expect 200 "scenario"
[ "$(jget "$BODY" data.status)" = ok ] || fail "scenario needs a usable climate snapshot (see ECO_CLIMATE_SNAPSHOT)"
SIM_A="$(jget "$BODY" data.id)"
[ "$(jlen "$BODY" data.indicators)" -gt 0 ] || fail "scenario indicators missing"
api POST /simulations/run "$ATOKEN" "{\"name\":\"E2E reference\",\"region_id\":\"$PROVINCE\",\"reference_from\":\"1991-01\",\"reference_to\":\"2020-12\"}"; expect 200 "reference scenario"
SIM_B="$(jget "$BODY" data.id)"
api GET "/simulations/compare?a=$SIM_A&b=$SIM_B" "$ATOKEN"; expect 200 "scenario comparison"
[ "$(jget "$BODY" data.same_baseline)" = true ] || fail "scenario baselines differ"
[ "$(jlen "$BODY" data.deltas)" -gt 0 ] || fail "scenario comparison is empty"
echo "PASS: extended flow observation -> review -> map -> cited chat; real-climate scenarios -> comparison"
