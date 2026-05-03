package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fikret/claude-sync/internal/fsops"
	"github.com/spf13/cobra"
)

type check struct {
	Name string
	OK   bool
	Note string
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Health check: filesystem, .stignore, sensitive files",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, err := loadAll()
		if err != nil {
			return err
		}
		var checks []check

		// .stignore present and matches canonical
		if got, err := os.ReadFile(ctx.P.StIgnore); err != nil {
			checks = append(checks, check{".stignore exists", false, err.Error()})
		} else {
			ok := string(got) == fsops.StIgnoreContent
			note := "matches canonical"
			if !ok {
				note = "drifted from canonical (run `claude-sync repair` to refresh)"
			}
			checks = append(checks, check{".stignore canonical", ok, note})
		}

		// .credentials.json must be ignored
		credPath := filepath.Join(ctx.P.Home, ".credentials.json")
		credExists := false
		if _, err := os.Stat(credPath); err == nil {
			credExists = true
		}
		got, _ := os.ReadFile(ctx.P.StIgnore)
		ignoredCred := strings.Contains(string(got), ".credentials.json")
		checks = append(checks, check{
			".credentials.json ignored",
			ignoredCred || !credExists,
			fmt.Sprintf("exists=%v ignored=%v", credExists, ignoredCred),
		})

		// _shared exists
		if _, err := os.Stat(ctx.P.Shared); err != nil {
			checks = append(checks, check{"_shared dir", false, err.Error()})
		} else {
			checks = append(checks, check{"_shared dir", true, ctx.P.Shared})
		}

		// machine name resolved
		checks = append(checks, check{"machine name", ctx.Machine != "", ctx.Machine})

		// each project on this host has a working symlink
		scan, _ := fsops.ScanProjects(ctx.P.Projects, ctx.P.Shared)
		for name, proj := range ctx.Cfg.Projects {
			mp, ok := proj.Paths[ctx.Machine]
			if !ok || mp == "" {
				continue
			}
			// recompute encoded — quick & dirty
			enc := strings.NewReplacer("/", "-", ".", "-").Replace(mp)
			info, present := scan.EncodedDirs[enc]
			ok = present && info.IsSymlink && strings.HasPrefix(info.LinkTarget, ctx.P.Shared)
			note := info.Path
			if !present {
				note = "no encoded dir at " + enc
			} else if !info.IsSymlink {
				note = "not a symlink: " + info.Path
			} else if !strings.HasPrefix(info.LinkTarget, ctx.P.Shared) {
				note = "link points outside _shared: " + info.LinkTarget
			}
			checks = append(checks, check{"project link: " + name, ok, note})
		}

		fail := 0
		for _, c := range checks {
			mark := "OK"
			if !c.OK {
				mark = "FAIL"
				fail++
			}
			fmt.Printf("[%-4s] %-32s %s\n", mark, c.Name, c.Note)
		}
		if fail > 0 {
			return fmt.Errorf("%d check(s) failed", fail)
		}
		return nil
	},
}

func init() { rootCmd.AddCommand(doctorCmd) }
