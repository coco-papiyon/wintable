# Windows版の導入と利用

この導入手順と標準サンプルはWindows amd64制御端末・Windows対象向けです。Linux対象にはhostの`os: "linux"`とLinux形式のmodule引数を指定します。Linuxの対象環境・準備・制限は[Linux command](linux-command.md)、[Linux file_content](linux-file-content.md)、[対応表](support.md)を参照してください。Windows対象はcmd.exe既定のOpenSSH ServerとPowerShell 5.1以上が必要です。制御側のZIP展開と実行にGo、WSL、Python、管理者権限は不要です。対象側のSSH Server導入・サービス設定には管理者権限が必要な場合があります。Windows実証環境は固定Server Core LTSC 2022コンテナです。Windows 11/フルServerの全シナリオ合格は主張しません。

## 導入・更新

配布元で確認したZIPのSHA-256と `Get-FileHash .\wintable-*.zip -Algorithm SHA256` を照合します。同じ経路で入手したハッシュだけでは配布元の真正性は保証できないため、信頼するリリースの値を確認してください。
一般ユーザーが書ける場所（例 `$env:LOCALAPPDATA\wintable`、日本語・空白入りパスも可）へ `Expand-Archive` で展開し、`wintable.exe help` を実行します。必要ならそのフォルダーをユーザーPATHへ登録します。システムPATHの変更は不要です。

更新時は実行中のwintableを終了し、別フォルダーへ新ZIPを展開・照合してから切り替えます。inventory、鍵、known_hostsは配布フォルダー外に保管し、サンプルで上書きしません。問題時は前版へ戻せますが、対象に実施済みの変更は戻りません。

## SSHの準備

1. 対象のOpenSSH Serverで公開鍵認証とTCP 22（変更時はinventoryのport）を有効にし、既定シェルをcmd.exeにします。PowerShellでAdd-Typeが使えることを確認します。
2. 専用の接続ユーザーと鍵ペアを用意します。対象ユーザーのauthorized_keysを設定します。Windowsの管理者ユーザーではsshd_configのAdministrators用authorized_keys設定も確認します。対象ファイルを変更できる最小限の権限を与えます。
3. 制御側の秘密鍵と鍵フォルダーは、その実行ユーザーと必要な管理者以外に読み取りを許さないACLにします。秘密鍵をJSON、Git、配布ZIPへ含めません。
4. 管理者など信頼できる別経路から対象ホスト鍵のフィンガープリントを確認し、known_hostsを事前作成します。`ssh-keyscan` の取得結果を未確認のまま信用しません。非標準ポートの登録は `[host]:port` 形式です。未知・変更ホスト鍵を自動承認する機能はありません。
5. 必要なら `ssh -i <秘密鍵> -o UserKnownHostsFile=<known_hosts> automation@host whoami` で疎通を確認します。wintable自身はssh.exeを必要としません。パスワード・SSH agent認証は非対応です。暗号化秘密鍵はプロセス環境変数 `WINTABLE_KEY_PASSPHRASE` に安全な方法で設定し、利用後に削除します。値をコマンド履歴やログへ記録しません。

## サンプルから実行

`examples/inventory.json` を作業用へコピーし、address、user、port、private_key_file、known_hosts_fileを変更します。鍵・known_hostsの相対パスはinventoryファイルのあるフォルダーが基準です。`examples/tasks.json` の対象パス `C:\Temp\wintable-message.txt` は必要に応じて変更し、親フォルダーを事前に作成してください。対象パスはWindows絶対パスです。サンプルに秘密情報は含まれません。

```powershell
.\wintable.exe run --inventory .\examples\inventory.json --tasks .\examples\tasks.json --format json > result.json
$code = $LASTEXITCODE
Get-Content .\result.json -Raw | ConvertFrom-Json
"exit=$code"
# 単一ホスト・変数上書き・制限時間・テキスト表示
.\wintable.exe run --inventory .\examples\inventory.json --tasks .\examples\tasks.json --limit win01 --vars-file .\examples\vars.json --forks 1 --timeout 60s --format text
```

結果はホスト名・task_index順で、JSONは単一文書、診断はstderrへ出ます。statusは `ok` / `changed` / `failed` / `unreachable` / `skipped` / `cancelled`。changedはtrue（変更あり）、false（変更なし）、null（変更したか不明）です。commandは終了0でも毎回changed=trueとなり、file_contentは同一バイトならchanged=falseです。子プロセスのexit_codeとCLIの終了値は別です。

| CLI終了値 | 意味 |
| --- | --- |
| 0 | 全件成功 |
| 1 | タスク失敗・到達不能等 |
| 2 | 引数・入力・全件事前検証のエラー |
| 3 | 内部・配布物・結果出力の障害 |
| 130 | Ctrl+C等のキャンセル |

併発時は130→3→2→1→0の優先順位です。stdout/stderr本文は既定で含みません。`--include-output` 指定時のみ各ストリーム先頭1MiBをbase64で含め、超過をstdout_truncated/stderr_truncatedで示します。本文は秘密を含む可能性があり、自動マスキングは保証しません。保存先のアクセス権にも注意してください。

## 制限と障害後の確認

Windows対象のcommandはCRT規則でargvを解釈する絶対パス.exe専用です。シェル構文、cmd.exe、.bat/.com/.ps1、独自引数解析exe、対話入力、PTY、パスワード・agent認証、権限昇格は非対応です。Windows file_contentは既存親フォルダー下の通常ファイル向けで、reparse point等を拒否します。OS別契約は[モジュール一覧](design/modules.md)、[Windows command](design/modules/command-module.md)、[Windows file_content](design/modules/file-content-module.md)、[Linux command](linux-command.md)、[Linux file_content](linux-file-content.md)を参照してください。

接続期限10秒、タスク期限は既定60秒（`--timeout`で指定）です。失敗ホストの後続タスクはskipped、他ホストは続行します。自動再試行はしません。タイムアウト・切断・Ctrl+CでSSHを閉じても対象プロセスの停止は保証できません。changed=nullなら対象のプロセス、出力ファイル、サービス状態等を別接続で調査し、処理がまだ動いていないか確認してから手動再実行してください。commandの二重実行は副作用を生む場合があります。


## リモートファイルの取得

fetchのsrcは対象OSの絶対パス、destはWindows制御端末の保存ルートです。[fetch利用例・保存名・上書き仕様](fetch-module.md)と[タスク例](../examples/fetch.json)を参照してください。inventoryの各host.vars.fetch_srcを設定して実行します。

## ローカルcommand委譲

`delegate_to: "localhost"` を指定したcommandはWindows制御端末上で実行します。`become` は省略または `false` のみで昇格しません。元ホストごとに変数展開・実行し、結果も元ホストに所属します。全taskが委譲ならSSH接続属性・鍵・known_hostsは不要です。絶対exeとargv、出力上限、期限・中断時の子孫清掃などは[ローカルcommandの仕様](local-command.md)を参照してください。

## fileディレクトリ作成

moduleをfile、argsをpath（対象OS絶対パス）とstate: directoryにします。Windowsローカルはdelegate_to: localhostを追加します。詳細と安全性は[file仕様](design/modules/file-module.md)、例は[Linux](../examples/linux-file.json)・[Windows](../examples/windows-file.json)・[ローカル](../examples/local-file.json)を参照してください。
