package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/fpirim/claude-sync/internal/encoder"
	"github.com/fpirim/claude-sync/internal/fsops"
	"github.com/spf13/cobra"
)

var statusJSON bool

type projectStatus struct {
	Name        string `json:"name"`
	OnThisHost  bool   `json:"on_this_host"`
	LocalPath   string `json:"local_path,omitempty"`
	EncodedDir  string `json:"encoded_dir,omitempty"`
	LinkOK      bool   `json:"link_ok"`
	SharedFiles int    `json:"shared_files"`
	HasConflict bool   `json:"has_conflict"`
}

type statusReport struct {
	Machine  string          `json:"machine"`
	Projects []projectStatus `json:"projects"`
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Print per-project sync status",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, err := loadAll()
		if err != nil {
			return err
		}
		scan, err := fsops.ScanProjects(ctx.P.Projects, ctx.P.Shared)
		if err != nil {
			return err
		}
		rep := statusReport{Machine: ctx.Machine}
		names := make([]string, 0, len(ctx.Cfg.Projects))
		for n := range ctx.Cfg.Projects {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, name := range names {
			pr := ctx.Cfg.Projects[name]
			ps := projectStatus{Name: name}
			if mp, ok := pr.Paths[ctx.Machine]; ok && mp != "" {
				ps.OnThisHost = true
				ps.LocalPath = mp
				ps.EncodedDir = encoder.Encode(mp)
				if info, ok := scan.EncodedDirs[ps.EncodedDir]; ok && info.IsSymlink {
					ps.LinkOK = true
				}
			}
			if si, ok := scan.SharedProjects[name]; ok {
				ps.SharedFiles = si.NumFiles
				ps.HasConflict = si.HasConflicts
			}
			rep.Projects = append(rep.Projects, ps)
		}
		if statusJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(rep)
		}
		fmt.Printf("machine: %s\n", rep.Machine)
		fmt.Printf("%-20s %-6s %-6s %-9s %-8s %s\n", "PROJECT", "HERE", "LINK", "SESSIONS", "CONFLICT", "PATH")
		for _, ps := range rep.Projects {
			here := "-"
			if ps.OnThisHost {
				here = "yes"
			}
			link := "-"
			if ps.OnThisHost {
				if ps.LinkOK {
					link = "ok"
				} else {
					link = "MISSING"
				}
			}
			cf := "-"
			if ps.HasConflict {
				cf = "YES"
			}
			fmt.Printf("%-20s %-6s %-6s %-9d %-8s %s\n", ps.Name, here, link, ps.SharedFiles, cf, ps.LocalPath)
		}
		return nil
	},
}

func init() {
	statusCmd.Flags().BoolVar(&statusJSON, "json", false, "emit JSON")
	rootCmd.AddCommand(statusCmd)
}
