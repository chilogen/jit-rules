package profile

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/chilogen/jit-rules/internal/pack"
	"github.com/chilogen/jit-rules/internal/registry"
	"github.com/chilogen/jit-rules/internal/unit"
	"gopkg.in/yaml.v3"
)

func TestProfileEditsPreserveComments(t *testing.T) {
	dir := t.TempDir()
	content := `# file comment
name: me
# packs comment
packs:
  - name: core # pack comment
    source: builtin
settings: {}
`
	writeFile(t, filepath.Join(dir, "profile.yaml"), content)
	assertComments := func() {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(dir, "profile.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		for _, comment := range []string{"# file comment", "# packs comment", "# pack comment"} {
			if !strings.Contains(string(data), comment) {
				t.Fatalf("missing %q after edit:\n%s", comment, data)
			}
		}
		if _, err := Load(dir); err != nil {
			t.Fatalf("Load() after edit: %v", err)
		}
	}
	steps := []func() error{
		func() error { return AddPack(dir, PackRef{Name: "one", Source: "/one"}) },
		func() error { return AddPack(dir, PackRef{Name: "two", Source: "/two"}) },
		func() error { return SetPackRef(dir, "one", "v2") },
		func() error { return RemovePack(dir, "two") },
		func() error { return AddDisable(dir, "one/test") },
		func() error { return AddDisable(dir, "one/test") },
		func() error { return RemoveDisable(dir, "one/test") },
	}
	for _, step := range steps {
		if err := step(); err != nil {
			t.Fatal(err)
		}
		assertComments()
	}
	profile, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(profile.Packs) != 2 || profile.Packs[1].Name != "one" ||
		profile.Packs[1].Ref != "v2" || len(profile.Disable) != 0 {
		t.Fatalf("edited profile = %#v", profile)
	}
	if err := RemovePack(dir, "missing"); err == nil {
		t.Fatal("RemovePack(missing) error = nil")
	}
}

func TestLockRoundTrip(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 11, 2, 1, 2, 3, 0, time.UTC)
	want := &Lock{
		Version: 1,
		Units: map[string]LockUnit{
			"me/test": {Hash: "sha256:a", EnabledAt: now, Tests: TestCounts{Passed: 2}},
		},
		Packs: map[string]LockPack{
			"sample": {Source: "/sample", Hash: "sha256:b", AddedAt: now},
		},
	}
	if err := SaveLock(dir, want); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "jitr.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), lockComment) {
		t.Fatalf("lock prefix = %q", data)
	}
	got, err := LoadLock(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("LoadLock() = %#v, want %#v", got, want)
	}
}

func TestInit(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "profile")
	if err := Init(dir, "me"); err != nil {
		t.Fatal(err)
	}
	profile, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(profile.Packs) != 1 || profile.Packs[0].Name != "core" ||
		profile.Packs[0].Source != "builtin" {
		t.Fatalf("Init profile = %#v", profile)
	}
	lock, err := LoadLock(dir)
	if err != nil {
		t.Fatal(err)
	}
	if lock.Version != 1 || len(lock.Units) != 0 || len(lock.Packs) != 0 {
		t.Fatalf("Init lock = %#v", lock)
	}

	git, err := exec.LookPath("git")
	if err != nil {
		return
	}
	parent := t.TempDir()
	runProfileGit(t, git, parent, "init")
	child := filepath.Join(parent, "nested", "profile")
	if err := Init(child, "nested"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(child, ".git")); !os.IsNotExist(err) {
		t.Fatalf("Init created nested .git: %v", err)
	}
}

func TestResolveOwnStates(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	dir := makeProfile(t, "mine", nil, nil, nil)
	active := writeUnit(t, dir, "active", "")
	modified := writeUnit(t, dir, "modified", "")
	writeUnit(t, dir, "draft", "")
	writeFile(t, filepath.Join(dir, "units", "invalid.md"), "not a unit\n")
	lock := emptyLock()
	lock.Units["mine/active"] = LockUnit{Hash: parsedHash(t, active)}
	lock.Units["mine/modified"] = LockUnit{Hash: "sha256:old"}
	if err := SaveLock(dir, lock); err != nil {
		t.Fatal(err)
	}
	cwd := saveRegistry(t, dir)
	resolved, err := Resolve(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if got := unitIDs(resolved.Units); !reflect.DeepEqual(got, []string{"mine/active"}) {
		t.Fatalf("Units = %v", got)
	}
	assertState(t, resolved, "mine/active", "active")
	assertState(t, resolved, "mine/modified", "modified")
	assertState(t, resolved, "mine/draft", "draft")
	assertState(t, resolved, "mine/invalid", "invalid")
	if len(resolved.Errors) != 0 {
		t.Fatalf("Errors = %v", resolved.Errors)
	}

	data, err := os.ReadFile(modified)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(modified, append(data, 'x'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestResolveShadowReplaceDisableAndFilters(t *testing.T) {
	t.Run("shadow", func(t *testing.T) {
		t.Setenv("JITR_HOME", t.TempDir())
		packDir := makeLocalPack(t, "shared", map[string]string{"same": ""})
		hash, _ := pack.Hash(packDir)
		a := makeProfile(t, "a", []PackRef{{Name: "shared", Source: packDir}}, nil, nil)
		b := makeProfile(t, "b", []PackRef{{Name: "shared", Source: packDir}}, nil, nil)
		for _, dir := range []string{a, b} {
			lock := emptyLock()
			lock.Packs["shared"] = LockPack{Source: packDir, Hash: hash}
			if err := SaveLock(dir, lock); err != nil {
				t.Fatal(err)
			}
		}
		cwd := saveRegistry(t, a, b)
		resolved, err := Resolve(cwd)
		if err != nil {
			t.Fatal(err)
		}
		if got := unitIDs(resolved.Units); !reflect.DeepEqual(got, []string{"shared/same"}) {
			t.Fatalf("Units = %v", got)
		}
		statuses := statesFor(resolved, "shared/same")
		if !reflect.DeepEqual(statuses, []string{"active", "shadowed"}) {
			t.Fatalf("statuses = %v", statuses)
		}
	})

	t.Run("replace-disable-filter", func(t *testing.T) {
		t.Setenv("JITR_HOME", t.TempDir())
		a := makeProfile(t, "a", nil, nil, nil)
		target := writeUnit(t, a, "target", "")
		aLock := emptyLock()
		aLock.Units["a/target"] = LockUnit{Hash: parsedHash(t, target)}
		if err := SaveLock(a, aLock); err != nil {
			t.Fatal(err)
		}
		packDir := makeLocalPack(t, "filter", map[string]string{
			"keep": "", "drop": "", "exclude": "",
		})
		hash, _ := pack.Hash(packDir)
		b := makeProfile(t, "b", []PackRef{{
			Name: "filter", Source: packDir,
			Include: []string{"keep*", "exclude*"}, Exclude: []string{"exclude*"},
		}}, []string{"b/off"}, nil)
		replacer := writeUnit(t, b, "replacer", "a/target")
		off := writeUnit(t, b, "off", "")
		bLock := emptyLock()
		bLock.Units["b/replacer"] = LockUnit{Hash: parsedHash(t, replacer)}
		bLock.Units["b/off"] = LockUnit{Hash: parsedHash(t, off)}
		bLock.Packs["filter"] = LockPack{Source: packDir, Hash: hash}
		if err := SaveLock(b, bLock); err != nil {
			t.Fatal(err)
		}
		cwd := saveRegistry(t, a, b)
		resolved, err := Resolve(cwd)
		if err != nil {
			t.Fatal(err)
		}
		if got := unitIDs(resolved.Units); !reflect.DeepEqual(got, []string{
			"b/replacer", "filter/keep",
		}) {
			t.Fatalf("Units = %v", got)
		}
		assertState(t, resolved, "a/target", "replaced")
		assertState(t, resolved, "b/off", "disabled")
		if len(statesFor(resolved, "filter/drop")) != 0 ||
			len(statesFor(resolved, "filter/exclude")) != 0 {
			t.Fatalf("filtered units appear in states: %#v", resolved.States)
		}
	})
}

func TestResolveLocalPackMutation(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	packDir := makeLocalPack(t, "local", map[string]string{"test": ""})
	hash, _ := pack.Hash(packDir)
	dir := makeProfile(t, "mine", []PackRef{{Name: "local", Source: packDir}}, nil, nil)
	lock := emptyLock()
	lock.Packs["local"] = LockPack{Source: packDir, Hash: hash}
	if err := SaveLock(dir, lock); err != nil {
		t.Fatal(err)
	}
	cwd := saveRegistry(t, dir)
	resolved, err := Resolve(cwd)
	if err != nil || len(resolved.Units) != 1 {
		t.Fatalf("Resolve() = %#v, %v", resolved, err)
	}
	path := filepath.Join(packDir, "units", "test.md")
	data, _ := os.ReadFile(path)
	writeFile(t, path, string(data)+"x")
	resolved, err = Resolve(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Units) != 0 || len(resolved.Errors) != 1 ||
		!strings.Contains(resolved.Errors[0].Error(), "jitr pack update") {
		t.Fatalf("mutated pack Resolve() = %#v", resolved)
	}
}

func TestResolveSettingsAndBrokenProfiles(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	a := makeProfile(t, "a", nil, nil, map[string]any{"redeliver_bytes": 1})
	b := makeProfile(t, "b", nil, nil, map[string]any{
		"redeliver_bytes": 2,
		"backends": map[string]any{
			"jev": map[string]any{"threshold": 0.6},
		},
	})
	badKey := makeProfile(t, "bad-key", nil, nil, map[string]any{
		"backends": map[string]any{"jev": map[string]any{"api_key": "secret"}},
	})
	broken := t.TempDir()
	writeFile(t, filepath.Join(broken, "profile.yaml"), "name: [\n")
	cwd := saveRegistry(t, a, badKey, broken, b)
	resolved, err := Resolve(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Settings.RedeliverBytes != 2 ||
		resolved.Settings.Backends["jev"]["threshold"] != 0.6 ||
		resolved.Settings.Backends["jev"]["model"] != "jev-latest" {
		t.Fatalf("Settings = %#v", resolved.Settings)
	}
	if resolved.Settings.Judge.TimeoutMs != Defaults().Judge.TimeoutMs {
		t.Fatal("default setting was not retained")
	}
	if len(resolved.Errors) != 2 || !reflect.DeepEqual(resolved.Profiles, []string{"b", "a"}) {
		t.Fatalf("Errors = %v, Profiles = %v", resolved.Errors, resolved.Profiles)
	}
}

func makeProfile(
	t *testing.T,
	name string,
	packs []PackRef,
	disable []string,
	settings map[string]any,
) string {
	t.Helper()
	dir := t.TempDir()
	if packs == nil {
		packs = []PackRef{}
	}
	if settings == nil {
		settings = map[string]any{}
	}
	profile := Profile{Name: name, Packs: packs, Disable: disable, Settings: settings}
	data, err := yaml.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "profile.yaml"), string(data))
	if err := os.Mkdir(filepath.Join(dir, "units"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := SaveLock(dir, emptyLock()); err != nil {
		t.Fatal(err)
	}
	return dir
}

func makeLocalPack(t *testing.T, name string, units map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pack.yaml"),
		"name: "+name+"\nversion: v1\ndescription: test\n")
	if err := os.Mkdir(filepath.Join(dir, "units"), 0o755); err != nil {
		t.Fatal(err)
	}
	for shortID, replaces := range units {
		writeFile(t, filepath.Join(dir, "units", shortID+".md"), unitText(replaces))
	}
	return dir
}

func writeUnit(t *testing.T, profileDir, shortID, replaces string) string {
	t.Helper()
	path := filepath.Join(profileDir, "units", shortID+".md")
	writeFile(t, path, unitText(replaces))
	return path
}

func unitText(replaces string) string {
	line := ""
	if replaces != "" {
		line = "replaces: [" + replaces + "]\n"
	}
	return `---
title: Test unit
trigger:
  events: [prompt]
  prompt: [test]
deliver:
  as: inject
` + line + `examples:
  - expect: hit
    event: {kind: prompt, prompt: test}
  - expect: miss
    event: {kind: prompt, prompt: other}
---
Test body.
`
}

func parsedHash(t *testing.T, path string) string {
	t.Helper()
	parsed, err := unit.ParseFile(path, strings.TrimSuffix(filepath.Base(filepath.Dir(path)), "units"))
	if err != nil {
		t.Fatal(err)
	}
	return parsed.Hash
}

func saveRegistry(t *testing.T, dirs ...string) string {
	t.Helper()
	entries := make([]registry.Entry, len(dirs))
	current := t.TempDir()
	for index, dir := range dirs {
		if index > 0 {
			current = filepath.Join(current, "child")
			if err := os.Mkdir(current, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		target := filepath.Join(current, ".jitr")
		if err := os.CopyFS(target, os.DirFS(dir)); err != nil {
			t.Fatal(err)
		}
		entries[index] = registry.Entry{Path: target}
	}
	if err := registry.Save(entries); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(current, "work")
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func unitIDs(units []*unit.Unit) []string {
	ids := make([]string, len(units))
	for index, parsed := range units {
		ids[index] = parsed.ID
	}
	return ids
}

func assertState(t *testing.T, resolved *Resolved, id, status string) {
	t.Helper()
	statuses := statesFor(resolved, id)
	if len(statuses) != 1 || statuses[0] != status {
		t.Fatalf("state %s = %v, want %s", id, statuses, status)
	}
}

func statesFor(resolved *Resolved, id string) []string {
	var statuses []string
	for _, state := range resolved.States {
		if state.ID == id {
			statuses = append(statuses, state.Status)
		}
	}
	return statuses
}

func runProfileGit(t *testing.T, git, dir string, args ...string) {
	t.Helper()
	command := exec.Command(git, args...)
	command.Dir = dir
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}
