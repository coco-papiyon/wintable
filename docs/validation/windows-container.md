# Windowsコンテナ検証と証跡

Issue #11本文の対象変更に従い、digest固定のServer Core 2022 amd64コンテナを対象とする。Windows 11/別Server版のVMの合格を意味しない。単体・模擬試験の制御側はWindowsホスト、実SSHの対象は隔離コンテナ、V-13の制御側も別のクリーンコンテナである。

PowerShell 7.3以上、Windows amd64、Go 1.27.3、WindowsコンテナモードのDockerとHyper-V分離を準備する。作業ツリーをcommitしてから実行する。

```powershell
./tests/validation/run.ps1
./tests/validation/test-summary.ps1
```

既定イメージは `mcr.microsoft.com/windows/servercore@sha256:b52799a67fa596d4bddcd82ae1dab0077a84767fbc33c7342b72b53ecc71ad09`。OpenSSHはホストのプログラムだけをコピーし、版とSHA-256をmanifestへ保存する。[SSH fixture](../../tests/windows-ssh/README.md)が専用ユーザー、cmd既定シェル、ホスト鍵と一時認証鍵を用意する。ホストのSSHサービスやネットワーク設定は変更しない。

## 実行内容

- `go test ./... -count=1 -v -timeout=8m`、`go vet ./...`、固定ラッパー長検査、Windows amd64/CGOなしbuild。
- Goでコンパイルしたmodule試験exeを、別の固定Server Coreコンテナ内で実行。file_contentの新規/更新/同値、UTF-8/BOM/改行、親なし、ロック、ACL保持、junction、同時書込み、失敗時の一時ファイル清掃を実測する。
- 実OpenSSHでcommand/file_content、CRT argv、起動長、stdin受信、切断、symlink差し替え、ACL拒否を検証する。
- ed25519、RSA 3072、ECDSA P-256の公開鍵認証と、暗号化ed25519鍵の正常/誤/未設定パスフレーズを検証する。未知/変更ホスト鍵、誤認証、閉じたTCP接続先を拒否する。鍵の互換範囲はこの列挙と使用OpenSSH版に限る。
- ループバックTCP中継で、実対象のSSHバナーを1,500ms遅延させ、300ms接続期限で解放することを確認する。中継は試験終了時に閉じ、ホストの遅延設定やファイアウォールは変更しない。
- 製品exeからLF/CRLF/BOM付きJSONを読み、SSHを実行し、JSON/テキストを取得する。CP932/UTF-8/CP437/CP1252の出力と全256バイトをbase64復元して照合する。
- 実SSHコマンド実行後、閉じたstdoutパイプで終了3を確認する。専用補助プロセスだけを既存コンソールから切り離し、新規コンソールを確保して製品へ実際の `CTRL_C_EVENT` を送る。対象exeの起動マーカーを待ってから送信し、終了130、単一JSON、changed=nullを確認する。呼出元コンソールには送らない。
- V-13は別の固定Server Core制御コンテナから製品exeでSSHを行い、command/file_content成功と再実行changed=falseを確認する。Server Core同梱のWSLランチャーは隔離制御コンテナの書込み層内だけで削除し、Go/WSL/Python/pyが解決されないことを検査する。exeと一時鍵は読取り専用bind mountで渡し、鍵をイメージに含めない。

Ctrl+Cや通信断で対象の子プロセスが停止する保証はしない。プロセス一覧を保存した後、専用コンテナを削除する。元の鍵、制御側コピー、対象/制御/ネイティブ試験コンテナ、専用SSHイメージの清掃結果を記録する。

## 証跡と合格条件

新規 `.cache/validation/<run-id>/` に保存する。既存ディレクトリは拒否し、修正前の失敗証跡を上書きしない。

| ファイル | 内容 |
| --- | --- |
| manifest.json | ソースSHA、作業ツリー状態、runnerハッシュ、Go/OS版、各コマンド/終了値、exe SHA-256、固定起動長測定式、集約結果 |
| test/vet/build/wrappers/module_binary.log | 単体・模擬、静的検査、ビルド、ラッパー長検査 |
| native-container.log | 隔離コンテナ内のファイル安全性試験 |
| ssh/manifest.json | 対象イメージ、OpenSSH版/ハッシュ、実行コマンド/終了値、清掃 |
| ssh/controller.json、controller.log | クリーン制御コンテナの環境、ツール不存在、製品結果、exe/source一致と清掃 |
| ssh/cli-test.log | 実exeの入力形式、出力障害、Ctrl+C |
| ssh/command-test.log、test.log | 実OpenSSH・cmd/PowerShell・CRT・鍵・遅延・切断の試験結果 |
| coverage.json、coverage.csv | 各必須試験名、シナリオ、照合ログ、判定、検証範囲 |
| scenarios.csv、test-cases.csv | V-01〜18の判定とログ別PASS/FAIL/SKIP索引 |
| sha256.csv | 保存ファイルの相対パスとハッシュ（exeはmanifestで別記） |

[`coverage.json`](../../tests/validation/coverage.json) は検証計画の必須ケースと実試験の対応表。集約器は各指定試験が指定ログに一度だけPASSしたことを確認する。欠落、SKIP、重複、FAILは合格にしない。ローカル試験のSSH未設定SKIPは保存し、実SSHログのPASSで対応する必須項目を検証する。

V-13は別途、正常終了、完了マーカー、exe/source一致、build成功、制御側清掃を照合する。全18シナリオのpassedに加え、test/vet/build/ラッパー/試験exeビルド/ネイティブコンテナの終了0、SSH三工程の終了0、source一致、全清掃、開始時作業ツリーcleanが揃った場合だけacceptance=passed・runner終了0。それ以外は終了1とする。各コマンドの終了値は独立して保存する。

再集約は `./tests/validation/summarize.ps1 -EvidenceDirectory <run-dir>`。集約器の23パターン試験は、部分結果、欠落/不整合、失敗、SKIP、重複、dirty source、清掃失敗などを合格にしないことを確認する。

証跡の公開前に秘密鍵やパスフレーズの混入を点検する。exe・試験exe・Docker context・一時鍵はGitへ含めない。レビュー済みの証跡は `docs/validation/evidence/` へ選別し、選別後の集合にハッシュ索引を作る。修正後の再検証は新しいrun-idとソースSHAで保存する。

保存済みの全シナリオ再検証は [20261108-complete](evidence/20261108-complete/README.md) を参照。
