package codexconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/chilogen/jit-rules/internal/harness"
	"github.com/chilogen/jit-rules/internal/launcher"
	"github.com/chilogen/jit-rules/internal/paths"
)

type implementation struct{}

func init() {
	launcher.Register(implementation{})
}

func (implementation) Kind() string { return "codex-config" }

func (implementation) Prepare(req launcher.Request) (launcher.Launch, error) {
	home, err := prepareHome(req)
	if err != nil {
		return launcher.Launch{}, err
	}
	generated, err := hookArguments(req)
	if err != nil {
		return launcher.Launch{}, err
	}
	env := map[string]string{
		optionString(req.Def.Isolate.Options["home_env"]): home,
	}
	return launcher.Launch{
		Program: req.Def.Isolate.Program,
		Args:    launcher.MergeArgs(req.Def, req.UserArgs, generated),
		Env:     env,
	}, nil
}

func prepareHome(req launcher.Request) (string, error) {
	destination := filepath.Join(req.TempDir, "home")
	if err := os.Mkdir(destination, 0o700); err != nil {
		return "", fmt.Errorf("create isolated home: %w", err)
	}
	source := paths.Expand(optionString(req.Def.Isolate.Options["home"]), "")
	for _, name := range optionStrings(req.Def.Isolate.Options["home_keep"]) {
		if err := copyRegularFile(filepath.Join(source, name), filepath.Join(destination, name)); err != nil {
			return "", err
		}
	}
	return destination, nil
}

func copyRegularFile(source, destination string) error {
	info, err := os.Stat(source)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect %s: %w", source, err)
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return fmt.Errorf("read %s: %w", source, err)
	}
	if err := os.WriteFile(destination, data, info.Mode().Perm()); err != nil {
		return fmt.Errorf("copy %s: %w", source, err)
	}
	if err := os.Chmod(destination, info.Mode().Perm()); err != nil {
		return fmt.Errorf("preserve permissions for %s: %w", source, err)
	}
	return nil
}

func hookArguments(req launcher.Request) ([]string, error) {
	if req.NoJitr {
		return nil, nil
	}
	goos := req.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	command := harness.CommandString(req.Def, req.Bin, goos)
	entries := harness.HookEntries(req.Def, command)
	events := make([]string, 0, len(entries))
	for event := range entries {
		events = append(events, event)
	}
	sort.Strings(events)
	result := make([]string, 0, len(events)*2)
	for _, event := range events {
		value, err := tomlValue(entries[event])
		if err != nil {
			return nil, err
		}
		result = append(result, "-c", "hooks."+event+"="+value)
	}
	return result, nil
}

func tomlValue(value any) (string, error) {
	switch value := value.(type) {
	case string:
		return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value) + `"`, nil
	case bool:
		return strconv.FormatBool(value), nil
	case int:
		return strconv.Itoa(value), nil
	case int64:
		return strconv.FormatInt(value, 10), nil
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64), nil
	case []any:
		return tomlArray(value)
	case map[string]any:
		return tomlTable(value)
	default:
		return "", fmt.Errorf("unsupported TOML hook value %T", value)
	}
}

func tomlArray(values []any) (string, error) {
	encoded := make([]string, 0, len(values))
	for _, value := range values {
		item, err := tomlValue(value)
		if err != nil {
			return "", err
		}
		encoded = append(encoded, item)
	}
	return "[" + strings.Join(encoded, ",") + "]", nil
}

func tomlTable(values map[string]any) (string, error) {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return keyOrder(keys[i]) < keyOrder(keys[j])
	})
	encoded := make([]string, 0, len(keys))
	for _, key := range keys {
		value, err := tomlValue(values[key])
		if err != nil {
			return "", err
		}
		encoded = append(encoded, key+"="+value)
	}
	return "{" + strings.Join(encoded, ",") + "}", nil
}

func keyOrder(key string) string {
	order := map[string]string{
		"matcher": "0", "hooks": "1", "type": "2", "command": "3", "timeout": "4",
	}
	if prefix, ok := order[key]; ok {
		return prefix
	}
	return "5" + key
}

func optionString(value any) string {
	text, _ := value.(string)
	return text
}

func optionStrings(value any) []string {
	if strings, ok := value.([]string); ok {
		return append([]string(nil), strings...)
	}
	values, _ := value.([]any)
	result := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok {
			result = append(result, text)
		}
	}
	return result
}
