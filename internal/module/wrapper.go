package module

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/base64"
	"encoding/binary"
	"unicode/utf16"
)

//go:embed command.cs
var commandSource string

// CommandWrapper is immutable code: only Request.Stdin contains task values.
// Compression is distribution encoding, not task-dependent script generation.
func CommandWrapper() string {
	var b bytes.Buffer
	z, _ := gzip.NewWriterLevel(&b, gzip.BestCompression)
	_, _ = z.Write([]byte(commandSource))
	_ = z.Close()
	script := `$ErrorActionPreference='Stop';$ProgressPreference='SilentlyContinue';try{$m=[IO.MemoryStream]::new([Convert]::FromBase64String('` + base64.StdEncoding.EncodeToString(b.Bytes()) + `'));$z=[IO.Compression.GzipStream]::new($m,[IO.Compression.CompressionMode]::Decompress);Add-Type ([IO.StreamReader]::new($z).ReadToEnd());[Receiver]::Run()}catch{[Console]::OpenStandardOutput().Write([Text.Encoding]::UTF8.GetBytes('{"error":"execution_failed"}'),0,28);exit 1}`
	units := utf16.Encode([]rune(script))
	data := make([]byte, len(units)*2)
	for i, u := range units {
		binary.LittleEndian.PutUint16(data[i*2:], u)
	}
	return `powershell.exe -NoLogo -NoProfile -NonInteractive -EncodedCommand ` + base64.StdEncoding.EncodeToString(data)
}

// WrapperLaunchLine models Win32-OpenSSH's default cmd.exe /c invocation.
// This is the measured line, not the SSH exec request (which omits cmd).
func WrapperLaunchLine(command string) string {
	return `"C:\Windows\System32\cmd.exe" /c "` + command + `"`
}
func ValidateCommandWrapper() error { return ValidateWrapper(WrapperLaunchLine(CommandWrapper())) }
