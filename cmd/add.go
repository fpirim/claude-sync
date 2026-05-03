package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/fpirim/claude-sync/internal/config"
	"github.com/spf13/cobra"
)

var addCmd = &cobra.Command{
	Use:   "add <name> <path>",
	Short: "Register a project with this machine's path",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		name, p := args[0], args[1]
		abs, err := filepath.Abs(p)
		if err != nil {
			return err
		}
		ctx, err := loadAll()
		if err != nil {
			return err
		}
		lock, err := config.AcquireLock(ctx.P.Lock)
		if err != nil {
			return err
		}
		defer lock.Release()

		// reload inside lock to avoid lost-update
		cfg, err := config.Load(ctx.P.Config)
		if err != nil {
			return err
		}
		proj, ok := cfg.Projects[name]
		if !ok {
			proj = config.Project{Paths: map[string]string{}}
		}
		if proj.Paths == nil {
			proj.Paths = map[string]string{}
		}
		old := proj.Paths[ctx.Machine]
		proj.Paths[ctx.Machine] = abs
		cfg.Projects[name] = proj

		if err := config.Save(ctx.P.Config, cfg); err != nil {
			return err
		}
		if old == "" {
			fmt.Printf("added %s on %s -> %s\n", name, ctx.Machine, abs)
		} else if old != abs {
			fmt.Printf("updated %s on %s: %s -> %s\n", name, ctx.Machine, old, abs)
		} else {
			fmt.Printf("no change: %s on %s already %s\n", name, ctx.Machine, abs)
		}
		fmt.Println("run `claude-sync repair` to apply")
		return nil
	},
}

func init() { rootCmd.AddCommand(addCmd) }
