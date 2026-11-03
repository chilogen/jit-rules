package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"time"

	"github.com/chilogen/jit-rules/internal/harness"
	"github.com/chilogen/jit-rules/internal/installer"
	"github.com/chilogen/jit-rules/internal/journal"
	"github.com/chilogen/jit-rules/internal/paths"
	"github.com/chilogen/jit-rules/internal/profile"
	"github.com/spf13/cobra"
)

type harnessStatus struct {
	Name      string   `json:"name"`
	Title     string   `json:"title"`
	User      bool     `json:"user"`
	Project   bool     `json:"project"`
	Recent    bool     `json:"recent"`
	Notes     []string `json:"notes"`
	CheckErrs []string `json:"errors"`
}

type statusResult struct {
	Harnesses    []harnessStatus     `json:"harnesses"`
	Profiles     []string            `json:"profiles"`
	Units        int                 `json:"units"`
	Errors       []string            `json:"errors"`
	Backends     []backendStatus     `json:"backends"`
	Interpreters []interpreterStatus `json:"interpreters"`
}

type backendStatus struct {
	Name      string `json:"name"`
	APIKeyEnv string `json:"api_key_env"`
	APIKeySet bool   `json:"api_key_set"`
}

type interpreterStatus struct {
	Name  string `json:"name"`
	Found bool   `json:"found"`
}

func newStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "显示运行状态",
		Args:  cobra.NoArgs,
		RunE:  runStatus,
	}
}

func runStatus(cmd *cobra.Command, _ []string) error {
	resolved, _, err := resolveUnits("")
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	records, err := journal.Since(now.Add(-24 * time.Hour))
	if err != nil {
		return fmt.Errorf("读取日志: %w", err)
	}
	harnesses, err := collectHarnessStatus(records)
	if err != nil {
		return err
	}
	internalErrors, err := recentInternalErrors(now.Add(-24 * time.Hour))
	if err != nil {
		return fmt.Errorf("读取内部错误: %w", err)
	}
	data := statusResult{
		Harnesses: harnesses, Profiles: resolved.Profiles,
		Units: len(resolved.Units), Errors: internalErrors,
		Backends:     collectBackendStatus(resolved),
		Interpreters: collectInterpreterStatus(resolved),
	}
	Output(cmd, true, data, formatStatus(data, resolved), resolved.Errors, nil)
	return nil
}

func collectHarnessStatus(records []journal.Record) ([]harnessStatus, error) {
	definitions, err := harness.LoadAll()
	if err != nil {
		return nil, fmt.Errorf("读取 harness 定义: %w", err)
	}
	recent := map[string]bool{}
	for _, record := range records {
		recent[record.Event.Harness] = true
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("读取当前目录: %w", err)
	}
	root := paths.ProjectRoot(cwd)
	result := make([]harnessStatus, 0, len(definitions))
	for _, definition := range definitions {
		item := harnessStatus{
			Name: definition.Name, Title: definition.Title,
			Recent: recent[definition.Name],
		}
		implementation, ok := installer.Get(definition.Install.Kind)
		if !ok {
			item.CheckErrs = append(item.CheckErrs, "安装器未注册")
			result = append(result, item)
			continue
		}
		item.User, item.CheckErrs = installedScope(
			implementation, definition, "user", "", item.CheckErrs,
		)
		if _, ok := definition.Install.Targets["project"]; ok {
			item.Project, item.CheckErrs = installedScope(
				implementation, definition, "project", root, item.CheckErrs,
			)
		}
		if (item.User || item.Project) && !item.Recent {
			item.Notes = append(item.Notes, definition.Install.Notes...)
		}
		result = append(result, item)
	}
	return result, nil
}

func installedScope(
	implementation installer.Installer,
	definition *harness.Definition,
	scope, root string,
	errs []string,
) (bool, []string) {
	installed, err := implementation.Installed(installer.Request{
		Def: definition, Scope: scope, Root: root, Bin: paths.Bin(), GOOS: runtime.GOOS,
	})
	if err != nil {
		errs = append(errs, fmt.Sprintf("%s: %v", scope, err))
	}
	return installed, errs
}

func formatStatus(data statusResult, resolved *profile.Resolved) string {
	var lines []string
	for _, item := range data.Harnesses {
		line := fmt.Sprintf(
			"%s：user=%s，project=%s", item.Title,
			installationWord(item.User), installationWord(item.Project),
		)
		if (item.User || item.Project) && !item.Recent {
			line += "；已安装但最近没有收到事件"
		}
		lines = append(lines, line)
		lines = append(lines, item.Notes...)
		for _, checkErr := range item.CheckErrs {
			lines = append(lines, item.Title+" 检查失败："+checkErr)
		}
	}
	lines = append(lines, profileStatusLines(resolved)...)
	for _, item := range data.Backends {
		keyStatus := "未设置"
		if item.APIKeySet {
			keyStatus = "已设置"
		}
		lines = append(lines, fmt.Sprintf(
			"judge 后端：%s；%s：%s", item.Name, item.APIKeyEnv, keyStatus,
		))
	}
	for _, item := range data.Interpreters {
		status := "未找到"
		if item.Found {
			status = "已找到"
		}
		lines = append(lines, fmt.Sprintf("脚本解释器：%s；%s", item.Name, status))
	}
	lines = append(lines, fmt.Sprintf("最近 24 小时内部错误：%d 条", len(data.Errors)))
	start := max(0, len(data.Errors)-3)
	lines = append(lines, data.Errors[start:]...)
	return joinNonempty(lines)
}

func collectBackendStatus(resolved *profile.Resolved) []backendStatus {
	names := map[string]bool{}
	for _, parsed := range resolved.Units {
		if parsed.Judge == nil || parsed.Judge.Question == "" {
			continue
		}
		name := resolved.Settings.Judge.Backend
		if parsed.Judge.Backend != "" {
			name = parsed.Judge.Backend
		}
		names[name] = true
	}
	sorted := sortedStatusNames(names)
	result := make([]backendStatus, 0, len(sorted))
	for _, name := range sorted {
		envName, _ := resolved.Settings.Backends[name]["api_key_env"].(string)
		result = append(result, backendStatus{
			Name: name, APIKeyEnv: envName,
			APIKeySet: envName != "" && os.Getenv(envName) != "",
		})
	}
	return result
}

func collectInterpreterStatus(resolved *profile.Resolved) []interpreterStatus {
	names := map[string]bool{}
	for _, parsed := range resolved.Units {
		if parsed.Judge != nil && parsed.Judge.Script != "" {
			names[parsed.Judge.Lang] = true
		}
	}
	sorted := sortedStatusNames(names)
	result := make([]interpreterStatus, 0, len(sorted))
	for _, name := range sorted {
		_, err := exec.LookPath(name)
		result = append(result, interpreterStatus{Name: name, Found: err == nil})
	}
	return result
}

func sortedStatusNames(names map[string]bool) []string {
	result := make([]string, 0, len(names))
	for name := range names {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func installationWord(installed bool) string {
	if installed {
		return "已安装"
	}
	return "未安装"
}

func recentInternalErrors(since time.Time) ([]string, error) {
	file, err := os.Open(filepath.Join(paths.State(), "errors.jsonl"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var result []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var item struct {
			TS    time.Time `json:"ts"`
			Stage string    `json:"stage"`
			Err   string    `json:"err"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &item); err != nil {
			return nil, err
		}
		if !item.TS.Before(since) {
			result = append(result, fmt.Sprintf(
				"%s %s %s", item.TS.Format(time.RFC3339), item.Stage, item.Err,
			))
		}
	}
	return result, scanner.Err()
}
