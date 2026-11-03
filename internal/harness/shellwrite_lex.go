package harness

import "strings"

type shellToken struct {
	kind           string
	text           string
	fd             string
	dynamic        bool
	quoted         bool
	start          int
	end            int
	heredoc        string
	heredocOK      bool
	heredocDynamic bool
}

type pendingHeredoc struct {
	token  int
	delim  string
	tabs   bool
	quoted bool
}

var shellOperators = []string{
	"&>>", "<<<", "<<-", "&>", ">>", ">|", ">&", "<<", "<&",
	"&&", "||", "|&", ">", "<", "|", ";", "&", "(", ")",
}

func lexShell(command string) ([]shellToken, bool) {
	var tokens []shellToken
	var pending []pendingHeredoc
	for i := 0; i < len(command); {
		if next, ok := skipShellSpace(command, i); ok {
			i = next
			continue
		}
		if command[i] == '\n' {
			tokens = append(tokens, shellToken{kind: "newline", start: i, end: i + 1})
			i++
			if !consumeHeredocs(command, &i, pending, tokens) {
				return nil, false
			}
			pending = nil
			if i > 0 && command[i-1] == '\n' {
				tokens = append(tokens, shellToken{kind: "newline", start: i - 1, end: i})
			}
			continue
		}
		if command[i] == '#' {
			i = skipComment(command, i)
			continue
		}
		if op := shellOperator(command[i:]); op != "" {
			if op == "(" || op == ")" {
				return nil, false
			}
			tokens = appendOperator(tokens, op, i)
			index := len(tokens) - 1
			i += len(op)
			if op == "<<" || op == "<<-" {
				var item pendingHeredoc
				tokens, i, item = lexHeredocStart(command, tokens, index, i, op == "<<-")
				if item.token < 0 {
					return nil, false
				}
				pending = append(pending, item)
			}
			continue
		}
		word, next, ok := lexShellWord(command, i)
		if !ok {
			return nil, false
		}
		tokens = append(tokens, word)
		i = next
	}
	if len(pending) != 0 {
		return nil, false
	}
	return tokens, true
}

func skipShellSpace(command string, i int) (int, bool) {
	start := i
	for i < len(command) {
		if command[i] == ' ' || command[i] == '\t' {
			i++
			continue
		}
		if command[i] == '\\' && i+1 < len(command) && command[i+1] == '\n' {
			i += 2
			continue
		}
		break
	}
	return i, i != start
}

func skipComment(command string, i int) int {
	for i < len(command) && command[i] != '\n' {
		i++
	}
	return i
}

func appendOperator(tokens []shellToken, op string, start int) []shellToken {
	token := shellToken{kind: "op", text: op, start: start, end: start + len(op)}
	if (strings.HasPrefix(op, ">") || strings.HasPrefix(op, "<")) && len(tokens) > 0 {
		previous := tokens[len(tokens)-1]
		if previous.kind == "word" && previous.end == start && !previous.quoted && digits(previous.text) {
			token.fd = previous.text
			tokens = tokens[:len(tokens)-1]
		}
	}
	return append(tokens, token)
}

func lexHeredocStart(command string, tokens []shellToken, opIndex, i int, tabs bool) ([]shellToken, int, pendingHeredoc) {
	for i < len(command) && (command[i] == ' ' || command[i] == '\t') {
		i++
	}
	if i >= len(command) || command[i] == '\n' {
		return tokens, i, pendingHeredoc{token: -1}
	}
	word, next, ok := lexShellWord(command, i)
	if !ok {
		return tokens, i, pendingHeredoc{token: -1}
	}
	tokens = append(tokens, word)
	return tokens, next, pendingHeredoc{token: opIndex, delim: word.text, tabs: tabs, quoted: word.quoted}
}

func consumeHeredocs(command string, i *int, pending []pendingHeredoc, tokens []shellToken) bool {
	for _, item := range pending {
		var body strings.Builder
		for {
			if *i > len(command) {
				return false
			}
			line, next, hasNewline := shellLine(command, *i)
			compare := line
			if item.tabs {
				compare = strings.TrimLeft(compare, "\t")
			}
			if compare == item.delim {
				*i = next
				tokens[item.token].heredoc = body.String()
				tokens[item.token].heredocOK = true
				tokens[item.token].heredocDynamic = !item.quoted &&
					strings.ContainsAny(body.String(), "$`")
				break
			}
			if *i == len(command) {
				return false
			}
			if item.tabs {
				line = strings.TrimLeft(line, "\t")
			}
			body.WriteString(line)
			body.WriteByte('\n')
			*i = next
			if !hasNewline {
				return false
			}
		}
	}
	return true
}

func shellLine(command string, start int) (string, int, bool) {
	if end := strings.IndexByte(command[start:], '\n'); end >= 0 {
		end += start
		return command[start:end], end + 1, true
	}
	return command[start:], len(command), false
}

func lexShellWord(command string, start int) (shellToken, int, bool) {
	token := shellToken{kind: "word", start: start}
	var value strings.Builder
	i := start
	for i < len(command) && !wordBoundary(command, i) {
		switch command[i] {
		case '\'':
			next, ok := lexSingleQuoted(command, i, &value)
			if !ok {
				return token, i, false
			}
			token.quoted = true
			i = next
		case '"':
			next, dynamic, ok := lexDoubleQuoted(command, i, &value)
			if !ok {
				return token, i, false
			}
			token.quoted = true
			token.dynamic = token.dynamic || dynamic
			i = next
		case '\\':
			token.quoted = true
			i = lexUnquotedEscape(command, i, &value)
		default:
			if unquotedDynamic(command[i], i == start) {
				token.dynamic = true
			}
			value.WriteByte(command[i])
			i++
		}
	}
	token.text = value.String()
	token.end = i
	return token, i, true
}

func wordBoundary(command string, i int) bool {
	if command[i] == ' ' || command[i] == '\t' || command[i] == '\n' {
		return true
	}
	return shellOperator(command[i:]) != ""
}

func lexSingleQuoted(command string, start int, value *strings.Builder) (int, bool) {
	end := strings.IndexByte(command[start+1:], '\'')
	if end < 0 {
		return start, false
	}
	end += start + 1
	value.WriteString(command[start+1 : end])
	return end + 1, true
}

func lexDoubleQuoted(command string, start int, value *strings.Builder) (int, bool, bool) {
	dynamic := false
	for i := start + 1; i < len(command); i++ {
		if command[i] == '"' {
			return i + 1, dynamic, true
		}
		if command[i] == '$' || command[i] == '`' {
			dynamic = true
		}
		if command[i] != '\\' || i+1 >= len(command) {
			value.WriteByte(command[i])
			continue
		}
		next := command[i+1]
		if strings.ContainsRune("$`\"\\", rune(next)) {
			value.WriteByte(next)
			i++
		} else if next == '\n' {
			i++
		} else {
			value.WriteByte('\\')
		}
	}
	return start, false, false
}

func lexUnquotedEscape(command string, i int, value *strings.Builder) int {
	if i+1 >= len(command) {
		value.WriteByte('\\')
		return i + 1
	}
	if command[i+1] != '\n' {
		value.WriteByte(command[i+1])
	}
	return i + 2
}

func unquotedDynamic(ch byte, first bool) bool {
	return ch == '$' || ch == '`' || ch == '*' || ch == '?' || ch == '[' || ch == '{' || first && ch == '~'
}

func shellOperator(value string) string {
	for _, operator := range shellOperators {
		if strings.HasPrefix(value, operator) {
			return operator
		}
	}
	return ""
}

func digits(value string) bool {
	if value == "" {
		return false
	}
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}
