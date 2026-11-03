package cli

import (
	"io"
	"time"

	"github.com/chilogen/jit-rules/internal/journal"
	"github.com/chilogen/jit-rules/internal/pipeline"
	"github.com/spf13/cobra"
)

const hookInputLimit = 10 * 1024 * 1024

func init() {
	Register(&cobra.Command{
		Use:                "hook",
		Short:              "处理一次 harness 钩子",
		DisableFlagParsing: true,
		Args:               cobra.ArbitraryArgs,
		RunE:               runHook,
	})
}

func runHook(cmd *cobra.Command, _ []string) error {
	data, err := io.ReadAll(io.LimitReader(cmd.InOrStdin(), hookInputLimit+1))
	if err != nil {
		journal.LogError("read", err)
		return nil
	}
	output := pipeline.Run(data, time.Now().UTC())
	if len(output.Stdout) > 0 {
		_, _ = cmd.OutOrStdout().Write(output.Stdout)
	}
	if len(output.Stderr) > 0 {
		_, _ = cmd.ErrOrStderr().Write(output.Stderr)
	}
	if output.Exit == 2 {
		return Silent(2)
	}
	return nil
}
