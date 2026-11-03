//go:build !windows

package paths

import (
	"os"
	"path/filepath"
)

func platformConfig() string {
	return xdgLocation("XDG_CONFIG_HOME", filepath.Join(Home(), ".config"))
}

func platformState() string {
	return xdgLocation("XDG_STATE_HOME", filepath.Join(Home(), ".local", "state"))
}

func platformData() string {
	return xdgLocation("XDG_DATA_HOME", filepath.Join(Home(), ".local", "share"))
}

func platformCache() string {
	return xdgLocation("XDG_CACHE_HOME", filepath.Join(Home(), ".cache"))
}

func platformBin() string {
	return filepath.Join(Home(), ".local", "bin", executableName())
}

func executableName() string { return "jitr" }

func xdgLocation(environment, fallback string) string {
	if value := os.Getenv(environment); value != "" {
		return filepath.Join(value, "jit-rules")
	}
	return filepath.Join(fallback, "jit-rules")
}
