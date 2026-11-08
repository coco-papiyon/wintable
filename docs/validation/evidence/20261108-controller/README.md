# クリーン制御コンテナのV-13証跡

実測ソースは `828c062e8fd02cb8b5cba4b39ae20d803233e08e`。製品コードは初回証跡から変更していない。`final/manifest.json` の作業ツリー状態も参照すること。runner全工程をこのSHAで実行し、V-13を含む最終結果を保存した。証跡追加だけの後続commitは試験実装を変更しない。

制御・対象ともServer Core 2022 amd64、digestは `sha256:b52799a67fa596d4bddcd82ae1dab0077a84767fbc33c7342b72b53ecc71ad09`。制御側では同梱WSLランチャーをコンテナ層内だけで除去し、Go/WSL/Python/pyコマンドなしを検査。PowerShell 5.1.20348.5773で製品exeから対象へ実SSH接続した。commandとfile_contentの初回・再実行がともに終了0で、再実行のfile_contentはchanged=false。

`final/ssh/controller.json` はexe SHA-256、ソースSHA、Dockerコマンド、終了値、後始末結果。`controller.log` は環境・実CLIコマンド・単一JSON結果・完了マーカー。`controller/` は秘密鍵とexeを除外した入力・実行スクリプト。対象OpenSSHとcmd既定シェルの情報はSSH manifestとtest.logに記録する。

`final/scenarios.csv` はV-13のみpassed、他は全サブケース照合が未完了のためunconfirmed。`test-cases.csv` は個別Go試験のPASS/FAIL/SKIPを保持する。全体acceptanceはunconfirmed、runner終了1。V-13合格を全18項目合格へ拡張しない。

`failed-wsl-present/` は同梱wsl.exeを検出した失敗、`failed-preparation/` は除去コマンドのパス表記の失敗、`failed-input-path/` は試験入力の絶対パス表記による終了2。失敗は上書きせず、各試行のmanifest・終了値・ログを残した。

`sha256.csv` はこの選別済みファイル集合のハッシュ。元runディレクトリの秘密鍵・バイナリ・OpenSSHコピー・Docker context・既存の大量ログは含めない。runnerの元索引は選別前の集合なので、選別後の索引と混同しない。秘密鍵コピー・元鍵、制御/対象コンテナ、専用イメージの後始末は各manifestで確認する。

集約の負例試験は `tests/validation/test-summary.ps1`：正常証跡でも全体は未確認、ログ欠落、exe/source SHA不一致、null/非0終了値、後始末未完了、完了マーカー欠落、ビルド証跡欠落の9件を検証する。

未完了：実CLI Ctrl+C・出力先障害、鍵種別と遅延注入の網羅、V-01〜18全サブケースの実測照合。Issue #11の全体受け入れは未完了。
