# Changelog

All notable changes to this project are documented in this file.
Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
versioning: [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2026-05-03

### Bug Fixes

- **sessions:** Pass --resume UUID as discrete argv, name tmux window (a7ddc72)

- **sessions:** Match Claude's exact custom-title shape and update list optimistically (50c8792)

- **sessions:** Rename refresh + c key resilience after stray migration (9e8a367)

- **sessions:** Write custom-title on R, open tmux new-window on c (663b87c)


### Chore

- Rename module path from github.com/fikret to github.com/fpirim (696d04a)

- Add license, funding, and release automation (309cbcc)


### Documentation

- Add README and CLAUDE.md, override global CLAUDE.md ignore (3bb4b50)


### Features

- **sessions:** C key resumes a session via claude --resume (f6fbccf)

- **tui:** Tab 3 Sync UI with Syncthing REST client (de46b5e)

- **sessions:** Native rename + 2-line list rows + AITitle fallback (caf0027)


### Other

- Initial commit: claude-sync TUI for cross-device Claude Code session sync

Brings up an end-to-end pipeline that lets multiple machines share Claude
Code's ~/.claude state (sessions, memory, todos, settings, commands) via
Syncthing while keeping per-machine encoded project directories pointing at
a single _shared/ store.

Highlights:
- CLI: add / repair / status / doctor / tui subcommands (cobra)
- Bubble Tea TUI with Projects, Sessions, Config tabs, indigo theme,
  shared/stray detection, modal dialogs, scrollbar, focus toggle, help popup
- fsops: idempotent symlink + migrate (sha256 verify), .stignore generator,
  scan, repair (dry-run capable)
- Sessions JSONL transcript renderer (user/assistant turns with tool notes)
- Makefile setup target: installs Syncthing locally and on a remote host,
  cross-builds and deploys the binary, auto-pairs the two Syncthing
  instances, sets GUI auth, shares the claude-home folder, runs repair on
  both ends. Idempotent — safe to re-run

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com> (1d40e5d)


### UX

- **sessions:** Pre-fill rename modal + yellow title in list (da7c71c)



