package paths

import (
	"os"
	"path/filepath"
)

func platformConfig() string {
	return filepath.Join(os.Getenv("APPDATA"), "jit-rules")
}

func platformState() string {
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "jit-rules", "state")
}

func platformData() string {
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "jit-rules", "data")
}

func platformCache() string {
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "jit-rules", "cache")
}

func platformBin() string {
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "jitr", executableName())
}

func executableName() string { return "jitr.exe" }
