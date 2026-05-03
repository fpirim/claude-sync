package sessions

import (
	"os"
	"path/filepath"
	"testing"
)

const sampleJSONL = `{"type":"permission-mode","permissionMode":"default","sessionId":"abc"}
{"parentUuid":null,"type":"user","message":{"role":"user","content":"hello world from test"},"uuid":"u1","timestamp":"2026-05-02T14:03:07.855Z","cwd":"/Users/fikret/foo","sessionId":"abc"}
{"type":"ai-title","aiTitle":"Initial AI title","sessionId":"abc"}
{"type":"assistant","message":{"role":"assistant","content":"hi there"},"uuid":"a1","timestamp":"2026-05-02T14:03:08.000Z","sessionId":"abc"}
{"type":"user","message":{"role":"user","content":[{"type":"text","text":"second user msg"}]},"uuid":"u2","timestamp":"2026-05-02T14:03:09.000Z","sessionId":"abc"}
{"type":"ai-title","aiTitle":"Refined title later","sessionId":"abc"}
`

func TestListProject(t *testing.T) {
	dir := t.TempDir()
	proj := filepath.Join(dir, "foo")
	os.MkdirAll(proj, 0o755)
	os.WriteFile(filepath.Join(proj, "abc.jsonl"), []byte(sampleJSONL), 0o644)

	out, err := ListProject(dir, "foo")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("got %d sessions", len(out))
	}
	s := out[0]
	if s.Preview != "hello world from test" {
		t.Errorf("preview = %q", s.Preview)
	}
	if s.MsgCount != 3 {
		t.Errorf("msgcount = %d", s.MsgCount)
	}
	if s.Cwd != "/Users/fikret/foo" {
		t.Errorf("cwd = %q", s.Cwd)
	}
	if s.FirstAt.IsZero() {
		t.Error("FirstAt zero")
	}
	// Latest ai-title wins (Refined > Initial).
	if s.AITitle != "Refined title later" {
		t.Errorf("AITitle = %q (want %q)", s.AITitle, "Refined title later")
	}
	// LastText is the most recent user/assistant body — assistant "hi there"
	// is followed by user "second user msg", so the latter wins.
	if s.LastText != "second user msg" {
		t.Errorf("LastText = %q (want %q)", s.LastText, "second user msg")
	}
}

// TestRenameAppendsAITitle verifies the new native-rename behavior: Rename
// must write a Claude-compatible ai-title record into the JSONL itself, not
// just into the .meta sidecar.
func TestRenameAppendsAITitle(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "abc.jsonl")
	os.WriteFile(p, []byte(`{"type":"user","message":{"role":"user","content":"hi"},"sessionId":"abc"}`+"\n"), 0o644)

	if err := Rename(p, "My Renamed Session"); err != nil {
		t.Fatal(err)
	}
	out, err := ListProject(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("got %d sessions", len(out))
	}
	if out[0].AITitle != "My Renamed Session" {
		t.Errorf("AITitle after rename = %q", out[0].AITitle)
	}
}

// TestRenameClearsLegacySidecarTitle: if a session has an old meta.Title
// from before the native-rename change, Rename should drop it so the new
// JSONL ai-title becomes the canonical source.
func TestRenameClearsLegacySidecarTitle(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "abc.jsonl")
	os.WriteFile(p, []byte("{}\n"), 0o644)
	// Seed a legacy sidecar title directly.
	if err := SaveMeta(p, Meta{Title: "Old Sidecar Title"}); err != nil {
		t.Fatal(err)
	}
	if err := Rename(p, "Fresh Title"); err != nil {
		t.Fatal(err)
	}
	m, _ := LoadMeta(p)
	if m.Title != "" {
		t.Errorf("legacy meta.Title still set: %q", m.Title)
	}
}

func TestTagsRoundtrip(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.jsonl")
	os.WriteFile(p, []byte("{}"), 0o644)
	if err := AddTag(p, "important"); err != nil {
		t.Fatal(err)
	}
	if err := AddTag(p, "important"); err != nil { // idempotent
		t.Fatal(err)
	}
	m, _ := LoadMeta(p)
	if len(m.Tags) != 1 || m.Tags[0] != "important" {
		t.Errorf("tags = %+v", m.Tags)
	}
}
