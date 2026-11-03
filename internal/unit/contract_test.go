package unit

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseAndValidateRequiredUnits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path string
		pack string
		id   string
	}{
		{"../../assets/packs/core/units/author.md", "core", "core/author"},
		{"../../assets/packs/core/units/learn.md", "core", "core/learn"},
		{"../../assets/unit-template.md", "me", "me/unit-template"},
		{"../../testdata/smoke/profile/units/md.first-line-h1.md", "smoke", "smoke/md.first-line-h1"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(filepath.Base(tt.path), func(t *testing.T) {
			t.Parallel()
			data, err := os.ReadFile(tt.path)
			if err != nil {
				t.Fatal(err)
			}
			u, err := ParseFile(tt.path, tt.pack)
			if err != nil {
				t.Fatalf("ParseFile() error = %v", err)
			}
			if u.ID != tt.id {
				t.Errorf("ID = %q, want %q", u.ID, tt.id)
			}
			if u.Pack != tt.pack {
				t.Errorf("Pack = %q, want %q", u.Pack, tt.pack)
			}
			if u.File != tt.path {
				t.Errorf("File = %q, want %q", u.File, tt.path)
			}
			wantHash := fmt.Sprintf("sha256:%x", sha256.Sum256(data))
			if u.Hash != wantHash {
				t.Errorf("Hash = %q, want %q", u.Hash, wantHash)
			}
			errs, warns := Validate(u)
			if len(errs) != 0 || len(warns) != 0 {
				t.Fatalf("Validate() = (%v, %v), want no diagnostics", errs, warns)
			}
		})
	}
}

func TestValidateAndDescribeToolResultUnit(t *testing.T) {
	t.Parallel()
	data := []byte(`---
title: post-write check
trigger:
  events: [tool_result]
  tools: [write, edit]
  paths: ["**/*.go"]
deliver:
  as: block
examples:
  - expect: hit
    event: {kind: tool_result, tool: edit, paths: [large.go]}
  - expect: miss
    event: {kind: tool_result, tool: edit, paths: [small.go]}
---
Fix the generated file before continuing.
`)
	u, err := Parse(data, "post-write.md", "test")
	if err != nil {
		t.Fatal(err)
	}
	errs, warns := Validate(u)
	if len(errs) != 0 || len(warns) != 0 {
		t.Fatalf("Validate() = (%v, %v), want no diagnostics", errs, warns)
	}
	got := Describe(u, 0.5)
	if !strings.Contains(got, "AI 工具执行之后") ||
		!strings.Contains(got, "工具结果已经产生；阻止工作流继续") {
		t.Fatalf("Describe() =\n%s", got)
	}
}

func TestValidateRuleFixtures(t *testing.T) {
	t.Parallel()

	errorRules := []int{1, 2, 3, 4, 5, 7, 8, 9, 12}
	for _, rule := range errorRules {
		rule := rule
		t.Run(fmt.Sprintf("error-%02d", rule), func(t *testing.T) {
			t.Parallel()
			u := parseFixture(t, fmt.Sprintf("rule-%02d.md", rule))
			errs, warns := Validate(u)
			if len(errs) != 1 {
				t.Fatalf("errors = %v, want exactly one", errs)
			}
			wantPrefix := fmt.Sprintf("规则 %d：", rule)
			if !strings.HasPrefix(errs[0].Error(), wantPrefix) {
				t.Errorf("error = %q, want prefix %q", errs[0], wantPrefix)
			}
			if len(warns) != 0 {
				t.Errorf("warnings = %v, want none", warns)
			}
		})
	}

	warningRules := []int{6, 10, 11, 13}
	for _, rule := range warningRules {
		rule := rule
		t.Run(fmt.Sprintf("warning-%02d", rule), func(t *testing.T) {
			t.Parallel()
			u := parseFixture(t, fmt.Sprintf("rule-%02d.md", rule))
			errs, warns := Validate(u)
			if len(errs) != 0 {
				t.Fatalf("errors = %v, want none", errs)
			}
			if len(warns) != 1 {
				t.Fatalf("warnings = %v, want exactly one", warns)
			}
			wantPrefix := fmt.Sprintf("规则 %d：", rule)
			if !strings.HasPrefix(warns[0], wantPrefix) {
				t.Errorf("warning = %q, want prefix %q", warns[0], wantPrefix)
			}
		})
	}
}

func TestParseRejectsUnknownFrontmatterFields(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"frontmatter-id.md", "frontmatter-tirgger.md"} {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseFile(filepath.Join("../../testdata/units", name), "test")
			if err == nil {
				t.Fatal("ParseFile() error = nil, want unknown-field error")
			}
			if !strings.HasPrefix(err.Error(), "规则 1：") {
				t.Errorf("error = %q, want rule 1 prefix", err)
			}
		})
	}
}

func TestValidateRejectsInvalidShortID(t *testing.T) {
	t.Parallel()

	u := parseFixture(t, "Bad_Name.md")
	errs, warns := Validate(u)
	if len(errs) != 1 || !strings.HasPrefix(errs[0].Error(), "规则 1：") {
		t.Fatalf("Validate() errors = %v, want one rule 1 error", errs)
	}
	if len(warns) != 0 {
		t.Errorf("Validate() warnings = %v, want none", warns)
	}
}

func TestParseCRLF(t *testing.T) {
	t.Parallel()

	path := "../../testdata/units/crlf.md"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("\r\n")) {
		t.Fatal("fixture does not contain CRLF line endings")
	}
	u, err := Parse(data, path, "test")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if strings.ContainsRune(u.Body, '\r') {
		t.Errorf("Body contains carriage return: %q", u.Body)
	}
	if u.Body != "line one\nline two" {
		t.Errorf("Body = %q, want normalized and trimmed body", u.Body)
	}
	wantHash := fmt.Sprintf("sha256:%x", sha256.Sum256(data))
	if u.Hash != wantHash {
		t.Errorf("Hash = %q, want hash of original bytes %q", u.Hash, wantHash)
	}
	errs, warns := Validate(u)
	if len(errs) != 0 || len(warns) != 0 {
		t.Errorf("Validate() = (%v, %v), want no diagnostics", errs, warns)
	}
}

func TestParseScriptJudge(t *testing.T) {
	t.Parallel()

	data := unitData("", "text before\n```sh jitr-judge\necho one\necho two\n```\ntext after")
	u, err := Parse(data, "script.md", "test")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if u.Judge == nil {
		t.Fatal("Judge = nil, want script judge")
	}
	if u.Judge.Lang != "sh" {
		t.Errorf("Judge.Lang = %q, want sh", u.Judge.Lang)
	}
	if u.Judge.Script != "echo one\necho two\n" {
		t.Errorf("Judge.Script = %q, want lines ending in newlines", u.Judge.Script)
	}
	if u.Body != "text before\ntext after" {
		t.Errorf("Body = %q, want script block removed", u.Body)
	}
}

func TestParseScriptFenceVariants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "four backticks",
			body: "````sh jitr-judge\necho before\n```\necho after\n````",
			want: "echo before\n```\necho after\n",
		},
		{
			name: "tildes",
			body: "~~~bash jitr-judge\necho tilde\n~~~",
			want: "echo tilde\n",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			u, err := Parse(unitData("", tt.body), "script.md", "test")
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if u.Judge == nil || u.Judge.Script != tt.want {
				t.Errorf("Judge = %#v, want script %q", u.Judge, tt.want)
			}
		})
	}
}

func TestParseOrdinaryFenceStaysInBody(t *testing.T) {
	t.Parallel()

	bodies := []string{
		"```python\nprint('ordinary')\n```",
		"````md\n```sh jitr-judge\ntrue\n```\n````",
	}
	for _, body := range bodies {
		body := body
		t.Run(body[:4], func(t *testing.T) {
			t.Parallel()
			u, err := Parse(unitData("", body), "ordinary.md", "test")
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if u.Judge != nil {
				t.Errorf("Judge = %#v, want nil", u.Judge)
			}
			if u.Body != body {
				t.Errorf("Body = %q, want %q", u.Body, body)
			}
		})
	}
}

func TestParseScriptCRLF(t *testing.T) {
	t.Parallel()

	lf := unitData("", "before\n```sh jitr-judge\necho ok\n```\nafter")
	crlf := bytes.ReplaceAll(lf, []byte("\n"), []byte("\r\n"))
	lfUnit, err := Parse(lf, "script.md", "test")
	if err != nil {
		t.Fatalf("Parse(LF) error = %v", err)
	}
	crlfUnit, err := Parse(crlf, "script.md", "test")
	if err != nil {
		t.Fatalf("Parse(CRLF) error = %v", err)
	}
	if crlfUnit.Body != lfUnit.Body || crlfUnit.Judge.Lang != lfUnit.Judge.Lang ||
		crlfUnit.Judge.Script != lfUnit.Judge.Script {
		t.Errorf("CRLF parse = %#v, want body and judge matching LF parse %#v", crlfUnit, lfUnit)
	}
	wantHash := fmt.Sprintf("sha256:%x", sha256.Sum256(crlf))
	if crlfUnit.Hash != wantHash {
		t.Errorf("Hash = %q, want original-byte hash %q", crlfUnit.Hash, wantHash)
	}
}

func TestValidateRule14(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		judge string
		body  string
	}{
		{
			name:  "question and script",
			judge: "judge:\n  question: Does this match?",
			body:  "body\n```sh jitr-judge\ntrue\n```",
		},
		{
			name:  "neither",
			judge: "judge: {}",
			body:  "body",
		},
		{
			name:  "not without question",
			judge: "judge:\n  not: excluded",
			body:  "body",
		},
		{
			name:  "backend with script",
			judge: "judge:\n  backend: openai",
			body:  "body\n```sh jitr-judge\ntrue\n```",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			u, err := Parse(unitData(tt.judge, tt.body), "rule-14.md", "test")
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			errs, _ := Validate(u)
			if !hasDiagnosticPrefix(errs, "规则 14：") {
				t.Errorf("Validate() errors = %v, want rule 14 error", errs)
			}
		})
	}
}

func TestValidateRejectsInvalidScriptBlocksWithRule15(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
	}{
		{"two blocks", "```sh jitr-judge\ntrue\n```\n```sh jitr-judge\ntrue\n```"},
		{"unclosed", "```sh jitr-judge\ntrue"},
		{"no interpreter", "```jitr-judge\ntrue\n```"},
		{"empty", "```sh jitr-judge\n```"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			u, err := Parse(unitData("", tt.body), "rule-15.md", "test")
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			errs, _ := Validate(u)
			if !hasDiagnosticPrefix(errs, "规则 15：") {
				t.Errorf("Validate() errors = %v, want rule 15 error", errs)
			}
		})
	}
}

func TestValidateScriptJudgeRuleChanges(t *testing.T) {
	t.Parallel()

	u, err := Parse(
		unitData("", "body\n```sh jitr-judge\ntrue\n```"),
		"script.md",
		"test",
	)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	u.Trigger.Events = []string{"tool"}
	errs, warns := Validate(u)
	if len(errs) != 0 {
		t.Errorf("Validate() errors = %v, want no rule 7 or other errors", errs)
	}
	if len(warns) != 1 || !strings.HasPrefix(warns[0], "规则 6：") ||
		!strings.Contains(warns[0], "运行脚本") {
		t.Errorf("Validate() warnings = %v, want rule 6 warning about running script", warns)
	}
	for _, warning := range warns {
		if strings.HasPrefix(warning, "规则 11：") {
			t.Errorf("Validate() warnings = %v, want no rule 11 warning", warns)
		}
	}
}

func TestParseRejectsInvalidCacheType(t *testing.T) {
	t.Parallel()

	_, err := Parse(
		unitData("judge:\n  question: Does this match?\n  cache: maybe", "body"),
		"cache.md",
		"test",
	)
	if err == nil || !strings.HasPrefix(err.Error(), "规则 1：") {
		t.Errorf("Parse() error = %v, want rule 1 type error", err)
	}
}

func TestDescribeJudges(t *testing.T) {
	t.Parallel()

	script, err := Parse(
		unitData("judge:\n  cache: true", "body\n```sh jitr-judge\necho one\necho two\n```"),
		"script.md",
		"test",
	)
	if err != nil {
		t.Fatalf("Parse(script) error = %v", err)
	}
	got := Describe(script, 0.99)
	want := "判断：在项目目录用 sh 运行下面的脚本，退出码为 0 才算命中（结果会缓存）\n" +
		"  echo one\n  echo two\n"
	if !strings.Contains(got, want) {
		t.Errorf("Describe(script) =\n%s\nwant judgment section\n%s", got, want)
	}
	if strings.Contains(got, "概率 ≥") {
		t.Errorf("Describe(script) contains threshold:\n%s", got)
	}

	question, err := Parse(
		unitData("judge:\n  question: Does this match?\n  backend: openai", "body"),
		"question.md",
		"test",
	)
	if err != nil {
		t.Fatalf("Parse(question) error = %v", err)
	}
	if got := Describe(question, 0.3); !strings.Contains(
		got,
		`判断：由模型 openai 回答"Does this match?"，概率 ≥ 0.3 才算命中`,
	) {
		t.Errorf("Describe(question) =\n%s\nwant named backend", got)
	}
}

func TestDescribeGolden(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		unit   string
		pack   string
		golden string
	}{
		{
			name:   "author",
			unit:   "../../assets/packs/core/units/author.md",
			pack:   "core",
			golden: "../../testdata/describe/author.txt",
		},
		{
			name:   "template",
			unit:   "../../assets/unit-template.md",
			pack:   "me",
			golden: "../../testdata/describe/template.txt",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			u, err := ParseFile(tt.unit, tt.pack)
			if err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(tt.golden)
			if err != nil {
				t.Fatal(err)
			}
			if got := Describe(u, 0.3); got != string(want) {
				t.Errorf("Describe() mismatch\n--- got ---\n%s--- want ---\n%s", got, want)
			}
		})
	}
}

func parseFixture(t *testing.T, name string) *Unit {
	t.Helper()
	u, err := ParseFile(filepath.Join("../../testdata/units", name), "test")
	if err != nil {
		t.Fatalf("ParseFile(%q) error = %v", name, err)
	}
	return u
}

func unitData(judge, body string) []byte {
	if judge != "" {
		judge += "\n"
	}
	return []byte(fmt.Sprintf(`---
title: test unit
trigger:
  events: [prompt]
%sexamples:
  - expect: hit
    event: {kind: prompt}
  - expect: miss
    event: {kind: prompt}
---
%s`, judge, body))
}

func hasDiagnosticPrefix(errs []error, prefix string) bool {
	for _, err := range errs {
		if strings.HasPrefix(err.Error(), prefix) {
			return true
		}
	}
	return false
}

func TestValidateReportsRule15WithOtherErrors(t *testing.T) {
	t.Parallel()

	data := []byte("---\ntitle: t\ntrigger:\n  events: [tool]\n  tools: [shell]\nexamples:\n  - expect: hit\n    event: {kind: tool, tool: shell}\n---\nbody\n```sh jitr-judge\ntrue\n")
	u, err := Parse(data, "rule-15-mixed.md", "test")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	errs, _ := Validate(u)
	if !hasDiagnosticPrefix(errs, "规则 8：") || !hasDiagnosticPrefix(errs, "规则 15：") {
		t.Errorf("Validate() errors = %v, want both rule 8 and rule 15", errs)
	}
}
