package fsops

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fikret/claude-sync/internal/config"
	"github.com/fikret/claude-sync/internal/encoder"
	"github.com/fikret/claude-sync/internal/paths"
)

// Action is a single planned change (or no-op note) from RepairPlan.
type Action struct {
	Kind    string // "ensure-dir" | "migrate" | "symlink" | "fix-symlink" | "stignore" | "orphan" | "noop" | "warn"
	Project string // empty for non-project actions
	Detail  string // human-readable summary
}

// RepairResult bundles planning + execution outcome.
type RepairResult struct {
	Actions  []Action
	Migrated map[string]MigrateReport // project -> report
}

// Repair brings the filesystem in line with config for the given machine.
// dryRun=true reports the plan without writing.
func Repair(p paths.Paths, cfg config.Config, machine string, dryRun bool) (RepairResult, error) {
	res := RepairResult{Migrated: map[string]MigrateReport{}}

	// 1. Ensure base dirs.
	for _, d := range []string{p.Projects, p.Shared, p.Sync} {
		if _, err := os.Stat(d); os.IsNotExist(err) {
			res.Actions = append(res.Actions, Action{Kind: "ensure-dir", Detail: d})
			if !dryRun {
				if err := os.MkdirAll(d, 0o755); err != nil {
					return res, err
				}
			}
		}
	}

	// 2. .stignore
	wantStignore := true
	if got, err := os.ReadFile(p.StIgnore); err == nil && string(got) == StIgnoreContent {
		wantStignore = false
	}
	if wantStignore {
		res.Actions = append(res.Actions, Action{Kind: "stignore", Detail: p.StIgnore})
		if !dryRun {
			if _, err := EnsureStIgnore(p.StIgnore); err != nil {
				return res, err
			}
		}
	}

	// 3. Per-project: migrate & symlink for paths assigned to this machine.
	for name, proj := range cfg.Projects {
		myPath, ok := proj.Paths[machine]
		if !ok || myPath == "" {
			continue
		}
		encoded := encoder.Encode(resolveMaybe(myPath))
		linkPath := p.ProjectLink(encoded)
		target := p.SharedProject(name)

		if _, err := os.Stat(target); os.IsNotExist(err) {
			res.Actions = append(res.Actions, Action{Kind: "ensure-dir", Project: name, Detail: target})
			if !dryRun {
				if err := os.MkdirAll(target, 0o755); err != nil {
					return res, err
				}
			}
		}

		st, err := InspectLink(linkPath, target)
		if err != nil {
			return res, err
		}
		switch st {
		case StateCorrectLink:
			res.Actions = append(res.Actions, Action{Kind: "noop", Project: name, Detail: linkPath + " -> " + target})
		case StateMissing:
			res.Actions = append(res.Actions, Action{Kind: "symlink", Project: name, Detail: linkPath + " -> " + target})
			if !dryRun {
				if _, err := EnsureSymlink(linkPath, target); err != nil {
					return res, err
				}
			}
		case StateWrongLink, StateRealDirEmpty:
			res.Actions = append(res.Actions, Action{Kind: "fix-symlink", Project: name, Detail: linkPath + " -> " + target})
			if !dryRun {
				if _, err := EnsureSymlink(linkPath, target); err != nil {
					return res, err
				}
			}
		case StateRealDirData:
			rep, err := Migrate(linkPath, target, machine, dryRun)
			if err != nil {
				return res, err
			}
			res.Migrated[name] = rep
			res.Actions = append(res.Actions, Action{
				Kind: "migrate", Project: name,
				Detail: fmt.Sprintf("%s -> %s (copied=%d skipped=%d conflict=%d)",
					linkPath, target, len(rep.Copied), len(rep.Skipped), len(rep.Conflict)),
			})
			if !dryRun {
				if _, err := EnsureSymlink(linkPath, target); err != nil {
					return res, err
				}
			}
		case StateOtherFile:
			res.Actions = append(res.Actions, Action{Kind: "warn", Project: name, Detail: linkPath + " is not a directory; skipped"})
		}
	}

	// 4. Orphan symlinks: links under projects/ whose project is no longer assigned to this machine.
	scan, err := ScanProjects(p.Projects, p.Shared)
	if err != nil {
		return res, err
	}
	allowed := map[string]struct{}{}
	for name, proj := range cfg.Projects {
		if mp, ok := proj.Paths[machine]; ok && mp != "" {
			allowed[encoder.Encode(resolveMaybe(mp))] = struct{}{}
			_ = name
		}
	}
	for encName, info := range scan.EncodedDirs {
		if _, ok := allowed[encName]; ok {
			continue
		}
		switch {
		case info.IsSymlink && hasParent(info.LinkTarget, p.Shared):
			res.Actions = append(res.Actions, Action{
				Kind:   "orphan",
				Detail: info.Path + " → " + info.LinkTarget + " (policy: " + cfg.Policies.SymlinkOrphanAction + ")",
			})
			if !dryRun && cfg.Policies.SymlinkOrphanAction == "delete" {
				_ = os.Remove(info.Path)
			}
		case info.IsSymlink:
			// External symlink, not into _shared. Leave it alone but report.
			res.Actions = append(res.Actions, Action{
				Kind:   "warn",
				Detail: info.Path + " is a symlink outside _shared; ignored",
			})
		case info.IsDir && info.Empty:
			// Stray empty directory not bound to any project on this host.
			res.Actions = append(res.Actions, Action{
				Kind:   "orphan",
				Detail: info.Path + " (empty dir; policy: " + cfg.Policies.SymlinkOrphanAction + ")",
			})
			if !dryRun && cfg.Policies.SymlinkOrphanAction == "delete" {
				_ = os.Remove(info.Path)
			}
		case info.IsDir:
			// Real directory with data but no project entry on this host.
			// Only surface it if the underlying source path still exists on
			// disk — otherwise this is dead history of a deleted project,
			// not actionable, just noise.
			if src := encoder.ResolveExisting(encName, nil); src != "" {
				res.Actions = append(res.Actions, Action{
					Kind:   "stranded",
					Detail: info.Path + " ← " + src + " (run `add` to adopt)",
				})
			}
		}
	}

	return res, nil
}

// resolveMaybe applies EvalSymlinks if the path exists, else returns abs as-is.
func resolveMaybe(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	if r, err := filepath.EvalSymlinks(abs); err == nil {
		return r
	}
	return abs
}

func hasParent(child, parent string) bool {
	c, _ := filepath.Abs(child)
	pa, _ := filepath.Abs(parent)
	rel, err := filepath.Rel(pa, c)
	if err != nil {
		return false
	}
	return rel != ".." && len(rel) > 0 && rel[0] != '.'
}
