package harness

import (
	"path/filepath"
	"strings"
)

type shellInput struct {
	text    string
	set     bool
	dynamic bool
}

type writeTarget struct {
	path    string
	append  bool
	content string
	unknown bool
}

func analyzeShell(command, cwd, root string, exists func(string) bool) Extracted {
	tokens, ok := lexShell(command)
	if !ok {
		return Extracted{}
	}
	var targets []writeTarget
	for _, segment := range splitShell(tokens) {
		found, valid := analyzeShellSegment(segment)
		if !valid {
			return Extracted{}
		}
		targets = append(targets, found...)
	}
	if len(targets) == 0 {
		return Extracted{}
	}
	paths := make([]string, 0, len(targets))
	for _, target := range targets {
		paths = append(paths, normalizePath(target.path, cwd, root))
	}
	result := Extracted{Paths: unique(paths)}
	if len(targets) > 1 {
		result.Effect = "edit"
		result.ContentUnknown = true
		return result
	}
	target := targets[0]
	absolute := target.path
	if !filepath.IsAbs(absolute) {
		absolute = filepath.Join(cwd, absolute)
	}
	result.Effect = "write"
	if target.append && exists(filepath.Clean(absolute)) {
		result.Effect = "edit"
	}
	result.Content = target.content
	result.ContentUnknown = target.unknown
	return result
}

func splitShell(tokens []shellToken) [][]shellToken {
	var segments [][]shellToken
	start := 0
	for i, token := range tokens {
		if token.kind == "newline" || token.kind == "op" && isSeparator(token.text) {
			if i > start {
				segments = append(segments, tokens[start:i])
			}
			start = i + 1
		}
	}
	if start < len(tokens) {
		segments = append(segments, tokens[start:])
	}
	return segments
}

func isSeparator(operator string) bool {
	switch operator {
	case "&&", "||", "|", "|&", ";", "&":
		return true
	default:
		return false
	}
}

func analyzeShellSegment(tokens []shellToken) ([]writeTarget, bool) {
	var args []shellToken
	var input shellInput
	var redirects []writeTarget
	for i := 0; i < len(tokens); i++ {
		token := tokens[i]
		if token.kind == "word" {
			args = append(args, token)
			continue
		}
		next, ok := redirectionWord(tokens, i)
		if !ok {
			return nil, false
		}
		switch token.text {
		case "<<", "<<-":
			input = shellInput{text: token.heredoc, set: token.heredocOK, dynamic: token.heredocDynamic}
		case "<<<":
			input = shellInput{text: next.text + "\n", set: true, dynamic: next.dynamic}
		case ">", ">|", ">>":
			if token.fd == "" || token.fd == "1" {
				redirects = appendTarget(redirects, next, token.text == ">>", false)
			}
		case "&>", "&>>":
			redirects = appendTarget(redirects, next, token.text == "&>>", true)
		}
		i++
	}
	content, known := shellStdout(args, input)
	for i := range redirects {
		if !redirects[i].unknown {
			redirects[i].content = content
			redirects[i].unknown = !known
		}
	}
	return append(redirects, teeTargets(args, input)...), true
}

func redirectionWord(tokens []shellToken, i int) (shellToken, bool) {
	if i+1 >= len(tokens) || tokens[i+1].kind != "word" {
		return shellToken{}, false
	}
	return tokens[i+1], true
}

func appendTarget(targets []writeTarget, word shellToken, appendMode, unknown bool) []writeTarget {
	if word.dynamic || ignoredTarget(word.text) {
		return targets
	}
	return append(targets, writeTarget{path: word.text, append: appendMode, unknown: unknown})
}

func ignoredTarget(path string) bool {
	return strings.HasPrefix(path, "/dev/") ||
		strings.HasPrefix(path, "/proc/") ||
		strings.HasPrefix(path, "/sys/")
}

func shellStdout(args []shellToken, input shellInput) (string, bool) {
	if len(args) == 0 {
		return "", false
	}
	switch args[0].text {
	case "cat":
		if len(args) == 1 && input.set && !input.dynamic {
			return input.text, true
		}
	case "echo":
		return echoOutput(args[1:])
	case "printf":
		return printfOutput(args[1:])
	}
	return "", false
}

func echoOutput(args []shellToken) (string, bool) {
	newline := true
	escapes := false
	index := 0
	for index < len(args) && echoOption(args[index].text) {
		for _, option := range args[index].text[1:] {
			if option == 'n' {
				newline = false
			} else if option == 'e' {
				escapes = true
			} else if option == 'E' {
				escapes = false
			}
		}
		index++
	}
	for _, arg := range args[index:] {
		if arg.dynamic {
			return "", false
		}
	}
	texts := make([]string, 0, len(args)-index)
	for _, arg := range args[index:] {
		texts = append(texts, arg.text)
	}
	result := strings.Join(texts, " ")
	if escapes {
		result = shellEscapes(result)
	}
	if newline {
		result += "\n"
	}
	return result, true
}

func echoOption(value string) bool {
	if len(value) < 2 || value[0] != '-' {
		return false
	}
	for _, ch := range value[1:] {
		if ch != 'n' && ch != 'e' && ch != 'E' {
			return false
		}
	}
	return true
}

func printfOutput(args []shellToken) (string, bool) {
	if len(args) == 0 || args[0].dynamic {
		return "", false
	}
	for _, arg := range args[1:] {
		if arg.dynamic {
			return "", false
		}
	}
	format := shellEscapes(args[0].text)
	if !strings.Contains(format, "%") {
		return format, len(args) == 1
	}
	if format == "%s" {
		var result strings.Builder
		for _, arg := range args[1:] {
			result.WriteString(arg.text)
		}
		return result.String(), true
	}
	if format == "%s\n" {
		var result strings.Builder
		if len(args) == 1 {
			return "\n", true
		}
		for _, arg := range args[1:] {
			result.WriteString(arg.text)
			result.WriteByte('\n')
		}
		return result.String(), true
	}
	return "", false
}

func shellEscapes(value string) string {
	var result strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] != '\\' || i+1 >= len(value) {
			result.WriteByte(value[i])
			continue
		}
		switch value[i+1] {
		case 'n':
			result.WriteByte('\n')
		case 't':
			result.WriteByte('\t')
		case '\\':
			result.WriteByte('\\')
		default:
			result.WriteByte('\\')
			result.WriteByte(value[i+1])
		}
		i++
	}
	return result.String()
}

func teeTargets(args []shellToken, input shellInput) []writeTarget {
	if len(args) == 0 || args[0].text != "tee" {
		return nil
	}
	appendMode := false
	for _, arg := range args[1:] {
		if arg.text == "-a" || arg.text == "--append" {
			appendMode = true
		}
	}
	var targets []writeTarget
	for _, arg := range args[1:] {
		if arg.text == "-a" || arg.text == "--append" {
			continue
		}
		if strings.HasPrefix(arg.text, "-") || arg.dynamic || ignoredTarget(arg.text) {
			continue
		}
		targets = append(targets, writeTarget{
			path: arg.text, append: appendMode, content: input.text,
			unknown: !input.set || input.dynamic,
		})
	}
	return targets
}
