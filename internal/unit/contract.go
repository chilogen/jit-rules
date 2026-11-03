// Package unit parses and validates judgment units.
package unit

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/chilogen/jit-rules/internal/event"
	"gopkg.in/yaml.v3"
)

var (
	shortIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{0,63}$`)
	topicPattern   = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*$`)
)

// Trigger defines the deterministic event filters for a unit.
type Trigger struct {
	Events  []string `yaml:"events"`
	Tools   []string `yaml:"tools"`
	Paths   []string `yaml:"paths"`
	Shell   []string `yaml:"shell"`
	Prompt  []string `yaml:"prompt"`
	Content []string `yaml:"content"`
}

// Judge defines the model question for a unit.
type Judge struct {
	Question string `yaml:"question"`
	Not      string `yaml:"not"`
	Backend  string `yaml:"backend"`
	Cache    *bool  `yaml:"cache"`
	Lang     string `yaml:"-"`
	Script   string `yaml:"-"`
}

// Deliver defines how a matching unit is delivered.
type Deliver struct {
	As string `yaml:"as"`
}

// Example is one executable unit example.
type Example struct {
	Expect string      `yaml:"expect"`
	Event  event.Event `yaml:"event"`
}

// Unit is a parsed judgment unit.
type Unit struct {
	ID       string    `yaml:"-"`
	Pack     string    `yaml:"-"`
	File     string    `yaml:"-"`
	Hash     string    `yaml:"-"`
	Body     string    `yaml:"-"`
	Title    string    `yaml:"title"`
	Trigger  Trigger   `yaml:"trigger"`
	Judge    *Judge    `yaml:"judge"`
	Deliver  Deliver   `yaml:"deliver"`
	Topic    string    `yaml:"topic"`
	Replaces []string  `yaml:"replaces"`
	Examples []Example `yaml:"examples"`
}

// ParseFile parses a unit from a file.
func ParseFile(path, pack string) (*Unit, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data, path, pack)
}

// Parse parses a unit from bytes.
func Parse(data []byte, file, pack string) (*Unit, error) {
	normalized := bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	if !bytes.HasPrefix(normalized, []byte("---\n")) {
		return nil, fmt.Errorf("unit file must start with ---")
	}
	end := bytes.Index(normalized[4:], []byte("\n---\n"))
	if end < 0 {
		return nil, fmt.Errorf("unit frontmatter must end with ---")
	}
	end += 4

	u := &Unit{}
	decoder := yaml.NewDecoder(bytes.NewReader(normalized[4:end]))
	decoder.KnownFields(true)
	if err := decoder.Decode(u); err != nil {
		return nil, fmt.Errorf("规则 1：frontmatter 无效：%w", err)
	}

	shortID := strings.TrimSuffix(filepath.Base(file), ".md")
	sum := sha256.Sum256(data)
	u.ID = pack + "/" + shortID
	u.Pack = pack
	u.File = file
	u.Hash = fmt.Sprintf("sha256:%x", sum)
	u.Body = strings.TrimSpace(string(normalized[end+5:]))
	// A malformed block stays in the body so Validate reports it as rule 15 with the other errors.
	if body, lang, script, err := parseScriptBlock(u.Body); err == nil && script != "" {
		u.Body = body
		if u.Judge == nil {
			u.Judge = &Judge{}
		}
		u.Judge.Lang = lang
		u.Judge.Script = script
	}
	if u.Deliver.As == "" {
		u.Deliver.As = "inject"
	}
	return u, nil
}

// Validate returns all validation errors and warnings.
func Validate(u *Unit) (errs []error, warns []string) {
	v := &validation{}
	if u == nil {
		v.addError(1, "单元不能为空")
		return v.errs, v.warns
	}
	v.rule1(u)
	v.rule2(u)
	v.rules3To7(u)
	v.rules8To13(u)
	v.rule14(u)
	v.rule15(u)
	return v.errs, v.warns
}

type validation struct {
	errs  []error
	warns []string
}

func (v *validation) addError(rule int, format string, args ...any) {
	v.errs = append(v.errs, fmt.Errorf("规则 %d：%s", rule, fmt.Sprintf(format, args...)))
}

func (v *validation) addWarning(rule int, text string) {
	v.warns = append(v.warns, fmt.Sprintf("规则 %d：%s", rule, text))
}

func (v *validation) rule1(u *Unit) {
	shortID := strings.TrimSuffix(filepath.Base(u.File), ".md")
	if !shortIDPattern.MatchString(shortID) {
		v.addError(1, "短 id %q 必须匹配 ^[a-z0-9][a-z0-9.-]{0,63}$", shortID)
	}
	if u.Title == "" {
		v.addError(1, "title 是必填字段")
	} else {
		if strings.ContainsAny(u.Title, "\r\n") {
			v.addError(1, "title 必须只有一行")
		}
		if utf8.RuneCountInString(u.Title) > 80 {
			v.addError(1, "title 不能超过 80 个字符")
		}
	}
	if len(u.Trigger.Events) == 0 {
		v.addError(1, "trigger.events 是必填字段")
	}
	for _, eventName := range u.Trigger.Events {
		if !isRuleEvent(eventName) {
			v.addError(1, "trigger.events 只能包含 prompt、tool、tool_result 或 stop，收到 %q", eventName)
		}
	}
	if u.Deliver.As != "inject" && u.Deliver.As != "block" {
		v.addError(1, "deliver.as 只能是 inject 或 block，收到 %q", u.Deliver.As)
	}
	if u.Topic != "" && !topicPattern.MatchString(u.Topic) {
		v.addError(1, "topic %q 必须匹配 ^[a-z0-9][a-z0-9.-]*$", u.Topic)
	}
	if len(u.Examples) == 0 {
		v.addError(1, "examples 是必填字段")
	}
	for i, example := range u.Examples {
		if example.Expect != "hit" && example.Expect != "miss" {
			v.addError(1, "examples[%d].expect 只能是 hit 或 miss", i)
		}
		if !isRuleEvent(example.Event.Kind) {
			v.addError(1, "examples[%d].event.kind 只能是 prompt、tool、tool_result 或 stop", i)
		}
	}
}

func isRuleEvent(name string) bool {
	return name == "prompt" || name == "tool" || name == "tool_result" || name == "stop"
}

func (v *validation) rule2(u *Unit) {
	for _, pattern := range append(append(
		append([]string{}, u.Trigger.Shell...),
		u.Trigger.Prompt...),
		u.Trigger.Content...) {
		if _, err := regexp.Compile(pattern); err != nil {
			v.addError(2, "正则 %q 无效：%v", pattern, err)
		}
	}
	for _, pattern := range append(append([]string{}, u.Trigger.Tools...), u.Trigger.Paths...) {
		if !doublestar.ValidatePattern(pattern) {
			v.addError(2, "glob %q 无效", pattern)
		}
	}
}

func (v *validation) rules3To7(u *Unit) {
	hasTool := contains(u.Trigger.Events, "tool")
	hasToolResult := contains(u.Trigger.Events, "tool_result")
	hasToolEvent := hasTool || hasToolResult
	hasPrompt := contains(u.Trigger.Events, "prompt")
	if !hasToolEvent {
		if len(u.Trigger.Tools) > 0 {
			v.addError(3, "events 不含 tool 或 tool_result 时不能写 trigger.tools")
		}
		if len(u.Trigger.Paths) > 0 {
			v.addError(3, "events 不含 tool 或 tool_result 时不能写 trigger.paths")
		}
		if len(u.Trigger.Shell) > 0 {
			v.addError(3, "events 不含 tool 或 tool_result 时不能写 trigger.shell")
		}
	}
	if !hasToolEvent && !contains(u.Trigger.Events, "stop") && len(u.Trigger.Content) > 0 {
		v.addError(3, "events 不含 tool、tool_result 或 stop 时不能写 trigger.content")
	}
	if !hasPrompt && len(u.Trigger.Prompt) > 0 {
		v.addError(4, "events 不含 prompt 时不能写 trigger.prompt")
	}
	if u.Deliver.As == "block" && !hasToolEvent {
		v.addError(5, "deliver.as 为 block 时 events 必须包含 tool 或 tool_result")
	}
	if u.Judge != nil && hasToolEvent && len(u.Trigger.Tools) == 0 && len(u.Trigger.Paths) == 0 {
		if u.Judge.Script != "" {
			v.addWarning(6, "写了脚本 judge 的 tool 单元应设置 tools 或 paths，否则每次工具调用都会运行脚本")
		} else {
			v.addWarning(6, "写了 judge 的 tool 单元应设置 tools 或 paths，否则每次工具调用都会联网判断")
		}
	}
	if len(u.Trigger.Tools) == 0 &&
		len(u.Trigger.Paths) == 0 &&
		len(u.Trigger.Shell) == 0 &&
		len(u.Trigger.Prompt) == 0 &&
		len(u.Trigger.Content) == 0 &&
		u.Judge == nil {
		v.addError(7, "除 events 外至少写一个 trigger 条件，或者写 judge")
	}
}

func (v *validation) rules8To13(u *Unit) {
	hits, misses := exampleCounts(u.Examples)
	if hits == 0 || misses == 0 {
		v.addError(8, "examples 必须同时包含 hit 和 miss")
	}
	if u.Body == "" {
		v.addError(9, "正文不能为空")
	}
	if utf8.RuneCountInString(u.Body) > 600 {
		v.addWarning(10, "正文超过 600 个字符")
	}
	if u.Judge != nil && u.Judge.Question != "" && !strings.HasSuffix(u.Judge.Question, "?") {
		v.addWarning(11, "judge.question 应以 ? 结尾")
	}
	if u.Deliver.As == "block" && !hasMatchingMissTool(u.Examples) {
		v.addError(12, "拦截单元需要一条'改对之后'的 miss 示例：和某条 hit 示例是同一个工具，只是内容合规")
	}
	toolEvent := contains(u.Trigger.Events, "tool") || contains(u.Trigger.Events, "tool_result")
	if u.Deliver.As == "inject" && len(u.Trigger.Content) > 0 && toolEvent {
		v.addWarning(13, "content 条件只能在内容写好之后判断，inject 只能让 AI 事后修正；要在写入之前纠正，请用 block")
	}
}

func (v *validation) rule15(u *Unit) {
	if u.Judge != nil && u.Judge.Script != "" {
		return
	}
	if _, _, _, err := parseScriptBlock(u.Body); err != nil {
		v.addError(15, "%s", err.Error())
	}
}

func (v *validation) rule14(u *Unit) {
	if u.Judge == nil {
		return
	}
	hasQuestion := u.Judge.Question != ""
	hasScript := u.Judge.Script != ""
	if hasQuestion == hasScript {
		v.addError(14, "judge.question 和脚本块必须且只能写一个")
	}
	if u.Judge.Not != "" && !hasQuestion {
		v.addError(14, "judge.not 只能和 judge.question 一起写")
	}
	if u.Judge.Backend != "" && !hasQuestion {
		v.addError(14, "judge.backend 只能和 judge.question 一起写")
	}
}

// Describe renders a unit in human-readable form.
// threshold applies only to a question judge; the caller passes Settings.ThresholdOf(the unit's backend).
func Describe(u *Unit, threshold float64) string {
	lines := []string{fmt.Sprintf("【%s】  %s", u.Title, u.ID)}
	if events := describeEvents(u.Trigger.Events); events != "" {
		lines = append(lines, "何时检查："+events)
	}
	if conditions := describeConditions(u.Trigger); conditions != "" {
		lines = append(lines, "条件："+conditions)
	}
	if judge := describeJudge(u.Judge, threshold); judge != "" {
		lines = append(lines, judge)
	}
	action := "在 AI 继续之前，把规则原文放进它的上下文"
	if onlyStop(u.Trigger.Events) {
		action = "在 AI 结束本轮之前提醒它一次，由它判断规则是否适用"
	}
	if u.Deliver.As == "block" {
		action = "拦下这次工具调用，把规则原文作为理由交给 AI，让它按规则改正后再试"
		if contains(u.Trigger.Events, "tool_result") && !contains(u.Trigger.Events, "tool") {
			action = "工具结果已经产生；阻止工作流继续，把规则原文交给 AI，要求先修复结果"
		}
	}
	lines = append(lines, "做什么："+action, "规则原文：")
	for _, line := range strings.Split(u.Body, "\n") {
		lines = append(lines, "  "+line)
	}
	if u.Topic != "" {
		lines = append(lines, "冲突域："+u.Topic+"（同一时刻只生效排位最高的一条）")
	}
	if len(u.Replaces) > 0 {
		lines = append(lines, "替换："+strings.Join(u.Replaces, "、"))
	}
	hits, misses := exampleCounts(u.Examples)
	lines = append(lines, fmt.Sprintf("示例：%d 条应触发，%d 条不应触发", hits, misses))
	return strings.Join(lines, "\n") + "\n"
}

// Cached resolves Judge.Cache against its default.
func (j *Judge) Cached() bool {
	if j == nil {
		return false
	}
	if j.Cache != nil {
		return *j.Cache
	}
	return j.Question != ""
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func hasMatchingMissTool(examples []Example) bool {
	hitTools := make(map[string]struct{})
	for _, example := range examples {
		if example.Expect == "hit" && example.Event.Tool != "" {
			hitTools[example.Event.Tool] = struct{}{}
		}
	}
	for _, example := range examples {
		if example.Expect == "miss" {
			if _, ok := hitTools[example.Event.Tool]; ok {
				return true
			}
		}
	}
	return false
}

func exampleCounts(examples []Example) (hits, misses int) {
	for _, example := range examples {
		if example.Expect == "hit" {
			hits++
		} else if example.Expect == "miss" {
			misses++
		}
	}
	return hits, misses
}
