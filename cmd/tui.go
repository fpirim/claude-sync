package cmd

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/fpirim/claude-sync/internal/tui"
	"github.com/spf13/cobra"
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Launch the interactive console UI",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, err := loadAll()
		if err != nil {
			return err
		}
		m := tui.New(ctx.P, ctx.Cfg, ctx.Machine)
		// AltScreen + mouse wheel scroll for viewports.
		p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
		_, err = p.Run()
		return err
	},
}

func init() {
	rootCmd.AddCommand(tuiCmd)
	// Default action: if no subcommand given, run TUI.
	rootCmd.RunE = tuiCmd.RunE
}
