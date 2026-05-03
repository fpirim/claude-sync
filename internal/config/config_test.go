package config

import (
	"path/filepath"
	"testing"
)

func TestLoadMissing(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "nope.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Version != SchemaVersion {
		t.Errorf("version = %d", c.Version)
	}
	if c.Syncthing.FolderID != "claude-home" {
		t.Errorf("default folder id missing")
	}
}

func TestSaveLoadRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	in := Default()
	in.Projects["foo"] = Project{Paths: map[string]string{
		"macbook": "/Users/fikret/code/foo",
		"oracle":  "/home/ubuntu/dev/foo",
	}}
	if err := Save(path, in); err != nil {
		t.Fatal(err)
	}
	out, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if out.Projects["foo"].Paths["macbook"] != "/Users/fikret/code/foo" {
		t.Errorf("roundtrip mismatch: %+v", out.Projects["foo"])
	}
	// Save again -> .bak should exist
	if err := Save(path, in); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path + ".bak"); err != nil {
		t.Errorf(".bak not created: %v", err)
	}
}

func TestLock(t *testing.T) {
	dir := t.TempDir()
	l, err := AcquireLock(filepath.Join(dir, ".lock"))
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Release(); err != nil {
		t.Fatal(err)
	}
}
