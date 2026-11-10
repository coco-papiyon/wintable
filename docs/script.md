# powershell / shell

`powershell` は Windows の Windows PowerShell 5.1、`shell` は Linux の `/bin/sh` を実行する。逆の OS、混在対象への同一 OS 専用タスク、`delegate_to` は接続前に入力エラー（終了2）。暗黙の読み替えはしない。

```json
{"version":1,"hosts":"all","tasks":[{"name":"script","module":"shell","args":{"script":"message='日本語'; printf '%s\\n' \"$message\" | cat"}}]}
```

引数は必須の `script` 文字列だけ。複数行と日本語を許容する。空・空白のみ・NUL・不正 UTF-8・未知キー・型違いは拒否する。展開後 UTF-8 で1MiB以下、JSON は2MiB以下。制御側と receiver の両方で検査する。

既存の `${name}` 一回展開を適用する。展開値はコードとして解釈され、自動引用しない。信頼できるコード・変数だけを渡す。POSIX の `${HOME}` は wintable 変数として解釈されるため `$HOME` を使う。未定義変数は接続前エラー。PowerShell の `$name`、`$env:NAME` は通常どおり利用できる。

タスク値は固定 SSH 起動行へ連結しない。Linux は既存の CPython 3.13 receiver が `/bin/sh` を配列で起動し、スクリプトを stdin に送信する。送信と stdout/stderr の読み取りは並行する。stdin はコードのストリームであり、追加のデータ入力としては使えない。コード送信後 EOF。`-c` 引数に載せないため OS の一引数長による制限は受けない。

Windows は継承されたユーザー TEMP の下にランダム名の専用ディレクトリを Win32 CreateDirectory で排他的に作り、CreateNew で `script.ps1` を保存する。PowerShell 5.1 が日本語を解釈できるよう UTF-8 BOM を付け、絶対パスの powershell.exe を `-NoProfile -NonInteractive -File` で別プロセス起動する。子 stdin は即時 EOF。ExecutionPolicy は変更しない。対象アカウントのポリシーがローカル生成スクリプトの実行を許可する必要がある。

通常終了・起動失敗時には finally でスクリプトとディレクトリを削除する。スクリプトが別ファイルを作った場合など、清掃できなければ `failed/changed:null`（終了値なし）。他のファイルを再帰削除しない。TEMP は同一アカウントと管理者を信頼する既存境界を使う。強制終了・異常切断時には一時領域や子孫プロセスが残る場合がある。切断後の子孫停止は保証しない。自動再試行しない。

終了0は `changed/true`、非0は `failed/null` と子 `exit_code`。実行前拒否・起動失敗は `failed/false`、起動後の通信断・timeout・不正応答・清掃失敗は `failed/null`。失敗後の同一ホスト後続タスクは skip。親 cancel は既存 engine の規則に従う。

PowerShell は通常の `-File` 終了規則を使う。明示 `exit n` は n、構文エラー・throw は1。既定の非終端 `Write-Error` とネイティブコマンド非0だけでは失敗にならない。ネイティブ終了値が必要なら `exit $LASTEXITCODE` を明示する。Linux は最終コマンド・明示 exit・構文エラーの `/bin/sh` 終了値を使い、`set -e` は追加しない。

出力は両ストリームとも先頭1MiBを保持し、超過分も drain して truncated を設定する。既定では非公開、`--include-output` の場合だけ base64 で返す。スクリプト出力や例外詳細を外側診断に混入させない。出力バイトのエンコードは実行環境依存。Windows の日本語出力を UTF-8 に固定する例は `[Console]::OutputEncoding=[Text.UTF8Encoding]::new($false); [Console]::Write('日本語')`。ファイル用の BOM と出力エンコードは別である。

PowerShell 7 / bash の自動選択、実行ファイル指定、cwd/env、引数配列、追加 stdin、自己申告 changed、オブジェクト結果は未対応。JSON 例は [Linux](../examples/tasks-shell.json) と [Windows](../examples/tasks-powershell.json)。

通常確認は全 Go 試験（PowerShell 実プロセス含む）と Linux 実 SSH（shell receiver・製品 CLI・出力上限・期限/中断/切断含む）。Windows 実 SSH と配布ゲートは配布物作成時に実施する。
