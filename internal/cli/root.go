// Package cli provides the jitr command-line interface.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	jsonMode bool
	rootCmd  = newRootCommand()
)

type rootCommand struct {
	*cobra.Command
	args []string
}

func (r *rootCommand) SetArgs(args []string) {
	r.args = append([]string(nil), args...)
	r.Command.SetArgs(args)
}

func newRootCommand() *rootCommand {
	cmd := &cobra.Command{
		Use:           "jitr",
		Short:         "即时规则",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	cmd.CompletionOptions.DisableDefaultCmd = true
	cmd.PersistentFlags().BoolVar(&jsonMode, "json", false, "以 JSON 输出")
	cmd.SetHelpFunc(renderHelp)
	return &rootCommand{Command: cmd}
}

// Register adds a command to the root command.
func Register(cmd *cobra.Command) {
	rootCmd.AddCommand(cmd)
}

// Execute runs the root command and returns the process exit code.
func Execute() int {
	defer resetJSONFlag()
	prepareTree(rootCmd.Command)
	cmd, err := rootCmd.ExecuteC()
	if err == nil {
		return 0
	}
	if code, silent := silentCode(err); silent {
		return code
	}
	code := errorCode(err)
	if cmd == nil {
		cmd = rootCmd.Command
	}
	if jsonMode || argsRequestJSON(executionArgs()) {
		jsonMode = true
		Output(cmd, false, nil, "", []error{err}, nil)
	} else {
		fmt.Fprintln(cmd.ErrOrStderr(), err)
	}
	return code
}

func executionArgs() []string {
	if rootCmd.args != nil {
		return rootCmd.args
	}
	return os.Args[1:]
}

func resetJSONFlag() {
	jsonMode = false
	flag := rootCmd.PersistentFlags().Lookup("json")
	_ = flag.Value.Set("false")
	flag.Changed = false
}

func prepareTree(cmd *cobra.Command) {
	if cmd.HasSubCommands() && cmd.Run == nil && cmd.RunE == nil {
		cmd.Args = cobra.NoArgs
		cmd.RunE = func(command *cobra.Command, _ []string) error {
			return command.Help()
		}
	}
	for _, child := range cmd.Commands() {
		child.SetHelpFunc(renderHelp)
		prepareTree(child)
	}
}

func renderHelp(cmd *cobra.Command, _ []string) {
	help := cmd.UsageString()
	if jsonMode {
		Output(cmd, true, map[string]string{"help": help}, "", nil, nil)
		return
	}
	fmt.Fprint(cmd.OutOrStdout(), help)
}

func argsRequestJSON(args []string) bool {
	for _, arg := range args {
		if arg == "--json" || arg == "--json=true" {
			return true
		}
	}
	return false
}
