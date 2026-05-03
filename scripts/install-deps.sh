#!/usr/bin/env bash
# Install claude-sync prerequisites (Syncthing) for the host's package manager.
# Idempotent: re-running on an already-prepared host is a no-op aside from the
# package manager's own update step.

set -euo pipefail

log()  { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m!! \033[0m %s\n' "$*" >&2; }
fail() { printf '\033[1;31mxx \033[0m %s\n' "$*" >&2; exit 1; }

OS=$(uname -s)

install_macos() {
  if ! command -v brew >/dev/null 2>&1; then
    fail "Homebrew not found. Install from https://brew.sh first."
  fi
  if command -v syncthing >/dev/null 2>&1; then
    log "syncthing already installed: $(syncthing --version | head -1)"
  else
    log "installing syncthing via brew"
    brew install syncthing
  fi
  # brew services keeps it running across reboots; ignore failure if launchd
  # is already managing it.
  log "ensuring syncthing service is started"
  brew services start syncthing >/dev/null 2>&1 || true
  log "syncthing web UI: http://127.0.0.1:8384"
}

install_linux() {
  if [ ! -f /etc/os-release ]; then
    fail "cannot detect Linux distribution (no /etc/os-release)"
  fi
  # shellcheck disable=SC1091
  . /etc/os-release

  if command -v syncthing >/dev/null 2>&1; then
    log "syncthing already installed: $(syncthing --version | head -1)"
  else
    case "${ID:-}${ID_LIKE:-}" in
      *ubuntu*|*debian*)
        log "installing syncthing from upstream apt repo"
        sudo apt-get update -qq
        sudo apt-get install -y curl gpg
        sudo install -d -m 0755 /etc/apt/keyrings
        curl -fsSL https://syncthing.net/release-key.gpg \
          | sudo tee /etc/apt/keyrings/syncthing.gpg >/dev/null
        echo "deb [signed-by=/etc/apt/keyrings/syncthing.gpg] https://apt.syncthing.net/ syncthing stable" \
          | sudo tee /etc/apt/sources.list.d/syncthing.list >/dev/null
        sudo apt-get update -qq
        sudo apt-get install -y syncthing
        ;;
      *fedora*|*rhel*|*centos*)
        log "installing syncthing via dnf"
        sudo dnf install -y syncthing
        ;;
      *arch*)
        log "installing syncthing via pacman"
        sudo pacman -S --needed --noconfirm syncthing
        ;;
      *)
        fail "unsupported distro: ${ID:-unknown}. Install syncthing manually."
        ;;
    esac
  fi

  # systemd user service. enable-linger keeps it running after the user logs out.
  if command -v systemctl >/dev/null 2>&1; then
    log "enabling syncthing as a user service"
    systemctl --user enable --now syncthing.service 2>/dev/null \
      || warn "systemctl --user failed (non-fatal; run manually if needed)"
    if command -v loginctl >/dev/null 2>&1; then
      sudo loginctl enable-linger "$USER" 2>/dev/null \
        || warn "loginctl enable-linger failed; service won't survive logout"
    fi
  else
    warn "systemd not available; start syncthing manually"
  fi
  log "syncthing web UI: http://127.0.0.1:8384 (SSH-tunnel from your laptop to access)"
}

case "$OS" in
  Darwin) install_macos ;;
  Linux)  install_linux ;;
  *)      fail "unsupported OS: $OS" ;;
esac

log "done"
