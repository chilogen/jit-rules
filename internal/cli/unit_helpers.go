package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/chilogen/jit-rules/internal/evaluate"
	"github.com/chilogen/jit-rules/internal/profile"
	"github.com/chilogen/jit-rules/internal/registry"
)

func resolveUnits(target string) (*profile.Resolved, string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, "", fmt.Errorf("读取当前目录: %w", err)
	}
	filter := ""
	if target != "" {
		selected, err := selectProfile(target)
		if err != nil {
			return nil, "", err
		}
		filter = selected.config.Name
		cwd = selected.config.Dir
		if !registry.IsGlobal(selected.entry) {
			cwd = registry.Scope(selected.entry)
		}
	}
	resolved, err := profile.Resolve(cwd)
	if err != nil {
		return nil, "", fmt.Errorf("解析 profile: %w", err)
	}
	return resolved, filter, nil
}

func findUnitState(id string) (profile.UnitState, *profile.Resolved, error) {
	resolved, _, err := resolveUnits("")
	if err != nil {
		return profile.UnitState{}, nil, err
	}
	var matches []profile.UnitState
	for _, state := range resolved.States {
		if state.ID == id || (!strings.Contains(id, "/") && shortUnitID(state.ID) == id) {
			matches = append(matches, state)
		}
	}
	if len(matches) == 0 {
		return profile.UnitState{}, resolved, fmt.Errorf("找不到单元 %s", id)
	}
	if !strings.Contains(id, "/") && distinctUnitIDs(matches) > 1 {
		return profile.UnitState{}, resolved, fmt.Errorf("短 id %s 不唯一，请使用全局 id", id)
	}
	return matches[0], resolved, nil
}

func requireParsedUnit(id string) (profile.UnitState, *profile.Resolved, error) {
	state, resolved, err := findUnitState(id)
	if err != nil {
		return state, resolved, err
	}
	if state.Unit == nil {
		return state, resolved, fmt.Errorf("单元 %s 无效：%s", state.ID, state.Err)
	}
	return state, resolved, nil
}

func shortUnitID(id string) string {
	if index := strings.IndexByte(id, '/'); index >= 0 {
		return id[index+1:]
	}
	return id
}

func distinctUnitIDs(states []profile.UnitState) int {
	ids := map[string]bool{}
	for _, state := range states {
		ids[state.ID] = true
	}
	return len(ids)
}

func testCounts(cases []evaluate.CaseResult) (profile.TestCounts, bool, bool) {
	var counts profile.TestCounts
	var failed, skipped bool
	for _, result := range cases {
		switch result.Result {
		case "pass":
			counts.Passed++
		case "skipped":
			counts.Skipped++
			skipped = true
		case "fail", "error":
			counts.Failed++
			failed = true
		}
	}
	return counts, failed, skipped
}

func caseLines(cases []evaluate.CaseResult) []string {
	lines := []string{"序号\t期望\t实际\t结果\t说明"}
	for _, result := range cases {
		lines = append(lines, fmt.Sprintf(
			"%d\t%s\t%s\t%s\t%s",
			result.Index, result.Expect, result.Got, result.Result, result.Note,
		))
	}
	return lines
}

func conflictOutput(conflicts []evaluate.Conflict, higher string) (string, []string, bool) {
	var reports, warnings []string
	confirmed := false
	for _, conflict := range conflicts {
		text := evaluate.FormatConflict(conflict, higher)
		if conflict.Confirmed {
			confirmed = true
			reports = append(reports, text)
		} else {
			warnings = append(warnings, text)
		}
	}
	if len(reports) == 0 && len(warnings) == 0 {
		reports = append(reports, "没有冲突")
	}
	return strings.Join(reports, "\n\n"), warnings, confirmed
}
