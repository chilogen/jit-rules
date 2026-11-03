package judge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/chilogen/jit-rules/internal/event"
	"github.com/chilogen/jit-rules/internal/fileutil"
	"github.com/chilogen/jit-rules/internal/paths"
	"github.com/chilogen/jit-rules/internal/unit"
)

func decideScript(ctx context.Context, ev *event.Event, judgedUnit *unit.Unit, stateJSON []byte) Verdict {
	lang := judgedUnit.Judge.Lang
	interpreter, err := exec.LookPath(lang)
	if err != nil {
		return scriptError(true, fmt.Sprintf("%s 不在 PATH 中", lang))
	}
	if err := checkDirectory(ev.Cwd); err != nil {
		return scriptError(false, err.Error())
	}
	key := hash("script", lang, judgedUnit.Judge.Script, ev.Cwd, string(stateJSON))
	if judgedUnit.Judge.Cached() {
		if value, ok := cached(key); ok {
			return Verdict{Hit: value.P == 1, P: value.P, Backend: "script"}
		}
	}
	scriptPath, err := prepareScript(judgedUnit.Judge.Script, lang)
	if err != nil {
		return scriptError(false, err.Error())
	}
	verdict := runScript(ctx, ev, judgedUnit.ID, lang, interpreter, scriptPath)
	if judgedUnit.Judge.Cached() && verdict.Exit != nil && (*verdict.Exit == 0 || *verdict.Exit == 1) {
		storeCached(key, cacheValue{P: verdict.P})
	}
	return verdict
}

func checkDirectory(dir string) error {
	if dir == "" {
		return fmt.Errorf("working directory is empty")
	}
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("working directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("working directory is not a directory")
	}
	return nil
}

func prepareScript(script, lang string) (string, error) {
	path := filepath.Join(paths.Cache(), "scripts", hash(script)+"."+scriptExtension(lang))
	_, err := os.Stat(path)
	if err == nil {
		return path, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("stat script: %w", err)
	}
	if err = fileutil.WriteAtomic(path, []byte(script), 0o600); err != nil {
		return "", fmt.Errorf("write script: %w", err)
	}
	return path, nil
}

func scriptExtension(lang string) string {
	switch lang {
	case "pwsh", "powershell":
		return "ps1"
	default:
		return lang
	}
}

func runScript(
	ctx context.Context,
	ev *event.Event,
	unitID, lang, interpreter, scriptPath string,
) Verdict {
	input, _ := json.Marshal(ev)
	var stderr bytes.Buffer
	command := exec.CommandContext(ctx, interpreter, scriptPath)
	command.Dir = ev.Cwd
	command.Env = append(os.Environ(), "JITR_UNIT="+unitID)
	command.Stdin = bytes.NewReader(input)
	command.Stdout = io.Discard
	command.Stderr = &stderr
	command.WaitDelay = 100 * time.Millisecond
	started := time.Now()
	err := command.Run()
	elapsed := int(time.Since(started).Milliseconds())
	if ctx.Err() != nil {
		return scriptRunError(elapsed, nil, stderr.String(), ctx.Err())
	}
	// A signal kill is not an exit of its own, so Exit stays nil as for a timeout.
	if command.ProcessState == nil || !command.ProcessState.Exited() {
		return scriptRunError(elapsed, nil, stderr.String(), err)
	}
	exit := command.ProcessState.ExitCode()
	return verdictForExit(lang, exit, elapsed, stderr.String(), err)
}

func verdictForExit(lang string, exit, elapsed int, stderr string, runErr error) Verdict {
	verdict := Verdict{
		Backend: "script",
		Ms:      elapsed,
		Exit:    &exit,
		Stderr:  trimStderr(stderr),
	}
	switch exit {
	case 0:
		verdict.Hit = true
		verdict.P = 1
	case 1:
	case 9009:
		verdict.Unconfigured = true
		verdict.Err = fmt.Sprintf("%s 无法运行", lang)
	default:
		verdict.Hit = true
		verdict.Fallback = true
		if runErr != nil {
			verdict.Err = runErr.Error()
		} else {
			verdict.Err = fmt.Sprintf("exit status %d", exit)
		}
	}
	return verdict
}

func scriptRunError(elapsed int, exit *int, stderr string, err error) Verdict {
	verdict := scriptError(false, err.Error())
	verdict.Ms = elapsed
	verdict.Exit = exit
	verdict.Stderr = trimStderr(stderr)
	return verdict
}

func scriptError(unconfigured bool, reason string) Verdict {
	return Verdict{
		Hit:          !unconfigured,
		Fallback:     !unconfigured,
		Unconfigured: unconfigured,
		Backend:      "script",
		Err:          reason,
	}
}

func trimStderr(stderr string) string {
	value := strings.TrimSpace(stderr)
	if utf8.RuneCountInString(value) <= 500 {
		return value
	}
	return string([]rune(value)[:500])
}
