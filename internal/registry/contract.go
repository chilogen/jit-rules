// Package registry stores registered profiles and their scopes.
package registry

// Entry is one registered profile.
type Entry struct {
	Path string `yaml:"path"`
}

// Load reads all registered profiles.
func Load() ([]Entry, error) { return load() }

// Save rewrites the profile registry.
func Save(entries []Entry) error { return save(entries) }

// Scope derives a project profile's scope from its .jitr directory.
func Scope(entry Entry) string { return scope(entry) }

// IsGlobal reports whether an entry is the fixed ~/.jitr profile.
func IsGlobal(entry Entry) bool { return isGlobal(entry) }

// Applicable returns entries ranked for cwd.
func Applicable(entries []Entry, cwd string) []Entry { return applicable(entries, cwd) }
