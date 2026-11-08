package module

import "testing"

func TestDistributionWrapper(t *testing.T) {
	t.Logf("Linux fixed launch bytes: %d", len(LinuxCommandWrapper()))
	if err := ValidateLinuxWrapper(LinuxCommandWrapper()); err != nil {
		t.Fatal(err)
	}
	t.Logf("file_content wrapper UTF-16 units: %d", UTF16Len(WrapperLaunchLine(FileContentWrapper())))
	if err := ValidateFileContentWrapper(); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"", "日本語\r\n\"😀", "&|%PATH%"} {
		p, err := Prepare("h", 1, "file_content", map[string]any{"path": `C:\file`, "content": value})
		if err != nil {
			t.Fatal(err)
		}
		r, err := p.Request(FileContentWrapper())
		if err != nil || r.Command != FileContentWrapper() {
			t.Fatal("file_content changed launch line", err)
		}
	}
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
