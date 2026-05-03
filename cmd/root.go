package cmd

import (
	"fmt"
	"os"

	"github.com/fpirim/claude-sync/internal/config"
	"github.com/fpirim/claude-sync/internal/machine"
	"github.com/fpirim/claude-sync/internal/paths"
	"github.com/spf13/cobra"
)

var Version = "0.1.0-dev"

// rootCmd is the entry point.
var rootCmd = &cobra.Command{
	Use:   "claude-sync",
	Short: "Manage and sync Claude Code history across devices",
	Long:  "claude-sync wires Claude Code's per-machine project dirs into a shared, syncable layout under ~/.claude/projects/_shared and orchestrates Syncthing.",
	Version: Version,
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// loadCtx is shared boilerplate: paths + machine + config.
type loadCtx struct {
	P       paths.Paths
	Machine string
	Cfg     config.Config
}

func loadAll() (loadCtx, error) {
	p, err := paths.Default()
	if err != nil {
		return loadCtx{}, err
	}
	m, err := machine.Resolve(p.Machine)
	if err != nil {
		return loadCtx{}, err
	}
	cfg, err := config.Load(p.Config)
	if err != nil {
		return loadCtx{}, err
	}
	return loadCtx{P: p, Machine: m, Cfg: cfg}, nil
}
