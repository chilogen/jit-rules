package dshpatch

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/chilogen/jit-rules/internal/harness"
	"github.com/chilogen/jit-rules/internal/launcher"
	"github.com/chilogen/jit-rules/internal/paths"
	"gopkg.in/yaml.v3"
)

type implementation struct{}

func init() {
	launcher.Register(implementation{})
}

func (implementation) Kind() string { return "dsh-patch" }

func (implementation) Prepare(req launcher.Request) (launcher.Launch, error) {
	hooksPath := filepath.Join(req.TempDir, "hooks.json")
	if !req.NoJitr {
		if err := writeHooks(req, hooksPath); err != nil {
			return launcher.Launch{}, err
		}
	}
	entries, err := isolationEntries(req, hooksPath)
	if err != nil {
		return launcher.Launch{}, err
	}
	data, err := yaml.Marshal(entries)
	if err != nil {
		return launcher.Launch{}, err
	}
	patchPath := filepath.Join(req.TempDir, "isolate.patch.yml")
	if err := os.WriteFile(patchPath, data, 0o600); err != nil {
		return launcher.Launch{}, err
	}
	return launcher.Launch{
		Program: req.Def.Isolate.Program,
		Args: launcher.MergeArgs(
			req.Def, req.UserArgs, []string{"--patch", patchPath},
		),
	}, nil
}

func writeHooks(req launcher.Request, path string) error {
	goos := req.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	command := harness.CommandString(req.Def, req.Bin, goos)
	root := map[string]any{"hooks": harness.HookEntries(req.Def, command)}
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

func isolationEntries(req launcher.Request, hooksPath string) ([]any, error) {
	ids := optionStrings(req.Def.Isolate.Options["disable_entries"])
	prefixes := optionStrings(req.Def.Isolate.Options["disable_entry_prefixes"])
	discovered, err := discoverEntries(dshHome(), prefixes)
	if err != nil {
		return nil, err
	}
	ids = appendUnique(ids, discovered...)
	entries := make([]any, 0, len(ids)+1)
	for _, id := range ids {
		entries = append(entries, map[string]any{"id": id, "disabled": true})
	}
	if !req.NoJitr {
		entries = append(entries, map[string]any{"insert": []any{map[string]any{
			"id":   req.Def.Install.Options["entry_id"],
			"name": req.Def.Install.Options["plugin"],
			"config": map[string]any{
				"configPath": hooksPath,
			},
		}}})
	}
	return entries, nil
}

func dshHome() string {
	return paths.Expand("{env:DSH_HOME:~/.dsh}", "")
}

func discoverEntries(home string, prefixes []string) ([]string, error) {
	patches := []string{filepath.Join(home, "cordis.patch.yml")}
	profiles, err := filepath.Glob(filepath.Join(home, "profiles", "*", "cordis.patch.yml"))
	if err != nil {
		return nil, err
	}
	patches = append(patches, profiles...)
	var found []string
	for _, path := range patches {
		ids, err := readInsertIDs(path)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			if hasPrefix(id, prefixes) {
				found = appendUnique(found, id)
			}
		}
	}
	return found, nil
}

func readInsertIDs(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, err
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.SequenceNode {
		return nil, nil
	}
	var ids []string
	for _, item := range document.Content[0].Content {
		insert := mappingValue(item, "insert")
		if insert == nil || insert.Kind != yaml.SequenceNode {
			continue
		}
		for _, entry := range insert.Content {
			if id := mappingValue(entry, "id"); id != nil {
				ids = append(ids, id.Value)
			}
		}
	}
	return ids, nil
}

func mappingValue(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			return mapping.Content[index+1]
		}
	}
	return nil
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

func appendUnique(values []string, additions ...string) []string {
	seen := make(map[string]bool, len(values)+len(additions))
	for _, value := range values {
		seen[value] = true
	}
	for _, value := range additions {
		if !seen[value] {
			values = append(values, value)
			seen[value] = true
		}
	}
	return values
}

func hasPrefix(value string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}
