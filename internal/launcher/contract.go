// Package launcher prepares isolated harness processes.
package launcher

import (
	"sync"

	"github.com/chilogen/jit-rules/internal/harness"
)

// Request describes an isolated launch.
type Request struct {
	Def      *harness.Definition
	Bin      string
	UserArgs []string
	NoJitr   bool
	TempDir  string
	GOOS     string
}

// Launch is a fully prepared child process.
type Launch struct {
	Program string
	Args    []string
	Env     map[string]string
}

// Launcher prepares one data-defined isolation kind.
type Launcher interface {
	Kind() string
	Prepare(req Request) (Launch, error)
}

var launchers = struct {
	sync.RWMutex
	values map[string]Launcher
}{values: make(map[string]Launcher)}

// Register makes a launcher available by kind.
func Register(l Launcher) {
	launchers.Lock()
	defer launchers.Unlock()
	launchers.values[l.Kind()] = l
}

// Get looks up a registered launcher by kind.
func Get(kind string) (Launcher, bool) {
	launchers.RLock()
	defer launchers.RUnlock()
	value, ok := launchers.values[kind]
	return value, ok
}

// MergeArgs inserts generated arguments at the definition-selected position.
func MergeArgs(def *harness.Definition, userArgs, generated []string) []string {
	isolate := def.Isolate
	result := make([]string, 0, len(userArgs)+len(isolate.Args)+len(generated))
	if len(userArgs) > 0 && contains(isolate.Subcommands, userArgs[0]) {
		subcommand := userArgs[0]
		result = append(result, subcommand)
		result = append(result, isolate.Args...)
		result = append(result, isolate.SubcommandArgs[subcommand]...)
		result = append(result, generated...)
		return append(result, userArgs[1:]...)
	}
	result = append(result, isolate.Args...)
	result = append(result, generated...)
	return append(result, userArgs...)
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
