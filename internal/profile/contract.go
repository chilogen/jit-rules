// Package profile manages profiles, locks, settings, and resolution.
package profile

import (
	"time"

	"github.com/chilogen/jit-rules/assets"
	"github.com/chilogen/jit-rules/internal/unit"
	"gopkg.in/yaml.v3"
)

// PackRef selects and filters one pack.
type PackRef struct {
	Name    string   `yaml:"name"`
	Source  string   `yaml:"source"`
	Ref     string   `yaml:"ref,omitempty"`
	Include []string `yaml:"include,omitempty"`
	Exclude []string `yaml:"exclude,omitempty"`
}

// Profile is the editable profile configuration.
type Profile struct {
	Dir         string         `yaml:"-"`
	Name        string         `yaml:"name"`
	Description string         `yaml:"description,omitempty"`
	Packs       []PackRef      `yaml:"packs"`
	Disable     []string       `yaml:"disable,omitempty"`
	Settings    map[string]any `yaml:"settings,omitempty"`
}

// TestCounts summarizes saved unit test outcomes.
type TestCounts struct {
	Passed  int `yaml:"passed"`
	Failed  int `yaml:"failed"`
	Skipped int `yaml:"skipped"`
}

// LockUnit records an enabled profile-owned unit.
type LockUnit struct {
	Hash      string     `yaml:"hash"`
	EnabledAt time.Time  `yaml:"enabled_at"`
	Tests     TestCounts `yaml:"tests"`
}

// LockPack records immutable pack identity.
type LockPack struct {
	Source  string    `yaml:"source"`
	Ref     string    `yaml:"ref,omitempty"`
	Commit  string    `yaml:"commit,omitempty"`
	Hash    string    `yaml:"hash"`
	AddedAt time.Time `yaml:"added_at"`
}

// Lock records active units and packs.
type Lock struct {
	Version int                 `yaml:"version"`
	Units   map[string]LockUnit `yaml:"units"`
	Packs   map[string]LockPack `yaml:"packs"`
}

// JudgeSettings controls model judgment.
type JudgeSettings struct {
	Backend       string `yaml:"backend"`
	TimeoutMs     int    `yaml:"timeout_ms"`
	MaxFieldChars int    `yaml:"max_field_chars"`
	SendContent   bool   `yaml:"send_content"`
}

// JournalSettings controls journal retention and redaction.
type JournalSettings struct {
	KeepDays     int  `yaml:"keep_days"`
	StoreContent bool `yaml:"store_content"`
}

// Settings is the fully merged runtime configuration.
type Settings struct {
	Judge          JudgeSettings             `yaml:"judge"`
	Backends       map[string]map[string]any `yaml:"backends"`
	RedeliverBytes int                       `yaml:"redeliver_bytes"`
	DeadlineMs     int                       `yaml:"deadline_ms"`
	MaxInjectChars int                       `yaml:"max_inject_chars"`
	Journal        JournalSettings           `yaml:"journal"`
}

// UnitState describes one unit encountered during resolution.
type UnitState struct {
	ID      string
	File    string
	Profile string
	Source  string
	Status  string
	Unit    *unit.Unit
	Err     string
}

// Resolved is the ranked runtime view of all applicable profiles.
type Resolved struct {
	Units    []*unit.Unit
	States   []UnitState
	Settings Settings
	Profiles []string
	Dirs     []string
	Errors   []error
}

// Defaults parses the embedded default settings.
func Defaults() Settings {
	data, err := assets.FS.ReadFile("defaults.yaml")
	if err != nil {
		panic(err)
	}
	var settings Settings
	if err := yaml.Unmarshal(data, &settings); err != nil {
		panic(err)
	}
	return settings
}

// Threshold returns the configured threshold for the selected backend.
func (s Settings) Threshold() float64 {
	return s.ThresholdOf(s.Judge.Backend)
}

// ThresholdOf returns the configured threshold for a backend.
func (s Settings) ThresholdOf(name string) float64 {
	config, ok := s.Backends[name]
	if !ok {
		return 0.5
	}
	switch value := config["threshold"].(type) {
	case int:
		return float64(value)
	case float64:
		return value
	default:
		return 0.5
	}
}

// Load reads a profile.
func Load(dir string) (*Profile, error) { return load(dir) }

// LoadLock reads a profile lock.
func LoadLock(dir string) (*Lock, error) { return loadLock(dir) }

// SaveLock rewrites a profile lock.
func SaveLock(dir string, l *Lock) error { return saveLock(dir, l) }

// AddPack appends a pack reference while preserving YAML comments.
func AddPack(dir string, p PackRef) error { return addPack(dir, p) }

// RemovePack removes a pack reference while preserving YAML comments.
func RemovePack(dir, name string) error { return removePack(dir, name) }

// SetPackRef updates a pack ref while preserving YAML comments.
func SetPackRef(dir, name, ref string) error { return setPackRef(dir, name, ref) }

// AddDisable appends a disabled unit ID.
func AddDisable(dir, id string) error { return addDisable(dir, id) }

// RemoveDisable removes a disabled unit ID.
func RemoveDisable(dir, id string) error { return removeDisable(dir, id) }

// Init creates a new unregistered profile.
func Init(dir, name string) error { return initProfile(dir, name) }

// Commit commits profile changes when the profile is a git repository.
func Commit(dir, message string) string { return commit(dir, message) }

// Resolve builds the ranked runtime view for cwd.
func Resolve(cwd string) (*Resolved, error) { return resolve(cwd) }
