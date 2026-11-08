package module

import (
	"bytes"
	"compress/flate"
	_ "embed"
	"encoding/base64"
	"encoding/binary"
	"unicode/utf16"
)

//go:embed command.cs
var commandSource string

//go:embed receiver.cs
var receiverSource string

//go:embed file_content.cs
var fileContentSource string

// CommandWrapper is immutable code: only Request.Stdin contains task values.
// Compression is distribution encoding, not task-dependent script generation.
func CommandWrapper() string {
	return fixedWrapper("using System.Diagnostics;using System.Threading.Tasks;" + receiverSource + commandSource)
}
func FileContentWrapper() string { return fixedWrapper(receiverSource + fileContentSource) }
func ValidateFileContentWrapper() error {
	return ValidateWrapper(WrapperLaunchLine(FileContentWrapper()))
}
func fixedWrapper(source string) string {
	var b bytes.Buffer
	z, _ := flate.NewWriter(&b, flate.BestCompression)
	_, _ = z.Write([]byte(source))
	_ = z.Close()
	script := `$ErrorActionPreference=1;$ProgressPreference=0;try{$m=[IO.MemoryStream]::new([Convert]::FromBase64String('` + base64.StdEncoding.EncodeToString(b.Bytes()) + `'));$z=[IO.Compression.DeflateStream]::new($m,[IO.Compression.CompressionMode]::Decompress);Add-Type ([IO.StreamReader]::new($z).ReadToEnd());[Receiver]::Run()}catch{exit 1}`
	units := utf16.Encode([]rune(script))
	data := make([]byte, len(units)*2)
	for i, u := range units {
		binary.LittleEndian.PutUint16(data[i*2:], u)
	}
	return `powershell.exe -NoProfile -NonInteractive -EncodedCommand ` + base64.StdEncoding.EncodeToString(data)
}

// WrapperLaunchLine models Win32-OpenSSH's default cmd.exe /c invocation.
// This is the measured line, not the SSH exec request (which omits cmd).
func WrapperLaunchLine(command string) string {
	return `"C:\Windows\System32\cmd.exe" /c "` + command + `"`
}
func ValidateCommandWrapper() error { return ValidateWrapper(WrapperLaunchLine(CommandWrapper())) }
