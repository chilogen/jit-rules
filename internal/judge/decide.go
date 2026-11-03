package judge

import (
	"context"
	"errors"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/chilogen/jit-rules/internal/backend"
	"github.com/chilogen/jit-rules/internal/event"
	"github.com/chilogen/jit-rules/internal/profile"
	"github.com/chilogen/jit-rules/internal/unit"
)

type pendingQuestion struct {
	unitID   string
	question backend.Question
	cacheKey string
	cache    bool
}

func decide(ctx context.Context, ev *event.Event, units []*unit.Unit, settings profile.Settings) map[string]Verdict {
	verdicts := make(map[string]Verdict, len(units))
	if len(units) == 0 {
		return verdicts
	}

	state := eventState(ev, settings.Judge)
	stateJSON := marshalState(state)
	groups, scripts := routeUnits(units, settings.Judge.Backend)
	var mu sync.Mutex
	var wait sync.WaitGroup
	for name, grouped := range groups {
		wait.Add(1)
		go func() {
			defer wait.Done()
			result := decideGroup(ctx, grouped, name, state, stateJSON, settings)
			mergeVerdicts(verdicts, result, &mu)
		}()
	}
	for _, judgedUnit := range scripts {
		wait.Add(1)
		go func() {
			defer wait.Done()
			verdict := decideScript(ctx, ev, judgedUnit, stateJSON)
			mu.Lock()
			verdicts[judgedUnit.ID] = verdict
			mu.Unlock()
		}()
	}
	wait.Wait()
	return verdicts
}

func routeUnits(units []*unit.Unit, defaultBackend string) (map[string][]*unit.Unit, []*unit.Unit) {
	groups := make(map[string][]*unit.Unit)
	var scripts []*unit.Unit
	for _, judgedUnit := range units {
		if judgedUnit.Judge.Script != "" {
			scripts = append(scripts, judgedUnit)
			continue
		}
		name := judgedUnit.Judge.Backend
		if name == "" {
			name = defaultBackend
		}
		groups[name] = append(groups[name], judgedUnit)
	}
	return groups, scripts
}

func decideGroup(
	ctx context.Context,
	units []*unit.Unit,
	name string,
	state map[string]any,
	stateJSON []byte,
	settings profile.Settings,
) map[string]Verdict {
	verdicts := make(map[string]Verdict, len(units))
	judgeBackend, ok := backend.Get(name)
	if !ok {
		for _, judgedUnit := range units {
			verdicts[judgedUnit.ID] = Verdict{
				Unconfigured: true,
				Backend:      name,
				Err:          "unknown backend",
			}
		}
		return verdicts
	}

	cfg := settings.Backends[name]
	model, _ := cfg["model"].(string)
	threshold := settings.ThresholdOf(name)
	pending := collectPending(units, name, model, stateJSON, threshold, verdicts)
	if len(pending) == 0 {
		return verdicts
	}

	callCtx, cancel := context.WithTimeout(ctx, time.Duration(settings.Judge.TimeoutMs)*time.Millisecond)
	defer cancel()
	started := time.Now()
	result, err := judgeBackend.Judge(callCtx, state, pendingQuestions(pending), cfg)
	elapsedMs := int(time.Since(started).Milliseconds())
	applyResult(pending, result, err, name, elapsedMs, threshold, verdicts)
	return verdicts
}

func mergeVerdicts(dst, src map[string]Verdict, mu *sync.Mutex) {
	mu.Lock()
	defer mu.Unlock()
	for id, verdict := range src {
		dst[id] = verdict
	}
}

func collectPending(
	units []*unit.Unit,
	name, model string,
	stateJSON []byte,
	threshold float64,
	verdicts map[string]Verdict,
) []pendingQuestion {
	pending := make([]pendingQuestion, 0, len(units))
	for _, judgedUnit := range units {
		question := backend.Question{
			ID:   judgedUnit.ID,
			Text: judgedUnit.Judge.Question,
			Not:  judgedUnit.Judge.Not,
		}
		key := cacheKey(name, model, question, stateJSON)
		useCache := judgedUnit.Judge.Cached()
		if value, ok := cached(key); useCache && ok {
			verdicts[judgedUnit.ID] = Verdict{
				Hit:     value.P >= threshold,
				P:       value.P,
				Backend: name,
				Model:   value.Model,
			}
			continue
		}
		pending = append(pending, pendingQuestion{
			unitID:   judgedUnit.ID,
			question: question,
			cacheKey: key,
			cache:    useCache,
		})
	}
	return pending
}

func pendingQuestions(pending []pendingQuestion) []backend.Question {
	questions := make([]backend.Question, len(pending))
	for index, item := range pending {
		questions[index] = item.question
	}
	return questions
}

func applyResult(
	pending []pendingQuestion,
	result backend.Result,
	err error,
	name string,
	elapsedMs int,
	threshold float64,
	verdicts map[string]Verdict,
) {
	if err != nil {
		for _, item := range pending {
			verdicts[item.unitID] = errorVerdict(err, result, name, elapsedMs)
		}
		return
	}
	for _, item := range pending {
		probability, ok := result.P[item.question.ID]
		if !ok {
			verdicts[item.unitID] = Verdict{
				Hit:      true,
				Fallback: true,
				Backend:  name,
				Model:    result.Model,
				ConnMs:   result.ConnMs,
				Ms:       elapsedMs,
				Err:      "missing",
			}
			continue
		}
		if item.cache {
			value := cacheValue{P: probability, Model: result.Model}
			storeCached(item.cacheKey, value)
		}
		verdicts[item.unitID] = Verdict{
			Hit:     probability >= threshold,
			P:       probability,
			Backend: name,
			Model:   result.Model,
			ConnMs:  result.ConnMs,
			Ms:      elapsedMs,
		}
	}
}

func errorVerdict(err error, result backend.Result, name string, elapsedMs int) Verdict {
	verdict := Verdict{
		Backend: name,
		Model:   result.Model,
		ConnMs:  result.ConnMs,
		Ms:      elapsedMs,
		Err:     err.Error(),
	}
	if errors.Is(err, backend.ErrNotConfigured) {
		verdict.Unconfigured = true
	} else {
		verdict.Hit = true
		verdict.Fallback = true
	}
	return verdict
}

func eventState(ev *event.Event, settings profile.JudgeSettings) map[string]any {
	state := make(map[string]any)
	addString(state, "kind", ev.Kind, settings.MaxFieldChars)
	addString(state, "tool", ev.Tool, settings.MaxFieldChars)
	addString(state, "raw_tool", ev.RawTool, settings.MaxFieldChars)
	addString(state, "effect", ev.Effect, settings.MaxFieldChars)
	if len(ev.Paths) > 0 {
		state["paths"] = append([]string(nil), ev.Paths...)
	}
	if settings.SendContent {
		addString(state, "shell", ev.Shell, settings.MaxFieldChars)
		addString(state, "content", ev.Content, settings.MaxFieldChars)
	}
	addString(state, "prompt", ev.Prompt, settings.MaxFieldChars)
	return state
}

func addString(state map[string]any, key, value string, limit int) {
	if value == "" {
		return
	}
	value = truncateRunes(value, limit)
	if value != "" {
		state[key] = value
	}
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit])
}
