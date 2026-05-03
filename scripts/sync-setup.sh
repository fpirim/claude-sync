#!/usr/bin/env bash
# Wire two Syncthing instances (local + remote-via-SSH) into a paired set
# sharing one folder. Reads everything from env (.env), is idempotent: if the
# password is already set, the peer already added, the folder already shared,
# the script reports it and moves on without errors.
#
# Required env:
#   GUI_USER, GUI_PASSWORD, FOLDER_ID, FOLDER_PATH
#   DEPLOY_HOST   (and optionally DEPLOY_KEY)

set -euo pipefail

log()  { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m!! \033[0m %s\n' "$*" >&2; }
fail() { printf '\033[1;31mxx \033[0m %s\n' "$*" >&2; exit 1; }

require() {
  local v=$1
  [ -n "${!v:-}" ] || fail "missing env: $v (set in .env)"
}

require GUI_USER
require GUI_PASSWORD
require FOLDER_ID
require FOLDER_PATH
require DEPLOY_HOST

SSH=(ssh)
SCP=(scp)
if [ -n "${DEPLOY_KEY:-}" ]; then
  SSH=(ssh -i "$DEPLOY_KEY")
  SCP=(scp -i "$DEPLOY_KEY")
fi

# ---------------------------------------------------------------------------
# Helpers

# Resolve Syncthing config dir for the current OS. Echoes the path that
# contains config.xml.
local_config_dir() {
  if [ -d "$HOME/Library/Application Support/Syncthing" ]; then
    echo "$HOME/Library/Application Support/Syncthing"
  elif [ -d "$HOME/.local/state/syncthing" ]; then
    echo "$HOME/.local/state/syncthing"
  elif [ -d "$HOME/.config/syncthing" ]; then
    echo "$HOME/.config/syncthing"
  else
    fail "cannot find local Syncthing config dir"
  fi
}

# Print the API key from a Syncthing config.xml. The key sits in <apikey>...
# of the <gui> element. We use a tolerant grep+sed since adding xmllint as a
# dep would be overkill.
extract_apikey() {
  local xml=$1
  grep -o '<apikey>[^<]*</apikey>' "$xml" | head -1 | sed -E 's|</?apikey>||g'
}

remote_config_dir() {
  "${SSH[@]}" "$DEPLOY_HOST" '
    if   [ -d "$HOME/.local/state/syncthing" ];  then echo "$HOME/.local/state/syncthing"
    elif [ -d "$HOME/.config/syncthing" ];       then echo "$HOME/.config/syncthing"
    else echo ""
    fi
  '
}

# Wait until Syncthing answers /rest/system/ping. Args: api_key url
wait_ready() {
  local key=$1 url=$2 i
  for i in $(seq 1 30); do
    if curl -fsS -H "X-API-Key: $key" "$url/rest/system/ping" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  fail "Syncthing at $url not responding after 30s"
}

# Pretty-print a JSON field with a fallback if jq is missing.
jget() {
  local field=$1
  if command -v jq >/dev/null 2>&1; then
    jq -r ".$field"
  else
    # Best-effort regex extraction. Works for top-level string fields.
    grep -o "\"$field\"[[:space:]]*:[[:space:]]*\"[^\"]*\"" | head -1 | sed -E "s/.*\"$field\"[[:space:]]*:[[:space:]]*\"([^\"]*)\".*/\1/"
  fi
}

# REST helpers. CALL_LOCAL/CALL_REMOTE wrap curl with the right key + base URL.
call_local()  { curl -fsS -H "X-API-Key: $LOCAL_KEY" "$@"; }
call_remote() {
  # Run curl on the remote host so we don't need an SSH tunnel.
  local args=()
  for a in "$@"; do args+=("$(printf '%q' "$a")"); done
  "${SSH[@]}" "$DEPLOY_HOST" "curl -fsS -H 'X-API-Key: $REMOTE_KEY' ${args[*]}"
}

# ---------------------------------------------------------------------------
# Discover keys + IDs

log "discovering local Syncthing"
LOCAL_DIR=$(local_config_dir)
[ -f "$LOCAL_DIR/config.xml" ] || fail "no config.xml in $LOCAL_DIR (start syncthing once)"
LOCAL_KEY=$(extract_apikey "$LOCAL_DIR/config.xml")
[ -n "$LOCAL_KEY" ] || fail "could not read local API key"
LOCAL_URL=http://127.0.0.1:8384

log "waiting for local syncthing to answer"
wait_ready "$LOCAL_KEY" "$LOCAL_URL"

LOCAL_ID=$(call_local "$LOCAL_URL/rest/system/status" | jget myID)
[ -n "$LOCAL_ID" ] || fail "could not read local device id"
log "local device id: $LOCAL_ID"

log "discovering remote Syncthing on $DEPLOY_HOST"
REMOTE_DIR=$(remote_config_dir)
[ -n "$REMOTE_DIR" ] || fail "remote Syncthing config dir not found (run setup again after first start)"
REMOTE_KEY=$("${SSH[@]}" "$DEPLOY_HOST" "grep -o '<apikey>[^<]*</apikey>' '$REMOTE_DIR/config.xml' | head -1 | sed -E 's|</?apikey>||g'")
[ -n "$REMOTE_KEY" ] || fail "could not read remote API key"
REMOTE_URL=http://127.0.0.1:8384

log "waiting for remote syncthing to answer"
"${SSH[@]}" "$DEPLOY_HOST" "
  for i in \$(seq 1 30); do
    curl -fsS -H 'X-API-Key: $REMOTE_KEY' $REMOTE_URL/rest/system/ping >/dev/null 2>&1 && exit 0
    sleep 1
  done
  exit 1
" || fail "remote Syncthing did not become ready"

REMOTE_ID=$(call_remote "$REMOTE_URL/rest/system/status" | jget myID)
[ -n "$REMOTE_ID" ] || fail "could not read remote device id"
log "remote device id: $REMOTE_ID"

# ---------------------------------------------------------------------------
# GUI password (set on both, idempotent)
#
# We compare the user; for the password we cannot read the existing hash, so
# we always PATCH. Syncthing hashes the new value on save, no plaintext leaks.

set_gui_password() {
  local label=$1 caller=$2 url=$3
  local body
  body=$(printf '{"user":"%s","password":"%s"}' "$GUI_USER" "$GUI_PASSWORD")
  $caller -X PATCH -H 'Content-Type: application/json' -d "$body" "$url/rest/config/gui" >/dev/null
  log "$label GUI auth set (user=$GUI_USER)"
}
set_gui_password "local"  call_local  "$LOCAL_URL"
set_gui_password "remote" call_remote "$REMOTE_URL"

# ---------------------------------------------------------------------------
# Pair devices (idempotent)

ensure_device() {
  local label=$1 caller=$2 url=$3 peer_id=$4 peer_name=$5
  if $caller "$url/rest/config/devices/$peer_id" >/dev/null 2>&1; then
    log "$label already knows peer $peer_name"
    return
  fi
  local body
  body=$(printf '{"deviceID":"%s","name":"%s","addresses":["dynamic"]}' "$peer_id" "$peer_name")
  $caller -X PUT -H 'Content-Type: application/json' -d "$body" "$url/rest/config/devices/$peer_id" >/dev/null
  log "$label added peer $peer_name"
}
ensure_device "local"  call_local  "$LOCAL_URL"  "$REMOTE_ID" "remote"
ensure_device "remote" call_remote "$REMOTE_URL" "$LOCAL_ID"  "local"

# ---------------------------------------------------------------------------
# Shared folder (idempotent on both sides)
#
# Folder body shape: id, label, path, devices: [{deviceID}, {deviceID}].
# Including BOTH devices on each side avoids needing the user to "Accept" a
# pending share in the web UI.

folder_body() {
  local own_id=$1 peer_id=$2 path=$3
  cat <<EOF
{"id":"$FOLDER_ID","label":"Claude Home","path":"$path","type":"sendreceive","devices":[{"deviceID":"$own_id"},{"deviceID":"$peer_id"}],"rescanIntervalS":60}
EOF
}

ensure_folder() {
  local label=$1 caller=$2 url=$3 own_id=$4 peer_id=$5 path=$6
  if $caller "$url/rest/config/folders/$FOLDER_ID" >/dev/null 2>&1; then
    log "$label already has folder $FOLDER_ID"
    return
  fi
  local body
  body=$(folder_body "$own_id" "$peer_id" "$path")
  $caller -X PUT -H 'Content-Type: application/json' -d "$body" "$url/rest/config/folders/$FOLDER_ID" >/dev/null
  log "$label added folder $FOLDER_ID at $path"
}

# Expand ~ for the local path so Syncthing gets an absolute path.
LOCAL_PATH="${FOLDER_PATH/#\~/$HOME}"
REMOTE_PATH=$("${SSH[@]}" "$DEPLOY_HOST" "echo $FOLDER_PATH")  # remote $HOME

ensure_folder "local"  call_local  "$LOCAL_URL"  "$LOCAL_ID"  "$REMOTE_ID" "$LOCAL_PATH"
ensure_folder "remote" call_remote "$REMOTE_URL" "$REMOTE_ID" "$LOCAL_ID"  "$REMOTE_PATH"

# ---------------------------------------------------------------------------
# .stignore — produced by `claude-sync repair` on each host.

log "running claude-sync repair on local"
"$HOME/bin/claude-sync" repair >/dev/null || warn "local repair failed (non-fatal)"

log "running claude-sync repair on remote"
"${SSH[@]}" "$DEPLOY_HOST" '~/bin/claude-sync repair' >/dev/null || warn "remote repair failed (non-fatal)"

log "sync setup complete"
log "local web UI:  http://127.0.0.1:8384"
log "remote web UI: SSH-tunnel with: ssh -L 18384:127.0.0.1:8384 $DEPLOY_HOST   then visit http://127.0.0.1:18384"
