package unit

import (
	"strings"
	"testing"
)

const stopUnit = `---
title: 改了文件就留档
trigger:
  events: [stop]
  content: ['.']
deliver:
  as: %s
examples:
  - expect: hit
    event: {kind: stop, content: 改好了}
  - expect: miss
    event: {kind: stop, content: ""}
---
本轮改了文件就把做了什么、为什么追加到 docs/log/ 当天的文件。
`

func TestValidateAndDescribeStopUnit(t *testing.T) {
	u, err := Parse([]byte(strings.Replace(stopUnit, "%s", "inject", 1)), "log.md", "p")
	if err != nil {
		t.Fatal(err)
	}
	errs, warns := Validate(u)
	if len(errs) != 0 || len(warns) != 0 {
		t.Fatalf("errs %v warns %v", errs, warns)
	}
	text := Describe(u, 0.5)
	if !strings.Contains(text, "何时检查：AI 结束本轮回复之前") ||
		!strings.Contains(text, "做什么：在 AI 结束本轮之前提醒它一次，由它判断规则是否适用") {
		t.Fatalf("Describe() =\n%s", text)
	}
}

func TestValidateStopBlockRejected(t *testing.T) {
	u, err := Parse([]byte(strings.Replace(stopUnit, "%s", "block", 1)), "log.md", "p")
	if err != nil {
		t.Fatal(err)
	}
	errs, _ := Validate(u)
	found := false
	for _, e := range errs {
		found = found || strings.HasPrefix(e.Error(), "规则 5：")
	}
	if !found {
		t.Fatalf("errs = %v, want rule 5", errs)
	}
}
