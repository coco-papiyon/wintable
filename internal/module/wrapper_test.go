package module

import "testing"

func TestDistributionWrapper(t *testing.T) {
	t.Logf("wrapper UTF-16 units: %d", UTF16Len(WrapperLaunchLine(CommandWrapper())))
	if err := ValidateCommandWrapper(); err != nil {
		t.Fatal(err)
	}
	line := CommandWrapper()
	for _, value := range []string{"", `a"b`, "😀\r\n&|%PATH%", "secret task value"} {
		p, err := Prepare("h", 1, "command", map[string]any{"executable": `C:\tool.exe`, "argv": []any{value}})
		if err != nil {
			t.Fatal(err)
		}
		r, err := p.Request(CommandWrapper())
		if err != nil || r.Command != line {
			t.Fatal("task changed launch line", err)
		}
	}
}
