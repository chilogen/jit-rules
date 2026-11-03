package profile

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/chilogen/jit-rules/assets"
	"github.com/chilogen/jit-rules/internal/pack"
	"github.com/chilogen/jit-rules/internal/paths"
	"github.com/chilogen/jit-rules/internal/registry"
	"github.com/chilogen/jit-rules/internal/unit"
	"gopkg.in/yaml.v3"
)

type rankedState struct {
	state       UnitState
	profileRank int
	sourceRank  int
	candidate   bool
	alive       bool
}

type loadedProfile struct {
	profile *Profile
	lock    *Lock
	rank    int
}

func resolve(cwd string) (*Resolved, error) {
	entries, err := registry.Load()
	if err != nil {
		return nil, err
	}
	result := &Resolved{Settings: Defaults()}
	var loaded []loadedProfile
	for rank, entry := range registry.Applicable(entries, cwd) {
		profile, err := load(entry.Path)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("%s: %w", entry.Path, err))
			continue
		}
		lock, err := loadLock(entry.Path)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("%s: %w", entry.Path, err))
			continue
		}
		loaded = append(loaded, loadedProfile{profile: profile, lock: lock, rank: rank})
		result.Profiles = append(result.Profiles, profile.Name)
		result.Dirs = append(result.Dirs, profile.Dir)
	}

	var states []*rankedState
	for _, item := range loaded {
		states = append(states, collectOwn(item, &result.Errors)...)
		states = append(states, collectPacks(item, &result.Errors)...)
	}
	sortRanked(states)
	applyDisable(states, loaded)
	applyShadow(states)
	applyReplaces(states)

	var active, inactive []UnitState
	for _, ranked := range states {
		if ranked.candidate && ranked.alive {
			ranked.state.Status = "active"
			result.Units = append(result.Units, ranked.state.Unit)
			active = append(active, ranked.state)
		} else {
			inactive = append(inactive, ranked.state)
		}
	}
	result.States = append(active, inactive...)
	settings, err := mergeSettings(loaded)
	if err != nil {
		return nil, err
	}
	result.Settings = settings
	return result, nil
}

func collectOwn(item loadedProfile, errorsList *[]error) []*rankedState {
	files, _ := filepath.Glob(filepath.Join(item.profile.Dir, "units", "*.md"))
	sort.Strings(files)
	states := make([]*rankedState, 0, len(files))
	for _, file := range files {
		data, readErr := os.ReadFile(file)
		shortID := strings.TrimSuffix(filepath.Base(file), ".md")
		id := item.profile.Name + "/" + shortID
		hash := ""
		if readErr == nil {
			sum := sha256.Sum256(data)
			hash = fmt.Sprintf("sha256:%x", sum)
		}
		parsed, parseErr := unit.Parse(data, file, item.profile.Name)
		if parseErr == nil {
			id = parsed.ID
			hash = parsed.Hash
			if validationErrors, _ := unit.Validate(parsed); len(validationErrors) > 0 {
				parseErr = fmt.Errorf("%v", validationErrors)
			}
		}
		state := UnitState{
			ID: id, File: file, Profile: item.profile.Name, Source: "own", Unit: parsed,
		}
		ranked := &rankedState{state: state, profileRank: item.rank, sourceRank: 0}
		locked, exists := item.lock.Units[id]
		switch {
		case readErr != nil || parseErr != nil:
			if readErr != nil {
				parseErr = readErr
			}
			ranked.state.Status = "invalid"
			ranked.state.Unit = nil
			ranked.state.Err = parseErr.Error()
			if exists && locked.Hash == hash {
				*errorsList = append(*errorsList, fmt.Errorf("%s: %w", file, parseErr))
			}
		case !exists:
			ranked.state.Status = "draft"
		case locked.Hash != hash:
			ranked.state.Status = "modified"
		default:
			ranked.candidate = true
			ranked.alive = true
		}
		states = append(states, ranked)
	}
	return states
}

func collectPacks(item loadedProfile, errorsList *[]error) []*rankedState {
	var states []*rankedState
	for index, reference := range item.profile.Packs {
		sourceRank := index + 1
		var units []*unit.Unit
		var dir string
		if reference.Source == "builtin" {
			manifest, builtins, err := pack.Builtin()
			if err != nil || manifest.Name != reference.Name {
				if err == nil {
					err = fmt.Errorf("builtin pack is %s, not %s", manifest.Name, reference.Name)
				}
				*errorsList = append(*errorsList, err)
				continue
			}
			units = builtins
		} else {
			locked, exists := item.lock.Packs[reference.Name]
			if !exists {
				continue
			}
			if locked.Commit != "" {
				dir = pack.GitDir(reference.Name, locked.Commit)
			} else {
				dir = paths.Expand(reference.Source, item.profile.Dir)
			}
			hash, err := pack.Hash(dir)
			if err != nil || hash != locked.Hash {
				*errorsList = append(*errorsList, fmt.Errorf(
					"包 %s 已被修改或缺失，运行 jitr pack update %s",
					reference.Name, reference.Name,
				))
				continue
			}
			units, states = readPackUnits(
				dir, reference, item, sourceRank, states, errorsList,
			)
		}
		for _, parsed := range units {
			if !included(reference, parsed.ID) {
				continue
			}
			states = append(states, &rankedState{
				state: UnitState{
					ID: parsed.ID, File: parsed.File, Profile: item.profile.Name,
					Source: reference.Name, Unit: parsed,
				},
				profileRank: item.rank, sourceRank: sourceRank, candidate: true, alive: true,
			})
		}
	}
	return states
}

func readPackUnits(
	dir string,
	reference PackRef,
	item loadedProfile,
	sourceRank int,
	states []*rankedState,
	errorsList *[]error,
) ([]*unit.Unit, []*rankedState) {
	files, _ := filepath.Glob(filepath.Join(dir, "units", "*.md"))
	sort.Strings(files)
	var valid []*unit.Unit
	for _, file := range files {
		parsed, err := unit.ParseFile(file, reference.Name)
		if err == nil {
			if validationErrors, _ := unit.Validate(parsed); len(validationErrors) > 0 {
				err = fmt.Errorf("%v", validationErrors)
			}
		}
		id := reference.Name + "/" + strings.TrimSuffix(filepath.Base(file), ".md")
		if parsed != nil {
			id = parsed.ID
		}
		if !included(reference, id) {
			continue
		}
		if err != nil {
			states = append(states, &rankedState{
				state: UnitState{
					ID: id, File: file, Profile: item.profile.Name, Source: reference.Name,
					Status: "invalid", Err: err.Error(),
				},
				profileRank: item.rank, sourceRank: sourceRank,
			})
			*errorsList = append(*errorsList, fmt.Errorf("%s: %w", file, err))
			continue
		}
		valid = append(valid, parsed)
	}
	return valid, states
}

func included(reference PackRef, id string) bool {
	shortID := strings.TrimPrefix(id, reference.Name+"/")
	includes := reference.Include
	if len(includes) == 0 {
		includes = []string{"*"}
	}
	match := false
	for _, pattern := range includes {
		if ok, _ := doublestar.Match(pattern, shortID); ok {
			match = true
			break
		}
	}
	if !match {
		return false
	}
	for _, pattern := range reference.Exclude {
		if ok, _ := doublestar.Match(pattern, shortID); ok {
			return false
		}
	}
	return true
}

func sortRanked(states []*rankedState) {
	sort.SliceStable(states, func(i, j int) bool {
		if states[i].profileRank != states[j].profileRank {
			return states[i].profileRank < states[j].profileRank
		}
		if states[i].sourceRank != states[j].sourceRank {
			return states[i].sourceRank < states[j].sourceRank
		}
		return states[i].state.ID < states[j].state.ID
	})
}

func applyDisable(states []*rankedState, profiles []loadedProfile) {
	disabled := make(map[int]map[string]bool, len(profiles))
	for _, item := range profiles {
		disabled[item.rank] = make(map[string]bool, len(item.profile.Disable))
		for _, id := range item.profile.Disable {
			disabled[item.rank][id] = true
		}
	}
	for _, state := range states {
		if state.candidate && disabled[state.profileRank][state.state.ID] {
			state.candidate = false
			state.alive = false
			state.state.Status = "disabled"
		}
	}
}

func applyShadow(states []*rankedState) {
	seen := map[string]bool{}
	for _, state := range states {
		if !state.candidate {
			continue
		}
		if seen[state.state.ID] {
			state.candidate = false
			state.alive = false
			state.state.Status = "shadowed"
			continue
		}
		seen[state.state.ID] = true
	}
}

func applyReplaces(states []*rankedState) {
	byID := map[string]*rankedState{}
	for _, state := range states {
		if state.candidate {
			byID[state.state.ID] = state
		}
	}
	for _, state := range states {
		if !state.candidate || !state.alive {
			continue
		}
		for _, id := range state.state.Unit.Replaces {
			replaced := byID[id]
			if replaced == nil || !replaced.alive || replaced == state {
				continue
			}
			replaced.alive = false
			replaced.state.Status = "replaced"
		}
	}
}

func mergeSettings(profiles []loadedProfile) (Settings, error) {
	data, err := assets.FS.ReadFile("defaults.yaml")
	if err != nil {
		return Settings{}, err
	}
	var merged map[string]any
	if err := yaml.Unmarshal(data, &merged); err != nil {
		return Settings{}, err
	}
	for index := len(profiles) - 1; index >= 0; index-- {
		deepMerge(merged, profiles[index].profile.Settings)
	}
	data, err = yaml.Marshal(merged)
	if err != nil {
		return Settings{}, err
	}
	var settings Settings
	if err := yaml.Unmarshal(data, &settings); err != nil {
		return Settings{}, err
	}
	return settings, nil
}

func deepMerge(target, source map[string]any) {
	for key, value := range source {
		sourceMap, sourceOK := value.(map[string]any)
		targetMap, targetOK := target[key].(map[string]any)
		if sourceOK && targetOK {
			deepMerge(targetMap, sourceMap)
			continue
		}
		target[key] = value
	}
}
