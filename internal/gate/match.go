package gate

import (
	"regexp"
	"sync"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/chilogen/jit-rules/internal/event"
	"github.com/chilogen/jit-rules/internal/unit"
)

var regexpCache sync.Map

func match(u *unit.Unit, ev *event.Event) Outcome {
	toolValues := []string{ev.Tool}
	if ev.Effect != "" {
		toolValues = append(toolValues, ev.Effect)
	}
	checks := []struct {
		name     string
		patterns []string
		values   []string
		regexp   bool
	}{
		{"tools", u.Trigger.Tools, toolValues, false},
		{"paths", u.Trigger.Paths, ev.Paths, false},
		{"shell", u.Trigger.Shell, []string{ev.Shell}, true},
		{"prompt", u.Trigger.Prompt, []string{ev.Prompt}, true},
	}
	for _, check := range checks {
		if len(check.patterns) == 0 {
			continue
		}
		if !matchesAny(check.patterns, check.values, check.regexp) {
			return Outcome{Failed: check.name}
		}
	}
	if len(u.Trigger.Content) == 0 {
		return Outcome{OK: true}
	}
	if ev.ContentUnknown {
		return Outcome{OK: true, ContentUncertain: true}
	}
	if !matchesAny(u.Trigger.Content, []string{ev.Content}, true) {
		return Outcome{Failed: "content"}
	}
	return Outcome{OK: true}
}

func matchesAny(patterns, values []string, useRegexp bool) bool {
	for _, pattern := range patterns {
		for _, value := range values {
			if useRegexp {
				if regexpMatches(pattern, value) {
					return true
				}
				continue
			}
			if ok, err := doublestar.Match(pattern, value); err == nil && ok {
				return true
			}
		}
	}
	return false
}

func regexpMatches(pattern, value string) bool {
	cached, ok := regexpCache.Load(pattern)
	if !ok {
		compiled, err := regexp.Compile(pattern)
		if err != nil {
			regexpCache.Store(pattern, (*regexp.Regexp)(nil))
			return false
		}
		cached, _ = regexpCache.LoadOrStore(pattern, compiled)
	}
	re, _ := cached.(*regexp.Regexp)
	return re != nil && re.MatchString(value)
}
