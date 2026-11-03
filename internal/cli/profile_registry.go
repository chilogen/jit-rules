package cli

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/chilogen/jit-rules/internal/profile"
	"github.com/chilogen/jit-rules/internal/registry"
)

func absolutePath(value string) (string, error) {
	path, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("解析路径 %s: %w", value, err)
	}
	return filepath.Clean(path), nil
}

func registeredPath(entries []registry.Entry, path string) bool {
	return containsEntry(entries, filepath.Clean(path))
}

func containsEntry(entries []registry.Entry, path string) bool {
	for _, entry := range entries {
		if filepath.Clean(entry.Path) == filepath.Clean(path) {
			return true
		}
	}
	return false
}

func commitWarning(dir, message string) []string {
	if warning := profile.Commit(dir, message); warning != "" {
		return []string{warning}
	}
	return nil
}

func profileRegistration(dir string) (registry.Entry, error) {
	if filepath.Base(filepath.Clean(dir)) != ".jitr" {
		return registry.Entry{}, errors.New("profile 必须位于 .jitr 目录")
	}
	return registry.Entry{Path: dir}, nil
}

func profileScope(entry registry.Entry) string {
	if registry.IsGlobal(entry) {
		return "~"
	}
	return registry.Scope(entry)
}
