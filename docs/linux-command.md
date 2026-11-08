# Linux command

Windows製品CLIのhostに `"os":"linux"` を指定するとLinux command backendを使う。
対象はDebian 13 amd64 / OpenSSH / POSIX互換 `/bin/sh` / `/usr/bin/python3` の
CPython 3.13系。制御端末はWindows。Linux file_content、sudo/become、汎用shell、
package/systemd、再起動は対象外。対応公開・CIはIssue #29で扱う。

```json
{"name":"example","module":"command","args":{"executable":"/usr/bin/printf","argv":["%s\n","こんにちは"]}}
```

埋込 `internal/module/command_linux.py` だけをPOSIX引用して
`/usr/bin/python3 -I -c '<固定コード>'` を起動する。タスク値はUTF-8 JSON stdinのみ。
固定行の最終UTF-8長は8KiB以下をbuildと全接続前preflightで検査する。
receiverは許容サイズの入力をEOFまで読んでから検査・起動する。
Linux/CPython 3.13、厳密UTF-8、単一JSON、未知/重複キー、型、Unicode、NULを再検査する。
要求2MiB、exeと各引数32KiB、各終端NUL込み合計64KiB、argv4096要素を上限とする。
JSON空白を含む2MiB境界は生receiverで試験する。通常の有効argvから到達できない
複合上限を製品CLIの到達試験と混同しない。

executableはPOSIX絶対パス。末尾/、制御文字、.と..セグメントは拒否する。
空argv・空要素・日本語・引用符・空白・改行・演算子は引数として保持する。
Popenの配列とshell=Falseで起動し、PATH探索・暗黙shell・ENOEXEC fallbackを行わない。
明示shebangは実行可能。子stdinはDEVNULLでEOF、cwdと環境はSSHから継承する。

stdout/stderrをselectorsで並行排出し、各先頭1MiBを保持して超過後も排出する。
非UTF-8バイトもbase64で保存する。外側終了0・stderr空・単一JSON・3MiB以下が契約。
専用decoderは共通の厳密スキーマ/base64検査に加えLinux終了値 -64〜255を検査する。
正常0はchanged/true、非0・signalはfailed/nullで子終了値（signalは-N）を保持する。
invalid_request/start_failedはfailed/false・exit_code=null。
OS/Python不一致は子起動前の正常拒否。runtime不存在、不正応答、実行後障害、
応答不能はfailed/null・exit_code=null。既定で出力本文は公開しない。

SSH/engineはWindowsと共通で、接続再利用、host内順序、forks、再送0、
失敗後skipped、timeout/cancel、終了優先130→3→2→1→0を維持する。
通信中断による子孫停止は保証しない。試験fixtureはコンテナ破棄で回収する。

## 検証の再実行

```powershell
go test ./...
go vet ./...
./build.ps1
./tests/linux-ssh/run.ps1 -DockerContext desktop-linux -ProductTestScript ./tests/linux-ssh/product.ps1
./tests/windows-ssh/run.ps1
```

`TestLinuxCommandSSH/product-*` はWindows製品exe→実sshd→receiver試験。
argvはreceiverとは独立したPython probeでUTF-8長とhexを照合する。
`raw-*` は生receiverへ不正要求・合成境界・限定runtime shimを送る試験。
`receiver-interruption` は製品receiverを実SSH Sessionから中断する試験。
`TestLinuxOpenSSH` は独立probeによる送信中/EOF後timeout/cancel、通信断、再送0の試験。
`TestLinuxDecoder` は合成応答、backend/engine/CLI単体は模擬を含む。
通常go testのopt-in SSH skipを実SSH合格として扱わない。
