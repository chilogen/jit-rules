// Package harness loads definitions and translates native hook payloads.
package harness

import (
	"errors"
	"time"

	"github.com/chilogen/jit-rules/internal/deliver"
	"github.com/chilogen/jit-rules/internal/event"
)

// ErrIgnored identifies a recognized event that needs no processing.
var ErrIgnored = errors.New("ignored event")

// Definition describes one harness integration.
type Definition struct {
	Name           string            `yaml:"name"`
	Title          string            `yaml:"title"`
	Detect         Detect            `yaml:"detect"`
	Events         map[string]string `yaml:"events"`
	CompactSources []string          `yaml:"compact_sources"`
	Tools          map[string]string `yaml:"tools"`
	PatchTools     []string          `yaml:"patch_tools"`
	Dedupe         Dedupe            `yaml:"dedupe"`
	Supports       Supports          `yaml:"supports"`
	Install        Install           `yaml:"install"`
	Isolate        Isolate           `yaml:"isolate"`
}

// Dedupe defines native fields used for event claims.
type Dedupe struct {
	Prompt []string `yaml:"prompt"`
	Tool   []string `yaml:"tool"`
}

// Supports lists optional harness output capabilities.
type Supports struct {
	SystemMessage bool `yaml:"system_message"`
}

// Detect defines data-driven harness recognition.
type Detect struct {
	Order       int            `yaml:"order"`
	AnyFields   []string       `yaml:"any_fields"`
	FieldEquals map[string]any `yaml:"field_equals"`
	Fallback    bool           `yaml:"fallback"`
}

// Target defines install target paths and selection behavior.
type Target struct {
	Paths []string `yaml:"paths"`
	Mode  string   `yaml:"mode"`
}

// Command stores platform-specific hook command templates.
type Command struct {
	Unix    string `yaml:"unix"`
	Windows string `yaml:"windows"`
}

// Install defines data-driven installation behavior.
type Install struct {
	Kind           string                    `yaml:"kind"`
	Targets        map[string]Target         `yaml:"targets"`
	FileDefaults   map[string]any            `yaml:"file_defaults"`
	HandlerExtra   map[string]map[string]any `yaml:"handler_extra"`
	TimeoutSeconds int                       `yaml:"timeout_seconds"`
	Command        Command                   `yaml:"command"`
	Options        map[string]string         `yaml:"options"`
	Notes          []string                  `yaml:"notes"`
}

// Isolate defines data-driven isolated launch behavior.
type Isolate struct {
	Kind           string              `yaml:"kind"`
	Program        string              `yaml:"program"`
	Subcommands    []string            `yaml:"subcommands"`
	Args           []string            `yaml:"args"`
	SubcommandArgs map[string][]string `yaml:"subcommand_args"`
	Settings       map[string]any      `yaml:"settings"`
	Options        map[string]any      `yaml:"options"`
	Notes          []string            `yaml:"notes"`
}

// Output is an encoded harness response.
type Output struct {
	Stdout []byte
	Stderr []byte
	Exit   int
}

// Extracted contains normalized tool input fields.
type Extracted struct {
	Tool           string
	Effect         string
	Paths          []string
	Content        string
	ContentUnknown bool
	Shell          string
}

// LoadAll loads built-in and user harness definitions.
func LoadAll() ([]*Definition, error) { return loadAll() }

// Find loads one named harness definition.
func Find(name string) (*Definition, error) { return find(name) }

// DetectDef recognizes a raw payload.
func DetectDef(defs []*Definition, raw map[string]any) *Definition {
	return detectDef(defs, raw)
}

// Decode normalizes a native payload.
func Decode(def *Definition, raw []byte, now time.Time) (*event.Event, error) {
	return decode(def, raw, now)
}

// Extract normalizes tool input fields.
func Extract(def *Definition, rawTool string, input map[string]any, cwd, root string, exists func(abs string) bool) Extracted {
	return extract(def, rawTool, input, cwd, root, exists)
}

// Encode translates a delivery result to native output.
func Encode(def *Definition, ev *event.Event, res deliver.Result) Output {
	return encode(def, ev, res)
}

// HookEntries builds installable hook configuration entries.
func HookEntries(def *Definition, command string) map[string][]any {
	return hookEntries(def, command)
}

// CommandString renders a platform-specific hook command.
func CommandString(def *Definition, bin string, goos string) string {
	return commandString(def, bin, goos)
}
