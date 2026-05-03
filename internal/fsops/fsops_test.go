package fsops

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fpirim/claude-sync/internal/config"
	"github.com/fpirim/claude-sync/internal/paths"
)

func TestEnsureSymlinkIdempotent(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "link")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	changed, err := EnsureSymlink(link, target)
	if err != nil || !changed {
		t.Fatalf("first call: changed=%v err=%v", changed, err)
	}
	changed, err = EnsureSymlink(link, target)
	if err != nil || changed {
		t.Fatalf("second call should be no-op: changed=%v err=%v", changed, err)
	}
}

func TestMigrateBasic(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"a.jsonl", "b.jsonl"} {
		if err := os.WriteFile(filepath.Join(src, n), []byte(n+"-data"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	rep, err := Migrate(src, dst, "test-host", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Copied) != 2 {
		t.Errorf("copied=%d", len(rep.Copied))
	}
	if _, err := os.Stat(filepath.Join(dst, "a.jsonl")); err != nil {
		t.Errorf("a not at dst: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("src not removed: %v", err)
	}
}

func TestMigrateIdenticalSkipped(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	os.MkdirAll(src, 0o755)
	os.MkdirAll(dst, 0o755)
	os.WriteFile(filepath.Join(src, "a.jsonl"), []byte("same"), 0o644)
	os.WriteFile(filepath.Join(dst, "a.jsonl"), []byte("same"), 0o644)
	rep, err := Migrate(src, dst, "h", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Skipped) != 1 || len(rep.Copied) != 0 {
		t.Errorf("rep=%+v", rep)
	}
}

func TestMigrateConflict(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	os.MkdirAll(src, 0o755)
	os.MkdirAll(dst, 0o755)
	os.WriteFile(filepath.Join(src, "a.jsonl"), []byte("AAA"), 0o644)
	os.WriteFile(filepath.Join(dst, "a.jsonl"), []byte("BBB"), 0o644)
	rep, err := Migrate(src, dst, "h", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Conflict) != 1 {
		t.Errorf("expected conflict, got %+v", rep)
	}
}

func TestRepairIdempotent(t *testing.T) {
	home := t.TempDir()
	p := paths.New(home)
	if err := p.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Projects["foo"] = config.Project{Paths: map[string]string{
		"testhost": filepath.Join(home, "fake-cwd", "foo"),
	}}
	os.MkdirAll(filepath.Join(home, "fake-cwd", "foo"), 0o755)

	r1, err := Repair(p, cfg, "testhost", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(r1.Actions) == 0 {
		t.Error("expected actions on first repair")
	}
	r2, err := Repair(p, cfg, "testhost", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range r2.Actions {
		if a.Kind != "noop" {
			t.Errorf("second repair not idempotent: %+v", a)
		}
	}
}
