# claude-sync

Sync Claude Code's `~/.claude` state across multiple machines and manage
session history (list, rename, delete) from a single TUI — backed by
[Syncthing](https://syncthing.net) for transport.

```
~/.claude/
├── projects/
│   ├── _shared/              ← real session JSONLs (synced)
│   │   ├── foo/
│   │   └── bar/
│   ├── -Users-fikret-code-foo  → _shared/foo   (per-machine symlink)
│   └── -home-fikret-dev-foo    → _shared/bar   (different host)
├── memory/                   (synced)
├── todos/                    (synced)
├── commands/                 (synced)
├── settings.json             (synced)
├── .credentials.json         (NEVER synced)
└── sync/
    ├── config.yml            (synced — projects, peers, policies)
    └── machine               (local override — not synced)
```

Each machine sees its own encoded `~/.claude/projects/<encoded-cwd>`
directory pointing at the same `_shared/<project>/`. So `claude --resume`
finds sessions natively, no matter which device started them.

## Why

Claude Code stores per-project session history under
`~/.claude/projects/<encoded-realpath>/<uuid>.jsonl`, with `<encoded-realpath>`
being the absolute resolved path with `/` and `.` collapsed to `-`. Because
the encoded directory is realpath-derived, the same project at different
absolute paths on different machines lands in different buckets — so
running Claude on a laptop and a server makes the histories invisible to
each other even if you sync the home directory.

`claude-sync` solves this by:

1. Migrating each per-machine encoded directory into a shared
   `_shared/<project>/` and replacing the original with a symlink.
2. Tracking a per-project mapping of `<machine> → <abs path>` in a synced
   `config.yml`, so each device knows where the project lives locally.
3. Wiring Syncthing to share the whole `~/.claude` minus a strict ignore
   list (`.credentials.json`, `statsig/`, runtime state, etc.).
4. Providing a TUI to browse, rename, and delete sessions; manage strays
   (un-registered Claude history) and adopt them; and watch Syncthing
   status live.

## Quickstart

```sh
# 1. Build + install the binary on every machine
git clone <this repo> claude-sync
cd claude-sync
make install                      # builds and copies to ~/bin/claude-sync

# 2. Configure deployment (only on the laptop you drive everything from)
cp .env.example .env
# edit .env:
#   DEPLOY_HOST=user@your-server
#   DEPLOY_KEY=~/path/to/key      # optional — omit if SSH config handles it
#   DEPLOY_ARCH=linux-arm64       # or linux-amd64
#   GUI_USER=admin
#   GUI_PASSWORD=<choose-one>     # applied to both Syncthing GUIs

# 3. One-shot bootstrap (idempotent — re-run anytime)
make setup
```

`make setup` does, top-to-bottom:

1. Install Syncthing locally via the right package manager (brew/apt/dnf/pacman)
2. Build and install the local binary
3. SSH to the remote host, install Syncthing there (systemd user service)
4. Cross-build and deploy the linux binary to `~/bin/claude-sync` on the remote
5. Discover both Syncthing API keys, set GUI auth, pair the two devices,
   share the `claude-home` folder, and run `claude-sync repair` on each end
   so the `.stignore` lands and any existing project dirs migrate

After this, **bidirectional file sync is live** between the two machines.

## Day-to-day

Open the TUI:

```sh
claude-sync                     # default action: launch TUI
```

| key   | tab        | what it does                                                |
| ----- | ---------- | ----------------------------------------------------------- |
| `1`   | Projects   | list registered projects + adopt strays (un-tracked dirs)   |
| `2`   | Sessions   | drill into a project, view JSONL transcripts, rename/delete |
| `3`   | Sync       | live Syncthing status: peers, folder state, conflicts       |
| `4`   | Config     | view/edit `config.yml` in `$EDITOR`                         |
| `?`   | (any)      | popup with the active tab's shortcuts                       |
| `q`   | (any)      | quit                                                        |

### Adding a new project

In any tab, register a project this machine should sync:

```sh
claude-sync add my-app /Users/fikret/code/my-app
claude-sync repair                # creates the symlink + migrates data
```

Or do both at once from the Projects tab: `a` → name → path. After repair,
open Claude Code in that directory and your sessions will land in the
shared history visible across devices.

### Topology

Syncthing connects devices peer-to-peer with TLS-encrypted transport. A
public Oracle (or any always-on server) acts as a de-facto hub because
it's always online, but **all data still flows directly between peers** —
there's no central storage.

If a peer can't reach another directly (NAT/firewall), Syncthing's open
relay network forwards the encrypted traffic. Relay servers can't read it.

### Conflicts

If two machines edit the same JSONL while both are offline, Syncthing
produces a `<file>.sync-conflict-<date>-<time>-<deviceID>.jsonl` next to
the original on the second-to-sync device. The Sync tab surfaces these
under the folder section.

By default `~/.claude/sync/config.yml` keeps both copies — you'll see the
conflict file in the project's session list and can decide which to keep.

### Removing things

* **Remove a project from this host only** — Tab 1 → `D` → "Remove from
  $host?" — local symlink is deleted, project entry stays for other hosts.
* **Delete a project from config entirely** — Tab 1 → `D` on a project
  that's not on this host → "Delete from config?". `_shared/` data is
  kept; you can clear sessions individually in Tab 2.
* **Delete a single session** — Tab 2 → select → `D`.

## Configuration

### `.env` (gitignored, host-specific)

```sh
DEPLOY_HOST=user@host
DEPLOY_KEY=/path/to/ssh-key       # optional
DEPLOY_ARCH=linux-arm64           # linux-arm64 | linux-amd64
GUI_USER=admin
GUI_PASSWORD=changeme
FOLDER_ID=claude-home             # optional — default
FOLDER_PATH=~/.claude             # optional — default
```

### `~/.claude/sync/config.yml` (synced)

```yaml
version: 1

projects:
  my-app:
    paths:
      laptop:  /Users/fikret/code/my-app
      server:  /home/ubuntu/dev/my-app

syncthing:
  folder_id: claude-home
  folder_label: Claude Home
  folder_path: ~/.claude
  endpoint: http://127.0.0.1:8384
  devices:
    laptop: { id: "ABCDE-…", address: "dynamic" }
    server: { id: "FGHIJ-…", address: "tcp://oracle.example.com:22000" }

policies:
  conflict_resolution: keep-both     # keep-both | newest-wins | manual
  symlink_orphan_action: delete      # delete | prompt | keep
```

`config.yml` is itself synced, so editing it on one device propagates
everywhere. `repair` is idempotent and self-healing.

### `~/.claude/sync/machine` (local override, NOT synced)

A single line containing this device's identifier. Defaults to the
sanitized hostname; create the file to override.

## CLI reference

```
claude-sync                       # → TUI
claude-sync repair [--dry-run]    # reconcile filesystem with config
claude-sync add <name> <path>     # register a project on this machine
claude-sync status [--json]       # per-project sync state
claude-sync doctor                # health check (.stignore, credentials, links)
claude-sync session list          # list sessions across projects
claude-sync session rename <uuid> "Title"
claude-sync session delete <uuid>
```

## Make targets

```
make setup     # full bootstrap: deps + binary on local AND remote (reads .env)
make install   # build and place binary in $HOME/bin
make deploy    # cross-build linux binary and push to remote
make build     # build local binary into ./claude-sync
make dist      # cross-compile darwin-arm64 + linux-{arm64,amd64}
make test      # go test ./...
```

## Troubleshooting

* **TUI shows "Sync — unavailable"** — Syncthing isn't running. Check
  `brew services list` (macOS) or `systemctl --user status syncthing`
  (Linux). Re-run `make setup` to re-bootstrap.
* **`.credentials.json` synced by accident** — `claude-sync doctor`
  flags it. The canonical `.stignore` excludes it; if it's already on a
  peer, delete it from the other devices first, then re-run repair.
* **Two devices have the same hostname** — write `~/.claude/sync/machine`
  on each with a unique identifier.
* **Sessions don't appear after sync** — verify the project is registered
  on both machines (`claude-sync status`); each machine needs an entry
  under `projects.<name>.paths.<machine>`.

## Architecture

| package                     | role                                                          |
| --------------------------- | ------------------------------------------------------------- |
| `cmd/`                      | Cobra subcommands (root, repair, add, status, doctor, tui)    |
| `internal/encoder`          | cwd ↔ encoded directory name (lossy: `/` and `.` both → `-`)  |
| `internal/paths`            | centralizes `~/.claude` directory locations                   |
| `internal/machine`          | resolves machine name (file > hostname)                       |
| `internal/config`           | YAML config load/save with flock + atomic write + `.bak`      |
| `internal/fsops`            | symlink + migrate (sha256 verify) + .stignore + repair        |
| `internal/sessions`         | JSONL streaming, transcript renderer, .meta.json sidecars     |
| `internal/syncthing`        | minimal REST client + `config.xml` discovery                  |
| `internal/tui`              | Bubble Tea program: Projects, Sessions, Sync, Config tabs     |
| `scripts/install-deps.sh`   | OS-aware Syncthing installer used by `make setup`             |
| `scripts/sync-setup.sh`     | pairs two Syncthing instances + shares the folder             |

Single static Go binary, no runtime deps. Tested on macOS arm64 and
Linux arm64/amd64.
