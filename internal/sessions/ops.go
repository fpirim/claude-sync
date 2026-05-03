package sessions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Rename gives the session a friendly title by APPENDING a Claude-native
// custom-title record to the JSONL. Claude Code uses TWO title record
// types, with strict precedence:
//
//   custom-title  — the user's manual rename (highest priority)
//   ai-title      — Claude's automatically refined title
//
// We pick custom-title because the rename is an explicit user action,
// equivalent to Claude's own /rename command. Writing ai-title would be
// shadowed the moment any older custom-title exists in the file.
//
// Implementation notes:
//   - O_APPEND makes the write atomic per call (single line < PIPE_BUF
//     stays whole even if Claude is also writing). Multiple writers append
//     safely at line boundaries on POSIX.
//   - Any pre-existing meta.Title (from the legacy sidecar-only renamer)
//     is cleared so the JSONL custom-title becomes the canonical source
//     going forward. Tags and archived flags stay in the sidecar.
func Rename(jsonlPath, title string) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return fmt.Errorf("title is empty")
	}
	sid := strings.TrimSuffix(filepath.Base(jsonlPath), ".jsonl")

	// Match Claude's exact custom-title shape — same fields, same order, no
	// timestamp. Empirically Claude treats records WITH a timestamp as
	// auto-generated metadata and ignores them for the displayed name; only
	// timestamp-less custom-title records are honored as the user's rename.
	rec := struct {
		Type        string `json:"type"`
		CustomTitle string `json:"customTitle"`
		SessionID   string `json:"sessionId"`
	}{
		Type:        "custom-title",
		CustomTitle: title,
		SessionID:   sid,
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	b = append(b, '\n')

	f, err := os.OpenFile(jsonlPath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("open jsonl for append: %w", err)
	}
	defer f.Close()

	// If the file doesn't end with a newline, prepend one so our object
	// starts on its own line. Cheap check: stat size, read last byte.
	if info, err := f.Stat(); err == nil && info.Size() > 0 {
		var last [1]byte
		if _, err := os.NewFile(f.Fd(), "").ReadAt(last[:], info.Size()-1); err == nil {
			if last[0] != '\n' {
				b = append([]byte{'\n'}, b...)
			}
		}
	}

	if _, err := f.Write(b); err != nil {
		return fmt.Errorf("append ai-title: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("fsync: %w", err)
	}

	// Drop legacy sidecar Title so the new JSONL ai-title isn't shadowed.
	if m, err := LoadMeta(jsonlPath); err == nil && m.Title != "" {
		m.Title = ""
		_ = SaveMeta(jsonlPath, m) // best-effort; not fatal
	}
	return nil
}

// SetArchived flips the archived flag.
func SetArchived(jsonlPath string, archived bool) error {
	m, err := LoadMeta(jsonlPath)
	if err != nil {
		return err
	}
	m.Archived = archived
	return SaveMeta(jsonlPath, m)
}

// AddTag appends a tag if absent.
func AddTag(jsonlPath, tag string) error {
	m, err := LoadMeta(jsonlPath)
	if err != nil {
		return err
	}
	for _, t := range m.Tags {
		if t == tag {
			return nil
		}
	}
	m.Tags = append(m.Tags, tag)
	return SaveMeta(jsonlPath, m)
}

// Delete removes both the JSONL and its sidecar.
// Caller is responsible for confirmation.
func Delete(jsonlPath string) error {
	if err := os.Remove(jsonlPath); err != nil {
		return fmt.Errorf("remove %s: %w", jsonlPath, err)
	}
	if err := os.Remove(MetaPathFor(jsonlPath)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove sidecar: %w", err)
	}
	return nil
}
