package pipeline

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const archiveUnit = "---\n" +
	"title: 改了文件就留档\n" +
	"trigger:\n  events: [stop]\n" +
	"examples:\n" +
	"  - expect: hit\n    event: {kind: stop, content: 改好了}\n" +
	"  - expect: miss\n    event: {kind: stop, content: 这个函数的意思是}\n" +
	"---\n" +
	"本轮改了文件就把做了什么、为什么追加到 docs/log/ 当天的文件。\n\n" +
	"```sh jitr-judge\n[ -n \"$(git status --porcelain)\" ]\n```\n"

func TestStopRemindsOncePerTurnWhenTreeIsDirty(t *testing.T) {
	setupCustomProfile(t, map[string]string{"archive.md": archiveUnit}, "")
	dirty, clean := gitRepo(t), gitRepo(t)
	writeFile(t, filepath.Join(dirty, "a.txt"), "x")
	stop := fixture(t, "testdata/payloads/claude-code/stop.json")
	active := fixture(t, "testdata/payloads/claude-code/stop_active.json")

	out := Run(replaceFixtureCWD(stop, dirty), testNow)
	if out.Exit != 0 || !strings.Contains(string(out.Stdout), `"decision":"block"`) ||
		!strings.Contains(string(out.Stdout), "[custom/archive]") {
		t.Fatalf("dirty end output = %+v", out)
	}
	assertSilent(t, Run(replaceFixtureCWD(active, dirty), testNow))
	assertSilent(t, Run(replaceFixtureCWD(changeJSON(t, stop, "last_assistant_message", "chat"), clean), testNow))

	got := records(t)
	first := got[len(got)-3]
	if first.Event.Kind != "stop" || first.Result.Action != "inject" {
		t.Fatalf("first record = %+v", first)
	}
	if second := got[len(got)-2]; !second.Event.StopActive || second.Units[0].Skip != "redeliver" {
		t.Fatalf("re-sent end record = %+v", second)
	}
}

func gitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	return dir
}
