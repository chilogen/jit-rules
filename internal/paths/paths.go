// Package paths centralizes all filesystem locations used by jitr.
package paths

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var envPlaceholder = regexp.MustCompile(`\{env:([A-Za-z_][A-Za-z0-9_]*):([^}]*)\}`)

// Config returns the configuration directory.
func Config() string { return location("config", platformConfig()) }

// State returns the state directory.
func State() string { return location("state", platformState()) }

// Data returns the data directory.
func Data() string { return location("data", platformData()) }

// Cache returns the cache directory.
func Cache() string { return location("cache", platformCache()) }

// Bin returns the stable executable path.
func Bin() string {
	if home := os.Getenv("JITR_HOME"); home != "" {
		return filepath.Join(home, "bin", executableName())
	}
	return platformBin()
}

// Home returns the current user's home directory.
func Home() string {
	home, _ := os.UserHomeDir()
	return home
}

// Expand expands path placeholders and anchors relative paths to root.
func Expand(s, root string) string {
	s = strings.ReplaceAll(s, "{config}", Config())
	s = envPlaceholder.ReplaceAllStringFunc(s, func(match string) string {
		parts := envPlaceholder.FindStringSubmatch(match)
		if value := os.Getenv(parts[1]); value != "" {
			return value
		}
		return parts[2]
	})
	if s == "~" {
		s = Home()
	} else if strings.HasPrefix(s, "~/") || strings.HasPrefix(s, `~\`) {
		s = filepath.Join(Home(), s[2:])
	}
	if root != "" && !filepath.IsAbs(s) {
		s = filepath.Join(root, s)
	}
	return filepath.Clean(s)
}

// ProjectRoot finds the closest ancestor containing a .git entry.
func ProjectRoot(cwd string) string {
	if _, err := os.Stat(cwd); err != nil {
		return cwd
	}
	current := filepath.Clean(cwd)
	for {
		if _, err := os.Stat(filepath.Join(current, ".git")); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return cwd
		}
		current = parent
	}
}

func location(name, fallback string) string {
	if home := os.Getenv("JITR_HOME"); home != "" {
		return filepath.Join(home, name)
	}
	return fallback
}
