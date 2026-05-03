# CLAUDE.md — guidance for Claude Code working on this repo

This file orients an AI assistant editing the codebase. The user-facing
overview is in `README.md`; this file captures the *internal* invariants
and pitfalls that aren't obvious from reading the code linearly.

## What the project is

A Go CLI + Bubble Tea TUI that orchestrates Syncthing to synchronize
Claude Code's `~/.claude` state across multiple machines while resolving
the per-machine encoded-path-divergence problem. See `README.md` for the
high-level mental model. The two key insights:

1. Claude Code encodes the **realpath** of the cwd into a directory under
   `~/.claude/projects/`, so the same logical project at different
   absolute paths on different machines lands in different buckets.
2. We store the actual data in `~/.claude/projects/_shared/<project>/`
   and replace the per-machine encoded directory with a **symlink** to
   it. Each machine's `config.yml` records the `<machine> → <abspath>`
   mapping. `_shared/` is what Syncthing replicates.

## Encoder — lossy and machine-tested

`internal/encoder/encoder.go` implements the rule:

```
realpath(cwd) → replace '/' and '.' with '-' → prepend '-'
```

Examples (verified empirically on macOS, Claude Code 2026-05):

```
/Users/fikret/code/foo       → -Users-fikret-code-foo
/private/tmp/test.dir/foo    → -private-tmp-test-dir-foo  (note: realpath of /tmp)
/Users/fikret/.dotfiles      → -Users-fikret--dotfiles    (dot becomes dash, runs together)
```

Decoding is lossy: `--` could mean `//` or `/.` or `..`. The
`DecodeCandidates` function enumerates plausible interpretations
(capped at 2¹⁰ to bound search), and `ResolveExisting` picks the first
one that matches a real on-disk path.

When changing the encoder rule, update both directions and the table
test in `encoder_test.go`. **Do not** add cleverness like preserving
case or special characters — the rule must match Claude Code exactly.

## lipgloss `.Width` / `.Height` contract

This burned us repeatedly. `Style.Width(N)` and `Style.Height(N)` set
the **content + padding** dimensions, NOT the total rendered size. With
a border applied, total rendered = `N + 2`. With padding, content area
shrinks accordingly.

To target a total render size of `paneW × h`:

```go
paneStyle.Width(paneW - 2).Height(h - 2).Render(content)
```

Lipgloss does **not** truncate when content overflows the configured
height; it lets the block grow, pushing surrounding elements off
screen. Always pre-clip content with `clipToHeight` (and `padToWidth`
for horizontal) before passing to `Style.Render`. The regression test
`TestSessionsViewFitsBody` exercises this for the sessions tab.

## Pane layout — Sessions tab specifically

The Sessions tab is the only place that uses two stacked panes glued by
a connector line:

```
╭─ top pane ───╮
│ header       │
│ list rows    │
├──────────────┤   ← connector: "├" + ─*innerW + "┤"
│ preview      │
│              │
╰──────────────╯
```

This is implemented as two separate lipgloss panes, the top with
`BorderBottom(false)` and the bottom with `BorderTop(false)`, plus a
manually rendered connector line in between. The math is:

```
total = (top top-border + top content + 0)        // listRowsH + 2 - 1 = listRowsH + 1
      + (connector)                               // 1
      + (0 + bottom content + bottom bottom-border) // previewH + 1
      = listRowsH + previewH + 3
```

Adjust `computeSplitForTwoPanes` if you need to tune defaults.

The active pane gets `colorIndigo`; the inactive pane gets
`colorIndigoDim`. `enter` from the list focuses the preview, `esc` /
`backspace` returns. Don't forward keys across the focus boundary —
when list is focused, `j/k` must NOT scroll the preview.

## Modal contract

`internal/tui/modal.go` is the single source of truth for input/confirm/
info dialogs. Sub-models emit `openModalMsg{M: ...}`; the root model
hosts the modal and **suppresses every global shortcut while it's open**
(including `q`, `r`, tab numbers). Esc always cancels.

Always go through `openModalMsg` rather than rendering inline forms —
we tried inline once, and the keybinding leakage made it unmaintainable.

For new dialog kinds extend `modalKind` and `Update` / `View`. Keep the
modal box width fixed (`Width(56)`) so the centered overlay stays stable.

## Idempotency invariants

* `fsops.Repair` MUST be idempotent. The 2nd run on unchanged state must
  produce only `"noop"` actions. The test `TestRepairIdempotent`
  enforces this. If you add a new repair step, gate it on a
  state-changed predicate.
* `make setup` MUST be safe to re-run. Each shell step inside it should
  detect "already done" and skip without erroring (e.g., Syncthing peer
  add returns 200 on a known device, rather than failing).
* `EnsureSymlink`, `EnsureStIgnore`, `applyAddRaw`, etc. all return
  `(changed bool, err error)` or equivalent. Don't introduce side
  effects in the *check* phase.

## Config file safety

`internal/config/config.go` saves with:

1. Marshal to bytes
2. Write to a tempfile in the same dir
3. fsync, close
4. Rename existing `config.yml` → `config.yml.bak`
5. Rename tempfile → `config.yml`

Combined with `flock(~/.claude/sync/.lock)` for cross-process safety.
Never write to `config.yml` directly — always go through `config.Save`
or `config.SaveLocked`.

## .stignore — security-critical

The canonical `.stignore` is in `internal/fsops/stignore.go`. Two
non-obvious requirements:

1. `.credentials.json` MUST be the first non-comment line. Never
   removed, never overridden by user policy.
2. The pattern `!projects/_shared/**` MUST appear **before** the broad
   `projects/` ignore. Syncthing evaluates patterns top-down for the
   `!` precedence rules to work cleanly. Without `**` the negation
   matches only the directory itself, not files inside.

`doctor` cross-checks these. Adding new ignored paths is fine; reorder
with care.

## Sub-model message routing

`internal/tui/app.go` routes messages to sub-models based on the active
tab, EXCEPT for messages that target a specific sub-model regardless
of focus:

```go
case configContentMsg, editorFinishedMsg:    // → cfgTab
case sessionsLoadedMsg, previewMsg:          // → sessions
case syncTickMsg, syncSnapshotMsg:           // → sync
```

If you add a new long-lived sub-model with async loads, you'll need to
add its message types to this list — otherwise the initial response
arrives while a different tab is focused and gets dropped.

## Testing conventions

* Pure-Go logic packages have unit tests next to them (encoder, machine,
  config, fsops, sessions, syncthing).
* TUI tests use the bubbletea `Update` loop directly with synthetic
  `WindowSizeMsg` and `KeyMsg` rather than running the program. See
  `tui_test.go` and `layout_test.go`.
* Filesystem tests use `t.TempDir()` + `paths.New(home)` to fully
  isolate from the real `~/.claude`. Tests must never mutate the
  user's actual home.
* `CLAUDE_HOME` env var (read in `paths.Default()`) lets you run the
  binary against an alternate home — useful for smoke tests without
  touching production data.

## Build & deploy

* `make build` → local binary
* `make dist` → cross-compile darwin-arm64 + linux-{arm64,amd64} into
  `dist/`. CGO disabled, `-trimpath`, `-s -w` for stripped binaries
  (~5 MB).
* `make deploy DEPLOY_HOST=...` → scp the right linux binary, install
  in `~/bin/claude-sync` on the remote.
* `.env` (gitignored) supplies `DEPLOY_HOST/KEY/ARCH` and Syncthing
  pairing creds for `make setup`.

## Conventions

* No external runtime dependencies — keep the binary self-contained.
  Adding a non-stdlib dependency requires a clear motivation.
* Comments on the *why*, not the *what*. The codebase is small enough
  that names + types are usually self-explanatory; comments should
  capture invariants, gotchas, and reference points (e.g., "lipgloss
  Width contract", "first non-comment line of .stignore must be …").
* All FS state changes go through `fsops` or `config`. UI code calls
  these — never `os.WriteFile` from a TUI handler.
* No emojis in code or docs. Visual indicators in the TUI (`✓ ✗ ●`)
  are intentional and serve a UX purpose.

## Pitfalls to avoid

* Don't forward unhandled keys to the inactive pane in Sessions —
  this causes the preview to scroll while the user is navigating
  the list. Each pane is strictly key-isolated.
* Don't call `Style.Render` on content of unknown size without
  pre-clipping. Lipgloss expands; the consequence is the global
  header/footer scrolling off-screen.
* Don't break the `_shared/` invariant: a project on this host means
  `<encoded> → _shared/<name>` symlink. If you migrate sessions, copy
  to `_shared` first, verify SHA, then delete the source. Never the
  other way around.
* Don't send raw plaintext credentials to log output. The Syncthing
  GUI password is set via PATCH on `/rest/config/gui` and Syncthing
  hashes it server-side; never store the plaintext anywhere except
  the gitignored `.env`.
* Don't add per-tab footer hints. The footer is intentionally minimal
  (`? help`); shortcuts live in the help modal opened by `?`.

## Quick map for first-time edits

* "Where do I add a new CLI subcommand?" → `cmd/<name>.go`, register
  via `rootCmd.AddCommand` in `init()`.
* "Where do I add a key binding to the Sessions tab?" → `sessions.go`
  `update()` switch on `msg.String()`. Update `help.go` to document it.
* "Where do I tune the default split between list and preview?" →
  `sessionsListHeight` and `computeSplitForTwoPanes` in `sessions.go`.
* "Where is the canonical `.stignore`?" → `internal/fsops/stignore.go`
  constant `StIgnoreContent`.
* "How do I expose a new Syncthing field?" → add to `types.go`, add a
  client method to `client.go`, surface in `tui/sync.go` view.
