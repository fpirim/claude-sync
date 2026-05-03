// Package config models ~/.claude/sync/config.yml: projects, syncthing, policies.
//
// All file operations are safe for concurrent invocations of claude-sync via
// a flock(2) on ~/.claude/sync/.lock. Writes are atomic: temp file + rename,
// with a single .bak rotation kept beside the live file.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

const SchemaVersion = 1

// Config is the full document.
type Config struct {
	Version   int                `yaml:"version"`
	Projects  map[string]Project `yaml:"projects"`
	Syncthing Syncthing          `yaml:"syncthing"`
	Policies  Policies           `yaml:"policies"`
}

// Project entry in the projects map.
type Project struct {
	// Paths maps machine name -> absolute filesystem path on that machine.
	Paths    map[string]string `yaml:"paths"`
	Aliases  []string          `yaml:"aliases,omitempty"`
	Archived bool              `yaml:"archived,omitempty"`
}

// Syncthing holds the state needed to manage the daemon's folder + peers.
type Syncthing struct {
	FolderID    string                  `yaml:"folder_id"`
	FolderLabel string                  `yaml:"folder_label"`
	FolderPath  string                  `yaml:"folder_path"`
	Devices     map[string]SyncDevice   `yaml:"devices,omitempty"`
	Endpoint    string                  `yaml:"endpoint,omitempty"` // e.g. http://127.0.0.1:8384
}

type SyncDevice struct {
	ID      string `yaml:"id"`
	Address string `yaml:"address,omitempty"` // "dynamic" or tcp://host:port
}

// Policies bundles configurable behaviors.
type Policies struct {
	ConflictResolution  string `yaml:"conflict_resolution"`   // keep-both | newest-wins | manual
	SymlinkOrphanAction string `yaml:"symlink_orphan_action"` // prompt | delete | keep
}

// Default returns a fresh, empty config with sensible defaults.
func Default() Config {
	return Config{
		Version:  SchemaVersion,
		Projects: map[string]Project{},
		Syncthing: Syncthing{
			FolderID:    "claude-home",
			FolderLabel: "Claude Home",
			FolderPath:  "~/.claude",
			Endpoint:    "http://127.0.0.1:8384",
			Devices:     map[string]SyncDevice{},
		},
		Policies: Policies{
			ConflictResolution:  "keep-both",
			SymlinkOrphanAction: "delete",
		},
	}
}

// Load parses config from disk. Returns Default() if the file does not exist.
func Load(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Default(), nil
		}
		return Config{}, err
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if c.Version == 0 {
		c.Version = SchemaVersion
	}
	if c.Projects == nil {
		c.Projects = map[string]Project{}
	}
	if c.Syncthing.Devices == nil {
		c.Syncthing.Devices = map[string]SyncDevice{}
	}
	return c, nil
}

// Save writes the config atomically: temp file in same dir, fsync, rename.
// The previous live file is rotated to <path>.bak (single generation).
func Save(path string, c Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.yml")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName) // best-effort if rename fails
	}()
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// Rotate existing -> .bak
	if _, err := os.Stat(path); err == nil {
		_ = os.Rename(path, path+".bak")
	}
	return os.Rename(tmpName, path)
}

// Mutex for in-process serialization. The flock handles cross-process.
var saveMu sync.Mutex

// SaveLocked acquires the process-level mutex around Save.
func SaveLocked(path string, c Config) error {
	saveMu.Lock()
	defer saveMu.Unlock()
	return Save(path, c)
}
