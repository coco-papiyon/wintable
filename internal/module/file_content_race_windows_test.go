package module

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Inject a synchronous swap immediately before the production no-follow open.
// The marker proves a real file symlink was installed, rather than accepting
// an unrelated failure (including missing symlink privilege) as a passing test.
func TestFileContentLinkSwapWindows(t *testing.T) {
	container := ""
	if fixture := os.Getenv("WINTABLE_SSH_FIXTURE"); fixture != "" {
		raw, err := os.ReadFile(filepath.Join(fixture, "fixture.json"))
		if err != nil {
			t.Fatal(err)
		}
		var cfg struct{ Container string }
		if err := json.Unmarshal(raw, &cfg); err != nil || cfg.Container == "" {
			t.Fatal("invalid fixture", err)
		}
		container = cfg.Container
	} else {
		dir := t.TempDir()
		if err := os.Symlink(filepath.Join(dir, "referent"), filepath.Join(dir, "probe")); err != nil {
			t.Skipf("file symlink privilege unavailable: %v", err)
		}
	}
	for _, content := range []string{"expected", "different"} {
		t.Run(content, func(t *testing.T) {
			anchor := "var h=CreateFile(p,0x80000000"
			if strings.Count(fileContentSource, anchor) != 1 {
				t.Fatal("open injection point changed")
			}
			hook := `File.GetAttributes(p);File.Delete(p);if(!Link(p,Environment.GetEnvironmentVariable("WT_REFERENT"),0))throw new Exception();File.WriteAllText(Environment.GetEnvironmentVariable("WT_MARKER"),"swapped");`
			source := strings.Replace(fileContentSource, anchor, hook+anchor, 1)
			source = strings.Replace(source, "public partial class Receiver {", `public partial class Receiver {[DllImport("kernel32",EntryPoint="CreateSymbolicLinkW",CharSet=CharSet.Unicode)]static extern bool Link(string p,string target,uint flags);`, 1)
			encoded := base64.StdEncoding.EncodeToString([]byte(receiverSource + source))
			// Only the test hook is added to the embedded receiver; all validation,
			// handle inspection, comparison, response and cleanup are production code.
			script := `$ErrorActionPreference='Stop';$d=Join-Path ([IO.Path]::GetTempPath()) ([Guid]::NewGuid().ToString('N'));[IO.Directory]::CreateDirectory($d)|Out-Null;try{$p=Join-Path $d 'target';$env:WT_REFERENT=Join-Path $d 'referent';$env:WT_MARKER=Join-Path $d 'marker';[IO.File]::WriteAllText($p,'original');[IO.File]::WriteAllText($env:WT_REFERENT,'` + content + `');Add-Type ([Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('` + encoded + `')));$json=@{module='file_content';args=@{path=$p;content='expected'}}|ConvertTo-Json -Compress;$inputFile=Join-Path $d 'input';[IO.File]::WriteAllText($inputFile,$json);$stream=[IO.File]::OpenRead($inputFile);`
			// Run() reads the native standard input, so launch the harness with
			// redirected input via a child PowerShell that compiles the same source.
			script = strings.Split(script, ";$stream=")[0] + `;$harness=Join-Path $d 'harness.ps1';[IO.File]::WriteAllText($harness,"Add-Type ([Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('` + encoded + `')));[Receiver]::Run()");$out=Join-Path $d 'output';$err=Join-Path $d 'error';$proc=Start-Process powershell.exe -WindowStyle Hidden -ArgumentList @('-NoProfile','-File',$harness) -RedirectStandardInput $inputFile -RedirectStandardOutput $out -RedirectStandardError $err -Wait -PassThru;if($proc.ExitCode -ne 0){throw 'harness failed'};if([IO.File]::ReadAllText($env:WT_MARKER) -ne 'swapped'){throw 'swap missing'};if(([IO.File]::GetAttributes($p) -band [IO.FileAttributes]::ReparsePoint) -eq 0){throw 'link missing'};if([IO.File]::ReadAllText($env:WT_REFERENT) -ne '` + content + `'){throw 'referent changed'};if(@(Get-ChildItem $d -Filter '.wintable-*.tmp').Count){throw 'temp leak'};[IO.File]::ReadAllText($out)}finally{if(Test-Path $p){[IO.File]::Delete($p)};Remove-Item -LiteralPath $d -Recurse -Force}`
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			args := []string{"-NoProfile", "-NonInteractive", "-Command", script}
			command := "powershell.exe"
			if container != "" {
				command = "docker"
				args = append([]string{"exec", container, "powershell.exe"}, args...)
			}
			out, err := exec.CommandContext(ctx, command, args...).CombinedOutput()
			if err != nil {
				t.Fatalf("synchronized link swap: %v: %s", err, out)
			}
			if !bytes.Equal(bytes.TrimSpace(out), []byte(`{"error":"write_failed"}`)) {
				t.Fatalf("link accepted or unexpected response: %s", out)
			}
			t.Log("synchronized file-to-symlink swap rejected; referent unchanged; no temporary files")
		})
	}
}
