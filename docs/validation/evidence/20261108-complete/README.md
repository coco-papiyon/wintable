# Windowsコンテナ全シナリオの再検証証跡

Issue #11の実装3回目。2026-11-08 01:29:56〜01:35:45 UTCに最終runner全工程を実行し、終了0、acceptance=passed、V-01〜18すべてpassedを確認した。過去のNGコメントは当時の結果として保持する。

実測ソースSHA: `78be7f2e89b15507fb1d147b00d668f82537fb6b`。この後の変更は本証跡保存とドキュメントのみ。製品exe SHA-256: `8DB7290FDED09579E28559EE778183683F63CA6F5B4CA674621B00B6D398971A`。実行コマンドは `./tests/validation/run.ps1 -OutputDirectory .cache/validation/issue11-accepted`。

## 環境と結果

制御・対象のServer Core 2022 amd64イメージは `mcr.microsoft.com/windows/servercore@sha256:b52799a67fa596d4bddcd82ae1dab0077a84767fbc33c7342b72b53ecc71ad09`、Hyper-V分離。対象はcmd既定、PowerShell 5.1.20348.5773、OpenSSH sshd.exe 9.5.6.4。Go 1.27.3、WindowsホストのPowerShell 7.6.9で試験を実行した。詳しいOS、版、ハッシュ、コマンド、終了値は各manifestを参照する。この組合せについての合格であり、別OS/別OpenSSH版の互換性を主張しない。

| 証跡 | 結果 |
| --- | --- |
| final/manifest.json | test/vet/ラッパー/build/試験exeビルド/ネイティブコンテナ各終了0、clean source、前提条件合格 |
| final/scenarios.csv、coverage.csv、coverage.json | 全18シナリオpassed。検証計画に対応する77照合行が指定ログのPASSと一致。対応表のSHA-256も照合 |
| final/native-container.log | 固定イメージ内で実file_content全試験成功（59.08秒）。新規/変更/同値、BOM/改行、ACL、親なし、ロック、junction、同時更新、一時ファイル清掃 |
| final/ssh/command-test.log | 実SSH module試験成功（107.967秒）。CRT、CP932/65001/437/1252、入力境界、分割UTF-8/EOF、実起動行、ACL拒否とsymlink競合 |
| final/ssh/cli-test.log | LF/CRLF/BOMとJSON/テキスト、SSH実行後のstdout障害で終了3、実CTRL_C_EVENTで終了130/cancelled/changed=null（14.70秒） |
| final/ssh/test.log | 実OpenSSH transport試験成功（36.34秒）。ed25519/RSA3072/ECDSA P256、暗号化鍵の正常/誤/未設定パスフレーズ、ホスト鍵拒否、閉じた接続先、1,500ms遅延/300ms期限、出力上限、切断、再試行なし |
| final/ssh/controller.json、controller.log | V-13成功。別のGo/WSL/Pythonなしの制御コンテナで製品exeからcommand/file_contentと再実行changed=false、exe/source一致、清掃完了 |
| final/wrappers.log、ssh/wrapper-launch*.json | command 7,185、file_content 7,997 UTF-16単位。実cmd接頭辞/引用符を含む観測値とモデルが一致。非BMPは2単位 |
| summary-tests.log | 集約器23パターン成功。欠落/SKIP/FAIL/重複/不整合/dirty source/清掃失敗/対応表欠落などを合格にしない |

`test-cases.csv` はローカルのSSH未設定やsymlink権限不足によるSKIPも保持する。これらをPASSへ書き換えず、コンテナ内・実SSHで実行した別ログの必須試験を照合している。V-01〜04/15等の単体・模擬試験と、Windows/SSHの実測をcoverage.csvのログ列で区別する。

## 修正前の失敗と再試験

- `failures/console/`: ソース `4a7047ab67626124ad3d3c45d1d67592c2794781`。専用コンソール確保がAccess deniedとなり、製品へCtrl+Cを送る前に補助試験が失敗した。補助プロセスだけをFreeConsole後にAllocConsoleするよう修正。
- `failures/observation/`: ソース `df72c05`。Ctrl+Cは成功したが、対象に残るラッパーが後続の起動行観測に干渉した。リモート停止を保証せず、起動行の観測をCLI中断試験の前へ移動して試験を分離した。
- 修正後は成功ログの差し替えで済ませず、上記実測ソースで全工程を新規ディレクトリへ再実行した。失敗した試行の結果・ログを合格の根拠には使わない。

対象プロセス一覧をfinal/ssh/processes.jsonに保存し、対象・制御・ネイティブ試験コンテナ、専用SSHイメージ、元の秘密鍵と制御側鍵コピーの削除を確認した。Ctrl+Cや通信断による対象子プロセスの停止は保証しない。

## 保存と追跡

`final/` は実測時のファイルから選別したコピー。秘密鍵、exe、試験exe、Docker context、肥大化したsshdログを含めない。公開前に秘密鍵ヘッダーと疑似パスフレーズの非混入を点検した。manifestに記載したexe/sourceハッシュと、保存ファイルの `sha256.csv` を保持する。親ディレクトリの.gitattributesがバイト列を保持し、checkout時の改行変換を防ぐ。

元の全ログは実行時の `.cache/validation/issue11-accepted/` にある。失敗試行は `.cache/validation/issue11-complete/` と `.cache/validation/issue11-final/` に保持した。これらのローカルコピーには保持期間を設けていない。レビュー・引継ぎ用の永続索引はこのディレクトリである。
