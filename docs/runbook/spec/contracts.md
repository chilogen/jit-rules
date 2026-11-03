# Spec: cross-package contracts

This is **the only source of every cross-package type and function signature**. Other spec files describe behavior only and do not redefine types; when they disagree, this file wins, and the disagreement is reported.

The shared contracts task writes the whole of this file into each package's `contract.go` in one go (function bodies are `panic("not implemented")`, except those the task says to implement fully). After that, **no task may change these types or signatures**, nor add exported types, fields, functions, or methods in `contract.go`. The only exception: types written as `struct{ /* unexported fields */ }` get the private fields they need, added in `contract.go` by the task that owns the package. The task that owns a package replaces `panic("not implemented")` with the implementation directly in `contract.go` (calling private functions in other files of the same package when the body is long); Go does not allow declaring a function with the same name twice in another file, so this is the only way to replace a body. Other files in the same package (not `_test.go`) may add private code freely, but must not add exported package-level types, functions, variables, or constants, nor exported fields or methods on the types in this file; methods that private types write to implement an interface (such as an implementation's `Kind()` or `Plan()`) are exempt; `internal/backend/fake` exports its struct as judge.md prescribes and is also exempt. Needing to export something new means the contract is insufficient; handle it per the next sentence. When a task finds the contract insufficient: stop that item, continue with the rest, and report what is missing and why; do not simulate it with another interface, compare error strings, or concatenate and split strings to get around it.

## paths

```go
package paths

func Config() string
func State() string
func Data() string
func Cache() string
func Bin() string
func Home() string
// Expand expands ~, {config}, {env:NAME:default}; if the result is still relative and root is non-empty, it is joined under root
func Expand(s, root string) string
// ProjectRoot returns the nearest directory, starting at cwd and walking up, that contains `.git`; cwd itself when there is none or cwd does not exist
func ProjectRoot(cwd string) string
```

## fileutil

```go
package fileutil

// WriteAtomic writes a temporary file in the same directory, then renames; creates the parent directory if missing; on Windows deletes an existing target before renaming
func WriteAtomic(path string, data []byte, perm os.FileMode) error
// Backup copies an existing file to <path>.bak-jitr-<YYYYMMDDHHMMSS> (local time); does nothing if the file does not exist
func Backup(path string, now time.Time) error
// ReplaceExecutable replaces dst with the contents of src, mode 0755 on Unix; on Windows first renames an existing dst to dst+".old"
func ReplaceExecutable(src, dst string) error
```

## event

```go
package event

type Event struct {
    V              int            `json:"v" yaml:"-"`                                     // always 1
    Harness        string         `json:"harness" yaml:"harness,omitempty"`               // name of the detected harness definition
    HookEvent      string         `json:"hook_event" yaml:"-"`                            // native event name, e.g. PreToolUse
    Kind           string         `json:"kind" yaml:"kind"`                               // prompt | tool | tool_result | stop | session_start | compact
    SessionID      string         `json:"session_id" yaml:"session_id,omitempty"`
    DedupeKey      string         `json:"dedupe_key" yaml:"-"`
    Cwd            string         `json:"cwd" yaml:"cwd,omitempty"`
    Root           string         `json:"root" yaml:"-"`
    TS             time.Time      `json:"ts" yaml:"-"`
    Tool           string         `json:"tool,omitempty" yaml:"tool,omitempty"`           // normalized tool name
    RawTool        string         `json:"raw_tool,omitempty" yaml:"raw_tool,omitempty"`
    Effect         string         `json:"effect,omitempty" yaml:"effect,omitempty"`       // effect of a shell command writing a file: write | edit | ""
    Paths          []string       `json:"paths,omitempty" yaml:"paths,omitempty"`
    Shell          string         `json:"shell,omitempty" yaml:"shell,omitempty"`
    Content        string         `json:"content,omitempty" yaml:"content,omitempty"`
    ContentUnknown bool           `json:"content_unknown,omitempty" yaml:"content_unknown,omitempty"` // a file is certainly being written, but its content cannot be known statically
    Prompt         string         `json:"prompt,omitempty" yaml:"prompt,omitempty"`
    StopActive     bool           `json:"stop_active,omitempty" yaml:"-"`                // stop only: the payload's stop_hook_active (this turn's end was already sent back once)
    Input          map[string]any `json:"-" yaml:"-"`                                     // raw tool_input, never journaled
    Size           int            `json:"size" yaml:"-"`
}

func CanonicalTool(raw string, table map[string]string) string
func SizeOf(kind string, payload map[string]any) int
// fields is dedupe.prompt or dedupe.tool from the definition; payload is the raw JSON object
func DedupeKey(kind string, fields []string, payload map[string]any, prompt string) string
```

## unit

```go
package unit

type Trigger struct {
    Events  []string `yaml:"events"`
    Tools   []string `yaml:"tools"`
    Paths   []string `yaml:"paths"`
    Shell   []string `yaml:"shell"`
    Prompt  []string `yaml:"prompt"`
    Content []string `yaml:"content"`
}
type Judge struct {
    Question string `yaml:"question"`
    Not      string `yaml:"not"`
    Backend  string `yaml:"backend"` // model backend name; empty means settings.judge.backend
    Cache    *bool  `yaml:"cache"`   // nil means the default: true for a question, false for a script
    Lang     string `yaml:"-"`       // interpreter of the script block
    Script   string `yaml:"-"`       // contents of the script block; non-empty means a script judge
}
type Deliver struct {
    As string `yaml:"as"` // inject | block; filled with inject when empty after parsing
}
type Example struct {
    Expect string      `yaml:"expect"` // hit | miss
    Event  event.Event `yaml:"event"`
}
type Unit struct {
    ID       string    `yaml:"-"` // global id
    Pack     string    `yaml:"-"` // pack name; the profile name for a profile's own units
    File     string    `yaml:"-"` // file path; builtin/core/units/<short id>.md for the built-in pack
    Hash     string    `yaml:"-"` // "sha256:" + hex of all the file's bytes
    Body     string    `yaml:"-"`
    Title    string    `yaml:"title"`
    Trigger  Trigger   `yaml:"trigger"`
    Judge    *Judge    `yaml:"judge"`
    Deliver  Deliver   `yaml:"deliver"`
    Topic    string    `yaml:"topic"`
    Replaces []string  `yaml:"replaces"`
    Examples []Example `yaml:"examples"`
}

func ParseFile(path, pack string) (*Unit, error)
// file is used only to compute the short id (base name without .md) and to fill Unit.File
func Parse(data []byte, file, pack string) (*Unit, error)
func Validate(u *Unit) (errs []error, warns []string)
// threshold applies only to a question judge; the caller passes Settings.ThresholdOf(the unit's backend)
func Describe(u *Unit, threshold float64) string
// Cached resolves Judge.Cache against its default
func (j *Judge) Cached() bool
```

`yaml:"-"` cannot be omitted: yaml.v3 decodes untagged fields by their lowercase names, so `id:` would be silently accepted.

## pack

```go
package pack

type Manifest struct {
    Name        string `yaml:"name"`
    Version     string `yaml:"version"`
    Description string `yaml:"description"`
}
type Fetched struct {
    Dir    string // directory of the pack
    Commit string // full commit sha for git sources; empty for local sources
}

// Fetch handles only local directories and git; source "builtin" is an error
func Fetch(source, ref string) (Fetched, error)
// Validate checks that the directory contains only allowed files
func Validate(dir string) error
func ReadManifest(dir string) (*Manifest, error)
// Units parses dir/units/*.md with pack name name, sorted by short id
func Units(dir, name string) ([]*unit.Unit, error)
// Hash computes the hash of the pack's content (algorithm in profile-pack.md)
func Hash(dir string) (string, error)
// Builtin reads the built-in core pack from embed
func Builtin() (*Manifest, []*unit.Unit, error)
// GitDir returns the directory of a git-sourced pack under {data}
func GitDir(name, commit string) string
```

## registry

```go
package registry

type Entry struct {
    Path string `yaml:"path"`
}

// Load reads {config}/registry.yaml; returns an empty list when the file does not exist
func Load() ([]Entry, error)
func Save(entries []Entry) error
// Scope derives a project profile's scope from its .jitr directory
func Scope(entry Entry) string
// IsGlobal reports whether an entry is the fixed ~/.jitr profile
func IsGlobal(entry Entry) bool
// Applicable returns the entries that apply to cwd, already ranked (highest first)
func Applicable(entries []Entry, cwd string) []Entry
```

## profile

```go
package profile

type PackRef struct {
    Name    string   `yaml:"name"`
    Source  string   `yaml:"source"`
    Ref     string   `yaml:"ref,omitempty"`
    Include []string `yaml:"include,omitempty"`
    Exclude []string `yaml:"exclude,omitempty"`
}
type Profile struct {
    Dir         string         `yaml:"-"`
    Name        string         `yaml:"name"`
    Description string         `yaml:"description,omitempty"`
    Packs       []PackRef      `yaml:"packs"`
    Disable     []string       `yaml:"disable,omitempty"`
    Settings    map[string]any `yaml:"settings,omitempty"`
}
type TestCounts struct {
    Passed  int `yaml:"passed"`
    Failed  int `yaml:"failed"`
    Skipped int `yaml:"skipped"`
}
type LockUnit struct {
    Hash      string     `yaml:"hash"`
    EnabledAt time.Time  `yaml:"enabled_at"`
    Tests     TestCounts `yaml:"tests"`
}
type LockPack struct {
    Source  string    `yaml:"source"`
    Ref     string    `yaml:"ref,omitempty"`
    Commit  string    `yaml:"commit,omitempty"`
    Hash    string    `yaml:"hash"`
    AddedAt time.Time `yaml:"added_at"`
}
type Lock struct {
    Version int                 `yaml:"version"`
    Units   map[string]LockUnit `yaml:"units"`
    Packs   map[string]LockPack `yaml:"packs"`
}

type JudgeSettings struct {
    Backend       string `yaml:"backend"`
    TimeoutMs     int    `yaml:"timeout_ms"`
    MaxFieldChars int    `yaml:"max_field_chars"`
    SendContent   bool   `yaml:"send_content"`
}
type JournalSettings struct {
    KeepDays     int  `yaml:"keep_days"`
    StoreContent bool `yaml:"store_content"`
}
type Settings struct {
    Judge          JudgeSettings             `yaml:"judge"`
    Backends       map[string]map[string]any `yaml:"backends"`
    RedeliverBytes int                       `yaml:"redeliver_bytes"`
    DeadlineMs     int                       `yaml:"deadline_ms"`
    MaxInjectChars int                       `yaml:"max_inject_chars"`
    Journal        JournalSettings           `yaml:"journal"`
}

// UnitState is a unit's situation within Resolve, used by unit list and status
type UnitState struct {
    ID      string
    File    string
    Profile string     // name of the profile it is in
    Source  string     // "own", or the pack name
    Status  string     // active | modified | draft | disabled | replaced | shadowed | invalid
    Unit    *unit.Unit // nil when Status is invalid
    Err     string     // the reason when Status is invalid
}
type Resolved struct {
    Units    []*unit.Unit // active units, already ranked
    States   []UnitState  // every unit seen, in rank order, inactive ones last
    Settings Settings
    Profiles []string // names of the profiles in effect, by rank
    Dirs     []string // directories matching Profiles one to one
    Errors   []error
}

// Defaults parses the embedded assets/defaults.yaml
func Defaults() Settings
// Threshold is ThresholdOf(Judge.Backend)
func (s Settings) Threshold() float64
// ThresholdOf is Backends[name]["threshold"] (int or float64), 0.5 when absent
func (s Settings) ThresholdOf(name string) float64
func Load(dir string) (*Profile, error)
// LoadLock reads jitr.lock; returns an empty Lock with Version 1 when the file does not exist
func LoadLock(dir string) (*Lock, error)
// SaveLock rewrites the whole jitr.lock
func SaveLock(dir string, l *Lock) error
// The five functions below modify the YAML nodes of profile.yaml directly, keeping the rest of the content and comments
func AddPack(dir string, p PackRef) error
func RemovePack(dir, name string) error
func SetPackRef(dir, name, ref string) error
func AddDisable(dir, id string) error
func RemoveDisable(dir, id string) error
// Init creates a new profile (contents in profile-pack.md) without registering it
func Init(dir, name string) error
// Commit commits every change when dir is a git repository; returns a warning on failure, empty on success or when not a repository
func Commit(dir, message string) string
func Resolve(cwd string) (*Resolved, error)
```

## gate

```go
package gate

type Outcome struct {
    OK               bool
    Failed           string // name of the first field that failed: tools | paths | shell | prompt | content
    ContentUncertain bool   // a content condition is written, and the event's ContentUnknown is true
}

func Match(u *unit.Unit, ev *event.Event) Outcome
```

## backend

```go
package backend

// ErrNotConfigured means the backend lacks required configuration (for example an empty api key variable). Backends wrap it with %w in the errors they return
var ErrNotConfigured = errors.New("judge backend not configured")

type Question struct {
    ID   string // global id of the unit
    Text string // judge.question
    Not  string // judge.not, may be empty
}
type Result struct {
    P      map[string]float64 // keyed by Question.ID
    Model  string             // model version that actually answered; the configured model when absent
    ConnMs int                // milliseconds spent setting up a new connection; 0 when reused or no connection was made. Filled on errors too
}
type Backend interface {
    Name() string
    // cfg is the backends.<Name()> section of settings
    Judge(ctx context.Context, state map[string]any, qs []Question, cfg map[string]any) (Result, error)
}

func Register(b Backend)
func Get(name string) (Backend, bool)
// Names returns registered names in lexical order
func Names() []string

// ConnTimer records the time spent setting up a new connection in one request, shared by jev and openai (rules in "Timing" of judge.md)
type ConnTimer struct{ /* unexported fields */ }
// NewConnTimer creates a timer whose start is the moment of creation
func NewConnTimer() *ConnTimer
// Trace returns the ClientTrace to attach to the request context (only GotConn is used)
func (t *ConnTimer) Trace() *httptrace.ClientTrace
// Ms returns the recorded milliseconds; 0 when no new connection was made. Callable at any time from any goroutine
func (t *ConnTimer) Ms() int
```

The threshold is determined only by `profile.Settings.Threshold()`; backends no longer provide one.

## judge

```go
package judge

type Verdict struct {
    Hit          bool
    P            float64
    Fallback     bool   // error or timeout, treated as a hit
    Unconfigured bool   // the backend is not configured (ErrNotConfigured), treated as a miss
    Backend      string
    Model        string
    ConnMs       int
    Ms           int
    Err          string
    Exit         *int   // script judges only: the exit code; nil when the process did not exit by itself or the result came from the cache
    Stderr       string // script judges only: trimmed stderr, first 500 runes
}

func Decide(ctx context.Context, ev *event.Event, units []*unit.Unit, s profile.Settings) map[string]Verdict
```

## session

```go
package session

type Mark struct {
    At int `json:"at"` // value of bytes at delivery
}
type State struct {
    V         int                  `json:"v"`
    Bytes     int                  `json:"bytes"`
    Delivered map[string]*Mark     `json:"delivered"`
    Seen      map[string]time.Time `json:"seen"`
    Notified  bool                 `json:"notified"`
    Updated   time.Time            `json:"updated"`
}
type Store struct{ /* unexported fields */ }

func Open(harness, sessionID string) *Store
// lock → read → fn modifies → write back → unlock. Returns locked=false without calling fn when the lock cannot be taken within 200ms
func (s *Store) Update(now time.Time, fn func(*State)) (locked bool, err error)
// checks and registers key under the lock (lifetime rules in pipeline.md); dup=true while still within its lifetime
func (s *Store) Claim(key string, now time.Time) (dup, locked bool, err error)
func (s *Store) Read() (*State, error)
func GC(now time.Time)
```

## deliver

```go
package deliver

type Hit struct {
    Unit    *unit.Unit
    Verdict *judge.Verdict // nil for units without a judge
    NoBlock bool           // this hit is uncertain: it may inject but never block (rules in pipeline.md)
}
type Result struct {
    Action    string            // none | inject | block
    Text      string
    Notice    string            // filled by the pipeline after Decide
    Delivered []string          // global ids of the units whose bodies made it into Text
    Blocked   []string          // the units in Delivered that serve as block reasons
    Skips     map[string]string // global id -> topic | redeliver | truncated
}

// hits are already ranked; st nil means stateless mode (treated as an empty state)
func Decide(hits []Hit, ev *event.Event, st *session.State, s profile.Settings) Result
```

## journal

```go
package journal

type JudgeRecord struct {
    P            float64 `json:"p"`
    Hit          bool    `json:"hit"`
    Fallback     bool    `json:"fallback"`
    Unconfigured bool    `json:"unconfigured"`
    Backend      string  `json:"backend"`
    Model        string  `json:"model"`
    ConnMs       int     `json:"conn_ms"`
    Ms           int     `json:"ms"`
    Err          string  `json:"err"`
    Exit         *int    `json:"exit,omitempty"`
    Stderr       string  `json:"stderr,omitempty"`
}
type UnitRecord struct {
    ID        string       `json:"id"`
    Hash      string       `json:"hash"`
    Judge     *JudgeRecord `json:"judge"`
    NoBlock   bool         `json:"no_block"`
    Skip      string       `json:"skip"`
    Delivered string       `json:"delivered"` // inject | block | ""
    Gate      string       `json:"gate,omitempty"` // empty when the gate passed; otherwise Outcome.Failed (pipeline.md step 7)
}
type ResultRecord struct {
    Action string `json:"action"`
    Chars  int    `json:"chars"`
}
type Record struct {
    V           int          `json:"v"`
    TS          time.Time    `json:"ts"`
    Isolated    bool         `json:"isolated"`
    Event       event.Event  `json:"event"`
    Profiles    []string     `json:"profiles"`
    GateFailed  int          `json:"gate_failed"`
    Units       []UnitRecord `json:"units"`
    Result      ResultRecord `json:"result"`
    Ms          int          `json:"ms"`
    Err         string       `json:"err"`
}

// with storeContent false, the Event's Content, Shell, and Prompt are rewritten as "sha256:<hex>" before writing
func Append(r Record, storeContent bool) error
func Since(t time.Time) ([]Record, error)
func GC(now time.Time, keepDays int)
func LogError(stage string, err error) // writes {state}/errors.jsonl
```

## harness

```go
package harness

var ErrIgnored = errors.New("ignored event")

type Definition struct {
    Name           string            `yaml:"name"`            // ^[a-z0-9-]+$
    Title          string            `yaml:"title"`
    Detect         Detect            `yaml:"detect"`
    Events         map[string]string `yaml:"events"`          // native event name -> kind
    CompactSources []string          `yaml:"compact_sources"` // kind=compact when SessionStart's source is in this list
    Tools          map[string]string `yaml:"tools"`
    PatchTools     []string          `yaml:"patch_tools"`
    Dedupe         Dedupe            `yaml:"dedupe"`
    Supports       Supports          `yaml:"supports"`
    Install        Install           `yaml:"install"`
    Isolate        Isolate           `yaml:"isolate"`
}
type Dedupe struct {
    Prompt []string `yaml:"prompt"`
    Tool   []string `yaml:"tool"`
}
type Supports struct {
    SystemMessage bool `yaml:"system_message"`
}
type Detect struct {
    Order       int            `yaml:"order"`
    AnyFields   []string       `yaml:"any_fields"`   // the payload's top level has any of these keys
    FieldEquals map[string]any `yaml:"field_equals"` // the payload's top level has all these keys with equal values (compared as strings)
    Fallback    bool           `yaml:"fallback"`
}
type Target struct {
    Paths []string `yaml:"paths"`
    Mode  string   `yaml:"mode"` // first | existing
}
type Command struct {
    Unix    string `yaml:"unix"`
    Windows string `yaml:"windows"`
}
type Install struct {
    Kind           string                    `yaml:"kind"`
    Targets        map[string]Target         `yaml:"targets"` // user / project
    FileDefaults   map[string]any            `yaml:"file_defaults"`
    HandlerExtra   map[string]map[string]any `yaml:"handler_extra"` // native event name -> extra fields
    TimeoutSeconds int                       `yaml:"timeout_seconds"`
    Command        Command                   `yaml:"command"`
    Options        map[string]string         `yaml:"options"`
    Notes          []string                  `yaml:"notes"`
}
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
type Output struct {
    Stdout []byte
    Stderr []byte
    Exit   int
}
type Extracted struct {
    Tool           string
    Effect         string
    Paths          []string // already normalized
    Content        string
    ContentUnknown bool
    Shell          string
}

func LoadAll() ([]*Definition, error) // built-in definitions + {config}/harnesses/*.yaml, the latter overriding on name clashes
func Find(name string) (*Definition, error)
func DetectDef(defs []*Definition, raw map[string]any) *Definition
func Decode(def *Definition, raw []byte, now time.Time) (*event.Event, error)
// exists reports whether a file exists at an absolute path; Decode passes an os.Stat-based implementation, tests pass a fake
func Extract(def *Definition, rawTool string, input map[string]any, cwd, root string, exists func(abs string) bool) Extracted
func Encode(def *Definition, ev *event.Event, res deliver.Result) Output
// HookEntries produces all hook entries for a harness: native event name -> array of entries for that event (hooks-json format). Used by both installer and launcher
func HookEntries(def *Definition, command string) map[string][]any
func CommandString(def *Definition, bin string, goos string) string
```

## installer

```go
package installer

type Change struct {
    Path   string
    Before []byte // original content; nil when the file does not exist
    After  []byte // new content; nil means delete the file
}
type Request struct {
    Def       *harness.Definition
    Scope     string // "user" or "project"
    Root      string // project root in the project scope
    Bin       string // absolute path of jitr at the stable path
    Uninstall bool
    GOOS      string // platform for generating the command string; runtime.GOOS when empty. Lets tests cover both platforms on one machine
}
type Installer interface {
    Kind() string                              // matches install.kind in the harness definition
    Plan(req Request) ([]Change, error)        // computes only, writes nothing
    Installed(req Request) (bool, error)       // whether the target file has jitr's entries; false when the target file does not exist
}

func Register(i Installer)
func Get(kind string) (Installer, bool)
// TargetPaths computes the files to write per Target.Mode (placeholders expanded)
func TargetPaths(def *harness.Definition, scope, root string) ([]string, error)
// Apply, for each Change: back up (fileutil.Backup), then write atomically; delete when After is nil
func Apply(changes []Change, now time.Time) error
```

## launcher

```go
package launcher

type Request struct {
    Def      *harness.Definition
    Bin      string   // absolute path of jitr at the stable path
    UserArgs []string // arguments the user gave after --
    NoJitr   bool
    TempDir  string
    GOOS     string // same as installer.Request.GOOS
}
type Launch struct {
    Program string
    Args    []string // complete arguments (already merged with the user's per the rules)
    Env     map[string]string // environment variables added or overridden on top of the current environment; nil when none
}
type Launcher interface {
    Kind() string // matches isolate.kind in the harness definition
    Prepare(req Request) (Launch, error)
}

func Register(l Launcher)
func Get(kind string) (Launcher, bool)
// MergeArgs implements "Where arguments are inserted" in harness.md; generated are the arguments the Launcher produced itself
func MergeArgs(def *harness.Definition, userArgs, generated []string) []string
```

## pipeline

```go
package pipeline

type GateTrace struct {
    ID     string
    OK     bool
    Failed string
}
type Trace struct {
    Gates  []GateTrace    // every unit whose Trigger.Events contains ev.Kind
    Record journal.Record // the same as the journal record Run would write
    Result deliver.Result
}

func Run(stdin []byte, now time.Time) harness.Output
// Explain runs Resolve, the gate, judgment, and the delivery decision on an already decoded event; no dedupe, no session state reads or writes, no journal
func Explain(ev *event.Event, now time.Time) (*Trace, error)
```

## evaluate

```go
package evaluate

type CaseResult struct {
    Index  int    // which example, from 0; -1 for the structural check
    Expect string // hit | miss
    Got    string // hit | miss | ""
    Result string // pass | fail | skipped | error
    Note   string
}
type TestRecord struct {
    Hash  string       `json:"hash"`
    TS    time.Time    `json:"ts"`
    Cases []CaseResult `json:"cases"`
}
type ReplayHit struct {
    TS      time.Time
    Summary string
}
type ReplayReport struct {
    Hits    []ReplayHit
    Judged  int // number of events judged
    Unknown int // number of events that needed model judgment but had no usable result
}
type Conflict struct {
    A, B      string // global ids
    Example   string // summary of the example that fires both
    Confirmed bool   // false means the model could not be called to confirm; only a "possible conflict"
}

func Test(u *unit.Unit, s profile.Settings, offline bool) []CaseResult
func SaveTest(u *unit.Unit, cases []CaseResult, now time.Time) error
func LoadTest(id string) (*TestRecord, error) // nil, nil when there is no record
func Replay(u *unit.Unit, s profile.Settings, since time.Duration, live bool, now time.Time) (ReplayReport, error)
// CoHits: a sorted pair of global ids → summary of one real event on which both hit
type CoHits map[[2]string]string
func LoadCoHits() CoHits
func Conflicts(target *unit.Unit, all []*unit.Unit, s profile.Settings, history CoHits) []Conflict
// FormatConflict prints the conflict report from judge.md; higher is the higher-ranked id of the two
func FormatConflict(c Conflict, higher string) string
```

## cli

```go
package cli

// injected by make with -ldflags -X; defaults "dev" and ""
var Version = "dev"
var Commit = ""

// each command file calls Register(cmd) in its own init(), without changing root.go
func Register(cmd *cobra.Command)
func Execute() int // returns the process exit code, rules in cli.md
// Output prints uniformly: with --json, {"ok","data","errors","warnings"}; otherwise the human string plus errors and warnings
func Output(cmd *cobra.Command, ok bool, data any, human string, errs []error, warns []string)
// Internal marks err as an internal error, exit code 3
func Internal(err error) error
// Silent means the result was already printed with Output; Execute prints nothing more and uses code as the exit code
func Silent(code int) error
```

## assets

```go
package assets

//go:embed guide.md defaults.yaml unit-template.md harnesses/*.yaml packs
var FS embed.FS
```
