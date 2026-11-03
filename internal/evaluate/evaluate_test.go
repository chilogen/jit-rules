package evaluate

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chilogen/jit-rules/internal/backend"
	"github.com/chilogen/jit-rules/internal/backend/fake"
	"github.com/chilogen/jit-rules/internal/event"
	"github.com/chilogen/jit-rules/internal/journal"
	"github.com/chilogen/jit-rules/internal/profile"
	"github.com/chilogen/jit-rules/internal/unit"
)

func TestTestEvaluatesExamplesWithFake(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	u := judgedUnit("me/language")
	settings := fakeSettings(t, "evaluate-cases", &fake.Backend{
		Func: func(state map[string]any, _ backend.Question) float64 {
			if strings.Contains(stringValue(state["shell"]), "文档") {
				return 0.1
			}
			return 0.9
		},
	})
	cases := Test(u, settings, false)
	if len(cases) != 4 {
		t.Fatalf("cases = %#v", cases)
	}
	for _, result := range cases {
		if result.Result != "pass" {
			t.Errorf("case = %#v, want pass", result)
		}
	}
}

func TestTestHandlesOfflineUnconfiguredAndFallback(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	u := judgedUnit("me/modes")
	offline := Test(u, profile.Defaults(), true)
	if offline[0].Result != "skipped" {
		t.Fatalf("offline = %#v", offline[0])
	}

	unconfiguredSettings := fakeSettings(t, "evaluate-unconfigured", &fake.Backend{
		Err: backend.ErrNotConfigured,
	})
	unconfigured := Test(u, unconfiguredSettings, false)
	if unconfigured[0].Result != "skipped" {
		t.Fatalf("unconfigured = %#v", unconfigured[0])
	}

	fallbackSettings := fakeSettings(t, "evaluate-fallback", &fake.Backend{
		Err: errors.New("backend failed"),
	})
	fallback := Test(u, fallbackSettings, false)
	if fallback[0].Result != "error" {
		t.Fatalf("fallback = %#v", fallback[0])
	}
}

func TestScriptTestSkipsPassingGate(t *testing.T) {
	u := plainUnit("me/script-test", "")
	u.Judge = &unit.Judge{Lang: "sh", Script: "exit 0\n"}
	cases := Test(u, profile.Defaults(), false)
	if cases[0].Result != "skipped" ||
		cases[0].Note != "脚本判断依赖真实环境，示例测试不运行脚本" {
		t.Fatalf("script case = %#v", cases[0])
	}
	if cases[1].Result != "pass" {
		t.Fatalf("gate miss = %#v", cases[1])
	}
}

func TestTestRechecksBlockRule12(t *testing.T) {
	u := judgedUnit("me/rule12")
	for index := range u.Examples {
		if u.Examples[index].Expect == "miss" {
			u.Examples[index].Event.Tool = "write"
		}
	}
	cases := Test(u, profile.Defaults(), true)
	if cases[0].Index != -1 || cases[0].Result != "fail" ||
		!strings.Contains(cases[0].Note, "规则 12") {
		t.Fatalf("structural case = %#v", cases[0])
	}
}

func TestSaveAndLoadTest(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	u := judgedUnit("me/save")
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	cases := []CaseResult{{Index: 0, Expect: "hit", Got: "hit", Result: "pass"}}
	if err := SaveTest(u, cases, now); err != nil {
		t.Fatal(err)
	}
	record, err := LoadTest(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if record == nil || record.Hash != u.Hash || !record.TS.Equal(now) ||
		len(record.Cases) != 1 {
		t.Fatalf("record = %#v", record)
	}
	missing, err := LoadTest("me/missing")
	if err != nil || missing != nil {
		t.Fatalf("missing = %#v, %v", missing, err)
	}
}

func TestReplayReportsTwoOfThreeHits(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	u := plainUnit("me/replay", "")
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	shells := []string{"git commit -m fix", "git status", "git commit -m docs"}
	for index, shell := range shells {
		record := journal.Record{
			TS: now.Add(time.Duration(index-3) * time.Minute),
			Event: event.Event{
				Harness: "fake", Kind: "tool", Tool: "shell", Shell: shell,
			},
		}
		if err := journal.Append(record, true); err != nil {
			t.Fatal(err)
		}
	}
	report, err := Replay(u, profile.Defaults(), time.Hour, false, now)
	if err != nil {
		t.Fatal(err)
	}
	if report.Judged != 3 || report.Unknown != 0 || len(report.Hits) != 2 {
		t.Fatalf("report = %#v", report)
	}
	if !strings.Contains(report.Hits[0].Summary, "git commit") {
		t.Fatalf("summary = %q", report.Hits[0].Summary)
	}
}

func TestReplayReusesScriptVerdictAndNeverRunsScript(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	marker := filepath.Join(t.TempDir(), "ran")
	u := plainUnit("me/script-replay", "")
	u.Judge = &unit.Judge{
		Lang: "sh", Script: "touch " + marker + "\nexit 0\n",
	}
	now := time.Now().UTC()
	records := []journal.Record{
		{
			TS: now.Add(-2 * time.Minute),
			Event: event.Event{
				Harness: "fake", Kind: "tool", Tool: "shell",
				Shell: "git commit -m reused", Cwd: t.TempDir(),
			},
			Units: []journal.UnitRecord{{
				ID: u.ID, Hash: u.Hash,
				Judge: &journal.JudgeRecord{Backend: "script", Hit: true},
			}},
		},
		{
			TS: now.Add(-time.Minute),
			Event: event.Event{
				Harness: "fake", Kind: "tool", Tool: "shell",
				Shell: "git commit -m unknown", Cwd: t.TempDir(),
			},
		},
	}
	for _, record := range records {
		if err := journal.Append(record, true); err != nil {
			t.Fatal(err)
		}
	}
	report, err := Replay(u, profile.Defaults(), time.Hour, true, now)
	if err != nil {
		t.Fatal(err)
	}
	if report.Judged != 1 || report.Unknown != 1 || len(report.Hits) != 1 {
		t.Fatalf("report = %#v", report)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("script ran during replay: %v", err)
	}
}

func TestConflictsConfirmedPotentialAndTopic(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	a := plainUnit("me/chinese", "commit 用中文")
	b := plainUnit("me/english", "commit 用英文")
	settings := fakeSettings(t, "evaluate-conflict", &fake.Backend{
		P: map[string]float64{"conflict": 0.9},
	})
	found := Conflicts(a, []*unit.Unit{a, b}, settings, nil)
	if len(found) != 1 || !found[0].Confirmed {
		t.Fatalf("confirmed conflicts = %#v", found)
	}
	formatted := FormatConflict(found[0], a.ID)
	if !strings.Contains(formatted, "可选处理：") ||
		!strings.Contains(formatted, "4. 两者其实不冲突") {
		t.Fatalf("formatted conflict = %q", formatted)
	}

	a.Topic, b.Topic = "commit-language", "commit-language"
	if found := Conflicts(a, []*unit.Unit{a, b}, settings, nil); len(found) != 0 {
		t.Fatalf("same-topic conflicts = %#v", found)
	}

	a.Topic, b.Topic = "", ""
	unavailable := fakeSettings(t, "evaluate-conflict-unavailable", &fake.Backend{
		Err: backend.ErrNotConfigured,
	})
	found = Conflicts(a, []*unit.Unit{a, b}, unavailable, nil)
	if len(found) != 1 || found[0].Confirmed {
		t.Fatalf("potential conflicts = %#v", found)
	}
	if !strings.HasPrefix(FormatConflict(found[0], a.ID), "可能冲突") {
		t.Fatalf("potential format = %q", FormatConflict(found[0], a.ID))
	}
}

func TestConflictsTreatScriptJudgesAsPossible(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "ran")
	a := plainUnit("me/script-a", "first")
	b := plainUnit("me/script-b", "second")
	a.Judge = &unit.Judge{Lang: "sh", Script: "touch " + marker + "\nexit 0\n"}
	b.Judge = &unit.Judge{Lang: "sh", Script: "touch " + marker + "\nexit 0\n"}
	found := Conflicts(a, []*unit.Unit{a, b}, profile.Defaults(), nil)
	if len(found) != 1 || found[0].Confirmed {
		t.Fatalf("script conflicts = %#v", found)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("script ran during conflict check: %v", err)
	}
}

func TestConflictsRejectsLowConflictProbability(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	a := plainUnit("me/chinese-low", "commit 用中文")
	b := plainUnit("me/english-low", "commit 用英文")
	settings := fakeSettings(t, "evaluate-no-conflict", &fake.Backend{
		P: map[string]float64{"conflict": 0.1},
	})
	if found := Conflicts(a, []*unit.Unit{a, b}, settings, nil); len(found) != 0 {
		t.Fatalf("low-probability conflicts = %#v", found)
	}
}

func TestConflictsUseJournalCoHitsBeyondExamples(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	a := plainUnit("me/commit-zh", "commit 用中文")
	b := plainUnit("team/commit-en", "commit 用英文")
	c := plainUnit("team/unrelated", "无关")
	b.Trigger.Shell = []string{`git\s+cm`}
	b.Examples[0].Event.Shell = "git cm -m fix"
	c.Trigger.Shell = b.Trigger.Shell
	c.Examples[0].Event.Shell = b.Examples[0].Event.Shell
	settings := fakeSettings(t, "evaluate-cohits", &fake.Backend{
		P: map[string]float64{"conflict": 0.9},
	})
	if found := Conflicts(a, []*unit.Unit{b}, settings, LoadCoHits()); len(found) != 0 {
		t.Fatalf("examples alone should not overlap: %#v", found)
	}

	now := time.Now().UTC()
	record := journal.Record{
		V: 1, TS: now,
		Event: event.Event{Kind: "tool", Tool: "shell", Shell: "git commit -m fix && git cm"},
		Units: []journal.UnitRecord{
			{ID: a.ID, Hash: "sha256:old"},
			{ID: b.ID, Judge: &journal.JudgeRecord{Hit: true, P: 0.9}},
			{ID: c.ID, Judge: &journal.JudgeRecord{Hit: false, P: 0.1}},
			{ID: c.ID, Gate: "content"},
		},
	}
	if err := journal.Append(record, true); err != nil {
		t.Fatal(err)
	}
	history := LoadCoHits()
	found := Conflicts(a, []*unit.Unit{b, c}, settings, history)
	if len(found) != 1 || found[0].B != b.ID || !found[0].Confirmed ||
		!strings.HasPrefix(found[0].Example, "真实事件 ") {
		t.Fatalf("co-hit conflicts = %#v", found)
	}
}

func judgedUnit(id string) *unit.Unit {
	u := plainUnit(id, "commit message 用中文")
	u.Judge = &unit.Judge{Question: "Is the commit message in English?"}
	u.Examples = append(u.Examples,
		unit.Example{
			Expect: "hit",
			Event:  event.Event{Kind: "tool", Tool: "shell", Shell: "git commit -m docs"},
		},
		unit.Example{
			Expect: "miss",
			Event:  event.Event{Kind: "tool", Tool: "shell", Shell: "git commit -m 文档"},
		},
	)
	return u
}

func plainUnit(id, body string) *unit.Unit {
	return &unit.Unit{
		ID: id, Hash: "sha256:" + id, Body: body, Title: id,
		Trigger: unit.Trigger{
			Events: []string{"tool"}, Tools: []string{"shell"},
			Shell: []string{`git\s+commit`},
		},
		Deliver: unit.Deliver{As: "block"},
		Examples: []unit.Example{
			{
				Expect: "hit",
				Event:  event.Event{Kind: "tool", Tool: "shell", Shell: "git commit -m fix"},
			},
			{
				Expect: "miss",
				Event:  event.Event{Kind: "tool", Tool: "shell", Shell: "git status"},
			},
		},
	}
}

func fakeSettings(t *testing.T, name string, implementation *fake.Backend) profile.Settings {
	t.Helper()
	implementation.BackendName = name
	backend.Register(implementation)
	settings := profile.Defaults()
	settings.Judge.Backend = name
	settings.Backends[name] = map[string]any{"threshold": 0.5, "model": "fake"}
	return settings
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}
