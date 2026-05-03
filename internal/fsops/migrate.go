package fsops

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// MigrateReport describes what Migrate did (or would do in dry-run mode).
type MigrateReport struct {
	Copied   []string // basenames moved into target
	Skipped  []string // identical files already present at target
	Conflict []string // basenames renamed because of content mismatch
	Removed  string   // source dir, removed on success (empty in dry-run)
}

// Migrate moves the contents of srcDir into dstDir using copy-then-verify-then-delete.
// If a file with the same name exists in dstDir:
//   - identical bytes (sha256) -> skip
//   - different bytes          -> store as "<base>.conflict-<host>-<RFC3339>"
//
// On success the (now empty) srcDir is removed. dryRun=true reports without touching.
func Migrate(srcDir, dstDir, host string, dryRun bool) (MigrateReport, error) {
	rep := MigrateReport{}
	if !dryRun {
		if err := os.MkdirAll(dstDir, 0o755); err != nil {
			return rep, err
		}
	}
	ents, err := os.ReadDir(srcDir)
	if err != nil {
		return rep, err
	}
	for _, e := range ents {
		if e.IsDir() {
			// Recurse: rare for Claude session dirs but supported.
			sub, err := Migrate(filepath.Join(srcDir, e.Name()), filepath.Join(dstDir, e.Name()), host, dryRun)
			if err != nil {
				return rep, err
			}
			rep.Copied = append(rep.Copied, sub.Copied...)
			rep.Skipped = append(rep.Skipped, sub.Skipped...)
			rep.Conflict = append(rep.Conflict, sub.Conflict...)
			continue
		}
		src := filepath.Join(srcDir, e.Name())
		dst := filepath.Join(dstDir, e.Name())

		if _, err := os.Stat(dst); err == nil {
			same, err := sameFile(src, dst)
			if err != nil {
				return rep, err
			}
			if same {
				rep.Skipped = append(rep.Skipped, e.Name())
				if !dryRun {
					if err := os.Remove(src); err != nil {
						return rep, err
					}
				}
				continue
			}
			// Conflict: keep dst, rename src into dst dir with suffix.
			suffix := fmt.Sprintf(".conflict-%s-%s", host, time.Now().UTC().Format("20060102T150405Z"))
			cdst := dst + suffix
			rep.Conflict = append(rep.Conflict, filepath.Base(cdst))
			if !dryRun {
				if err := copyFile(src, cdst); err != nil {
					return rep, err
				}
				if err := os.Remove(src); err != nil {
					return rep, err
				}
			}
			continue
		}
		// New file at dst.
		rep.Copied = append(rep.Copied, e.Name())
		if !dryRun {
			if err := copyFile(src, dst); err != nil {
				return rep, err
			}
			if err := os.Remove(src); err != nil {
				return rep, err
			}
		}
	}
	if !dryRun {
		// Try to remove srcDir if empty.
		if rem, _ := os.ReadDir(srcDir); len(rem) == 0 {
			if err := os.Remove(srcDir); err == nil {
				rep.Removed = srcDir
			}
		}
	}
	return rep, nil
}

// copyFile copies preserving mode; uses temp + rename so dst is never half-written.
func copyFile(src, dst string) error {
	si, err := os.Stat(src)
	if err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".cs-cp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // best-effort if rename succeeds Remove fails
	if _, err := io.Copy(tmp, in); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, si.Mode()); err != nil {
		return err
	}
	return os.Rename(tmpName, dst)
}

func sameFile(a, b string) (bool, error) {
	sa, err := sha256file(a)
	if err != nil {
		return false, err
	}
	sb, err := sha256file(b)
	if err != nil {
		return false, err
	}
	return sa == sb, nil
}

func sha256file(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
