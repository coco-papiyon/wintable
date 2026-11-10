# file: ディレクトリ作成

`module: "file"` の引数は `{"path":"<絶対パス>","state":"directory"}` の2つです。
Windows/LinuxのSSH対象と、Windows制御端末への `delegate_to: "localhost"` に対応します。
WindowsはPowerShell 5.1/C#、Linuxは既存のDebian 13/OpenSSH/CPython 3.13前提を使います。

必要な親を順に作成します。作成した場合はchanged=true、すべて既存のディレクトリなら
changed=falseです。既存ファイルは保持して失敗します。ACL・所有者・属性・既存時刻は
強制変更しません。新規作成は実行ユーザーの権限とOSの継承・umaskに従います。
touch・削除・リンク作成・権限変更には対応しません。

## パスと安全性

変数展開後の全件事前検証とreceiverの両方で検証します。OSルート、相対パス、空の要素、
dot/dot-dot、末尾区切り、制御文字を拒否します。Windowsはdrive-rootedのバックスラッシュ
形式のみです。UNC/device/ADS、予約名（拡張子付きやCOM¹等を含む）、末尾の空白・ピリオドを
拒否します。日本語・空白・シェル特殊文字は使用できます。値はJSON stdinにだけ渡します。
JSON上限2MiB、固定起動行はWindows 8,000 UTF-16単位/Linux 8KiBです。

対象と既存祖先のsymlink/junction/reparse pointは拒否します。Windowsはルートから
OPEN_REPARSE_POINT/BACKUP_SEMANTICSで開いたディレクトリのハンドルを保持し、delete共有を
許可せず名前の置換を防ぎます。各階層のCreateDirectory後もハンドルを検査します。
LinuxはrootからO_DIRECTORY/O_NOFOLLOWのfdを保持し、open/mkdirをdir_fd相対で行います。
リンクへの差し替えを追跡しません。ただしLinuxでは別プロセスによる開いたディレクトリの
renameを禁止できません。祖先の名前空間は信頼するユーザーが管理する必要があり、
敵対的ユーザーによるrenameやmount変更へのパス固定・隔離は保証しません。

## エラー・中断

入力拒否はfailed/changed=falseです。ファイル衝突・権限不足など実行段階の失敗は
failed/changed=nullとし、部分的に作成された親は残します。自動ロールバックしません。
通信断・timeout/cancelでも確定できない変更は不明です。再実行前に状態を確認してください。
例外・パス・内容は結果へ出しません。file_contentと同じ厳密なchanged/error応答を使い、
終了値やコマンド出力は返しません。

Windowsローカルも同じ固定receiverを使い、#41のJob Objectによるプロセス起動・期限・
中断・子孫清掃を共有します。元hostごとに実行し、順序・forks・失敗後skipを維持します。
becomeは省略またはfalseのみです。全taskがローカル委譲ならSSH属性は不要です。

## 試験

通常確認は全Go試験（Windowsローカル実プロセス、CLI事前検証を含む）とLinux実SSH
（transport、fileのreceiver/製品CLIを含む）です。Windows実SSHのTestFileSSHは配布時に
実行します。通常確認だけで両OS配布受入の合格とはしません。
