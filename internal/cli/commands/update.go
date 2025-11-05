package commands

import (
	"fmt"

	"github.com/branchd-dev/branchd/internal/cli/update"
	"github.com/spf13/cobra"
)

// NewUpdateCmd creates the update command
func NewUpdateCmd(version string) *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Update branchd CLI",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdate(version)
		},
	}
}

func runUpdate(currentVersion string) error {
	if err := update.SelfUpdate(currentVersion); err != nil {
		return fmt.Errorf("update failed: %w", err)
	}

	return nil
}
