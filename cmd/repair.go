package cmd

import (
	"fmt"

	"github.com/fpirim/claude-sync/internal/config"
	"github.com/fpirim/claude-sync/internal/fsops"
	"github.com/spf13/cobra"
)

var repairDryRun bool

var repairCmd = &cobra.Command{
	Use:   "repair",
	Short: "Bring filesystem in line with config (idempotent)",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, err := loadAll()
		if err != nil {
			return err
		}
		lock, err := config.AcquireLock(ctx.P.Lock)
		if err != nil {
			return fmt.Errorf("acquire lock: %w", err)
		}
		defer lock.Release()

		res, err := fsops.Repair(ctx.P, ctx.Cfg, ctx.Machine, repairDryRun)
		if err != nil {
			return err
		}
		printRepair(res, repairDryRun)
		return nil
	},
}

func printRepair(r fsops.RepairResult, dryRun bool) {
	mode := "applied"
	if dryRun {
		mode = "DRY RUN — would apply"
	}
	fmt.Printf("== claude-sync repair (%s) ==\n", mode)
	if len(r.Actions) == 0 {
		fmt.Println("nothing to do")
		return
	}
	for _, a := range r.Actions {
		prefix := a.Kind
		if a.Project != "" {
			prefix = fmt.Sprintf("%s[%s]", a.Kind, a.Project)
		}
		fmt.Printf("  %-18s %s\n", prefix, a.Detail)
	}
}

func init() {
	repairCmd.Flags().BoolVar(&repairDryRun, "dry-run", false, "report planned actions without writing")
	rootCmd.AddCommand(repairCmd)
}
