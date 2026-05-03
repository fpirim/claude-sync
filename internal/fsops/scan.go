package fsops

import (
	"os"
	"path/filepath"
	"strings"
)

// ProjectsScan reports what's currently in ~/.claude/projects/ at a high level.
type ProjectsScan struct {
	// EncodedDirs are entries directly under projects/, keyed by name.
	EncodedDirs map[string]EncodedDirInfo
	// SharedProjects are subdirs of projects/_shared/, keyed by project name.
	SharedProjects map[string]SharedInfo
}

type EncodedDirInfo struct {
	Path        string
	IsSymlink   bool
	LinkTarget  string // resolved target if symlink (absolute)
	IsDir       bool   // real dir
	Empty       bool   // real dir with no entries
}

type SharedInfo struct {
	Path     string
	NumFiles int
	HasConflicts bool
}

// ScanProjects walks projectsDir (= ~/.claude/projects) and sharedDir (= projectsDir/_shared)
// returning a snapshot. Missing directories are not errors; they yield empty maps.
func ScanProjects(projectsDir, sharedDir string) (ProjectsScan, error) {
	out := ProjectsScan{
		EncodedDirs:    map[string]EncodedDirInfo{},
		SharedProjects: map[string]SharedInfo{},
	}
	ents, err := os.ReadDir(projectsDir)
	if err != nil && !os.IsNotExist(err) {
		return out, err
	}
	for _, e := range ents {
		if e.Name() == "_shared" {
			continue
		}
		p := filepath.Join(projectsDir, e.Name())
		info, err := os.Lstat(p)
		if err != nil {
			continue
		}
		ed := EncodedDirInfo{Path: p}
		if info.Mode()&os.ModeSymlink != 0 {
			ed.IsSymlink = true
			if t, err := os.Readlink(p); err == nil {
				if !filepath.IsAbs(t) {
					t = filepath.Join(filepath.Dir(p), t)
				}
				abs, _ := filepath.Abs(t)
				ed.LinkTarget = abs
			}
		} else if info.IsDir() {
			ed.IsDir = true
			sub, _ := os.ReadDir(p)
			ed.Empty = len(sub) == 0
		}
		out.EncodedDirs[e.Name()] = ed
	}

	sents, err := os.ReadDir(sharedDir)
	if err != nil && !os.IsNotExist(err) {
		return out, err
	}
	for _, e := range sents {
		if !e.IsDir() {
			continue
		}
		sp := filepath.Join(sharedDir, e.Name())
		files, _ := os.ReadDir(sp)
		si := SharedInfo{Path: sp}
		for _, f := range files {
			if f.IsDir() {
				continue
			}
			si.NumFiles++
			if strings.Contains(f.Name(), ".sync-conflict-") {
				si.HasConflicts = true
			}
		}
		out.SharedProjects[e.Name()] = si
	}
	return out, nil
}
