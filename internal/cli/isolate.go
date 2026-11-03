package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"

	"github.com/chilogen/jit-rules/internal/harness"
	"github.com/chilogen/jit-rules/internal/launcher"
	_ "github.com/chilogen/jit-rules/internal/launcher/claudesettings"
	_ "github.com/chilogen/jit-rules/internal/launcher/codexconfig"
	_ "github.com/chilogen/jit-rules/internal/launcher/dshpatch"
	_ "github.com/chilogen/jit-rules/internal/launcher/manual"
	"github.com/chilogen/jit-rules/internal/paths"
	"github.com/spf13/cobra"
)

func init() {
	Register(newIsolateCommand())
}

func newIsolateCommand() *cobra.Command {
	var noJitr bool
	command := &cobra.Command{
		Use:   "isolate <harness> [--no-jitr] -- <harness 参数>...",
		Short: "以隔离模式启动 harness",
		Args:  validateIsolateArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if jsonMode {
				return fmt.Errorf("isolate 不支持 --json")
			}
			code, err := executeIsolate(cmd, args[0], args[1:], noJitr)
			if err != nil {
				return err
			}
			if code != 0 {
				return Silent(code)
			}
			return nil
		},
	}
	command.Flags().BoolVar(&noJitr, "no-jitr", false, "同时禁用 jitr 钩子")
	return command
}

func validateIsolateArgs(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("需要指定 harness")
	}
	dash := cmd.ArgsLenAtDash()
	if dash >= 0 && dash != 1 {
		return fmt.Errorf("harness 参数必须放在 -- 之后")
	}
	if dash < 0 && len(args) != 1 {
		return fmt.Errorf("harness 参数必须放在 -- 之后")
	}
	return nil
}

func executeIsolate(cmd *cobra.Command, name string, userArgs []string, noJitr bool) (int, error) {
	def, err := harness.Find(name)
	if err != nil {
		return 1, err
	}
	return runIsolate(cmd, def, userArgs, noJitr)
}

func runIsolate(
	cmd *cobra.Command,
	def *harness.Definition,
	userArgs []string,
	noJitr bool,
) (code int, err error) {
	tempDir, err := os.MkdirTemp("", "jitr-isolate-*")
	if err != nil {
		return 1, err
	}
	defer func() {
		if removeErr := os.RemoveAll(tempDir); err == nil && removeErr != nil {
			code, err = 1, removeErr
		}
	}()
	strategy, ok := launcher.Get(def.Isolate.Kind)
	if !ok {
		return 1, fmt.Errorf("没有隔离启动器 %q", def.Isolate.Kind)
	}
	launch, err := strategy.Prepare(launcher.Request{
		Def:      def,
		Bin:      paths.Bin(),
		UserArgs: userArgs,
		NoJitr:   noJitr,
		TempDir:  tempDir,
		GOOS:     runtime.GOOS,
	})
	if err != nil {
		return 1, err
	}
	for _, note := range def.Isolate.Notes {
		fmt.Fprintln(cmd.OutOrStdout(), note)
	}
	if launch.Program == "" {
		return 1, nil
	}
	return runLaunch(cmd, launch, noJitr)
}

func runLaunch(cmd *cobra.Command, launch launcher.Launch, noJitr bool) (int, error) {
	process := exec.Command(launch.Program, launch.Args...)
	process.Stdin = cmd.InOrStdin()
	process.Stdout = cmd.OutOrStdout()
	process.Stderr = cmd.ErrOrStderr()
	overrides := make(map[string]string, len(launch.Env)+2)
	for name, value := range launch.Env {
		overrides[name] = value
	}
	overrides["JITR_ISOLATED"] = "1"
	if noJitr {
		overrides["JITR_DISABLE"] = "1"
	}
	process.Env = mergeEnvironment(os.Environ(), overrides)
	err := process.Run()
	if err == nil {
		return 0, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) && exitError.ExitCode() >= 0 {
		return exitError.ExitCode(), nil
	}
	return 1, err
}

func mergeEnvironment(base []string, overrides map[string]string) []string {
	result := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		name, _, found := strings.Cut(entry, "=")
		if !found || !hasEnvironmentKey(overrides, name) {
			result = append(result, entry)
		}
	}
	names := make([]string, 0, len(overrides))
	for name := range overrides {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		result = append(result, name+"="+overrides[name])
	}
	return result
}

func hasEnvironmentKey(values map[string]string, target string) bool {
	for name := range values {
		if name == target || runtime.GOOS == "windows" && strings.EqualFold(name, target) {
			return true
		}
	}
	return false
}
