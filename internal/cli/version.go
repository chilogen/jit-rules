package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version is injected by the build.
var Version = "dev"

// Commit is injected by the build.
var Commit = ""

func init() {
	Register(&cobra.Command{
		Use:   "version",
		Short: "显示版本",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			data := map[string]string{"version": Version, "commit": Commit}
			human := fmt.Sprintf("Version: %s\nCommit: %s", Version, Commit)
			Output(cmd, true, data, human, nil, nil)
			return nil
		},
	})
}
