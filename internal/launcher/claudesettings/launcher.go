package claudesettings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"

	"github.com/chilogen/jit-rules/internal/harness"
	"github.com/chilogen/jit-rules/internal/launcher"
)

type implementation struct{}

func init() {
	launcher.Register(implementation{})
}

func (implementation) Kind() string { return "claude-settings" }

func (implementation) Prepare(req launcher.Request) (launcher.Launch, error) {
	settings := make(map[string]any, len(req.Def.Isolate.Settings)+1)
	for key, value := range req.Def.Isolate.Settings {
		settings[key] = value
	}
	if !req.NoJitr {
		settings["hooks"] = harness.HookEntries(req.Def, hookCommand(req))
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return launcher.Launch{}, err
	}
	path := filepath.Join(req.TempDir, "settings.json")
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return launcher.Launch{}, err
	}
	return launcher.Launch{
		Program: req.Def.Isolate.Program,
		Args: launcher.MergeArgs(
			req.Def, req.UserArgs, []string{"--settings", path},
		),
	}, nil
}

func hookCommand(req launcher.Request) string {
	goos := req.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	return harness.CommandString(req.Def, req.Bin, goos)
}
