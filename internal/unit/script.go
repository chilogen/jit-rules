package unit

import (
	"fmt"
	"strings"
)

func parseScriptBlock(body string) (string, string, string, error) {
	lines := strings.SplitAfter(body, "\n")
	blockStart := -1
	blockEnd := -1
	lang := ""
	script := ""

	for i := 0; i < len(lines); {
		text := strings.TrimSuffix(lines[i], "\n")
		fence, info, ok := openingFence(text)
		if !ok {
			i++
			continue
		}

		end := findClosingFence(lines, i+1, fence[0], len(fence))
		isScript := contains(strings.Fields(info), "jitr-judge")
		if end < 0 {
			if isScript {
				return "", "", "", fmt.Errorf("jitr-judge 脚本块没有闭合")
			}
			break
		}
		if !isScript {
			i = end + 1
			continue
		}
		if blockStart >= 0 {
			return "", "", "", fmt.Errorf("只能写一个 jitr-judge 脚本块")
		}
		fields := strings.Fields(info)
		if len(fields) == 0 || fields[0] == "jitr-judge" {
			return "", "", "", fmt.Errorf("jitr-judge 前必须写解释器")
		}

		blockStart = i
		blockEnd = end
		lang = fields[0]
		var scriptLines strings.Builder
		for j := i + 1; j < end; j++ {
			scriptLines.WriteString(strings.TrimSuffix(lines[j], "\n"))
			scriptLines.WriteByte('\n')
		}
		script = scriptLines.String()
		if strings.TrimSpace(script) == "" {
			return "", "", "", fmt.Errorf("jitr-judge 脚本不能为空")
		}
		i = end + 1
	}

	if blockStart < 0 {
		return body, "", "", nil
	}
	remaining := append([]string{}, lines[:blockStart]...)
	remaining = append(remaining, lines[blockEnd+1:]...)
	return strings.TrimSpace(strings.Join(remaining, "")), lang, script, nil
}

func findClosingFence(lines []string, start int, fence byte, minimum int) int {
	for i := start; i < len(lines); i++ {
		candidate := strings.TrimSuffix(lines[i], "\n")
		if closingFence(candidate, fence, minimum) {
			return i
		}
	}
	return -1
}

func openingFence(line string) (fence, info string, ok bool) {
	if len(line) < 3 || (line[0] != '`' && line[0] != '~') {
		return "", "", false
	}
	n := 1
	for n < len(line) && line[n] == line[0] {
		n++
	}
	if n < 3 {
		return "", "", false
	}
	return line[:n], line[n:], true
}

func closingFence(line string, fence byte, minimum int) bool {
	trimmed := strings.TrimRight(line, " ")
	if len(trimmed) < minimum {
		return false
	}
	for i := range trimmed {
		if trimmed[i] != fence {
			return false
		}
	}
	return true
}
