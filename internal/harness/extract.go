package harness

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/chilogen/jit-rules/internal/event"
)

func extract(def *Definition, rawTool string, input map[string]any, cwd, root string, exists func(string) bool) Extracted {
	tool := event.CanonicalTool(rawTool, def.Tools)
	if contains(def.PatchTools, rawTool) {
		return extractPatch(tool, input, cwd, root)
	}
	if tool == "shell" {
		command, _ := input["command"].(string)
		got := analyzeShell(command, cwd, root, exists)
		got.Tool = tool
		got.Shell = command
		return got
	}
	return Extracted{
		Tool:    tool,
		Paths:   normalizePaths(extractPaths(input), cwd, root),
		Content: extractContent(tool, input),
	}
}

func extractPatch(tool string, input map[string]any, cwd, root string) Extracted {
	patch := firstNonemptyString(input, "command", "input")
	var paths []string
	var content []string
	allAdds := true
	hasFile := false
	for _, line := range strings.Split(patch, "\n") {
		if path, kind, ok := patchPath(line); ok {
			paths = append(paths, path)
			hasFile = true
			if kind != "add" {
				allAdds = false
			}
		}
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			content = append(content, strings.TrimPrefix(line, "+"))
		}
	}
	if hasFile && allAdds {
		tool = "write"
	}
	return Extracted{
		Tool:    tool,
		Paths:   normalizePaths(unique(paths), cwd, root),
		Content: strings.Join(content, "\n"),
	}
}

func patchPath(line string) (string, string, bool) {
	prefixes := []struct {
		text string
		kind string
	}{
		{"*** Add File: ", "add"},
		{"*** Update File: ", "update"},
		{"*** Delete File: ", "delete"},
		{"*** Move to: ", "move"},
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(line, prefix.text) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix.text)), prefix.kind, true
		}
	}
	return "", "", false
}

func extractPaths(input map[string]any) []string {
	for _, key := range []string{"file_path", "path", "notebook_path", "filePath", "target_file"} {
		if value, ok := input[key].(string); ok && value != "" {
			return []string{value}
		}
	}
	keys := make([]string, 0)
	for key := range input {
		if strings.HasSuffix(key, "_path") {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	var result []string
	for _, key := range keys {
		if value, ok := input[key].(string); ok {
			result = append(result, value)
		}
	}
	return result
}

func extractContent(tool string, input map[string]any) string {
	if tool != "write" && tool != "edit" && !strings.HasPrefix(tool, "mcp/") {
		return ""
	}
	if edits, ok := input["edits"].([]any); ok {
		values := make([]string, 0, len(edits))
		for _, item := range edits {
			edit, _ := item.(map[string]any)
			if value, ok := edit["new_string"].(string); ok {
				values = append(values, value)
			}
		}
		return strings.Join(values, "\n")
	}
	for _, key := range []string{"content", "file_text", "new_string", "new_str", "new_source"} {
		if value, ok := input[key].(string); ok {
			return value
		}
	}
	return ""
}

func normalizePaths(values []string, cwd, root string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, normalizePath(value, cwd, root))
	}
	return unique(result)
}

func normalizePath(value, cwd, root string) string {
	absolute := value
	if !filepath.IsAbs(absolute) {
		absolute = filepath.Join(cwd, absolute)
	}
	absolute = filepath.Clean(absolute)
	if relative, err := filepath.Rel(root, absolute); err == nil && insideRoot(relative) {
		return filepath.ToSlash(relative)
	}
	return absolute
}

func insideRoot(relative string) bool {
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func unique(values []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func firstNonemptyString(input map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := input[key].(string); ok && value != "" {
			return value
		}
	}
	return ""
}
