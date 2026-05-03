package fsops

import (
	"fmt"
	"os"
	"path/filepath"
)

// SymlinkState describes what is at linkPath right now.
type SymlinkState int

const (
	StateMissing       SymlinkState = iota // nothing at linkPath
	StateCorrectLink                       // symlink already points to target
	StateWrongLink                         // symlink to a different target
	StateRealDirEmpty                      // a real directory but empty
	StateRealDirData                       // a real directory with files (needs migration)
	StateOtherFile                         // a regular file or odd thing — refuse to touch
)

// InspectLink reports the state of linkPath relative to wantTarget.
// wantTarget should be an absolute path. linkPath may or may not exist.
func InspectLink(linkPath, wantTarget string) (SymlinkState, error) {
	info, err := os.Lstat(linkPath)
	if err != nil {
		if os.IsNotExist(err) {
			return StateMissing, nil
		}
		return 0, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		got, err := os.Readlink(linkPath)
		if err != nil {
			return 0, err
		}
		// Resolve relative readlink against linkPath's dir.
		if !filepath.IsAbs(got) {
			got = filepath.Join(filepath.Dir(linkPath), got)
		}
		gotAbs, err := filepath.Abs(got)
		if err != nil {
			return 0, err
		}
		wantAbs, err := filepath.Abs(wantTarget)
		if err != nil {
			return 0, err
		}
		if gotAbs == wantAbs {
			return StateCorrectLink, nil
		}
		return StateWrongLink, nil
	}
	if info.IsDir() {
		ents, err := os.ReadDir(linkPath)
		if err != nil {
			return 0, err
		}
		if len(ents) == 0 {
			return StateRealDirEmpty, nil
		}
		return StateRealDirData, nil
	}
	return StateOtherFile, nil
}

// EnsureSymlink makes linkPath point to target, idempotently.
// It refuses to clobber a real directory that contains data — caller must
// migrate first. Returns true if any change was made.
func EnsureSymlink(linkPath, target string) (bool, error) {
	st, err := InspectLink(linkPath, target)
	if err != nil {
		return false, err
	}
	switch st {
	case StateCorrectLink:
		return false, nil
	case StateMissing:
		return true, os.Symlink(target, linkPath)
	case StateWrongLink, StateRealDirEmpty:
		if err := os.Remove(linkPath); err != nil {
			return false, err
		}
		return true, os.Symlink(target, linkPath)
	case StateRealDirData:
		return false, fmt.Errorf("%s contains data; migrate before linking", linkPath)
	case StateOtherFile:
		return false, fmt.Errorf("%s is not a directory or symlink; refusing to touch", linkPath)
	}
	return false, nil
}
