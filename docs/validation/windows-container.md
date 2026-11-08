# Windowsコンテナ検証と証跡

Issue #11本文の対象変更に従い、Windowsコンテナを対象とする。過去の計画にあるWindows 11 / Server 2022のVMマトリクスは今回の合格条件へ転用しない。

PowerShell 7.3以上、Windows amd64、Go 1.27.3、WindowsコンテナモードのDockerとHyper-V分離を準備する。OpenSSHの準備、隔離ユーザー、鍵、ACL、後始末は[既存fixture](../../tests/windows-ssh/README.md)を参照。ホストを対象として初期化しない。

```powershell
./tests/validation/run.ps1
```

既定のServer Core 2022イメージはdigest固定。OpenSSHはホストのバイナリをコピーし、版とSHA-256をSSH manifestに記録する。Go試験の制御側はWindowsホスト。V-13は別の同一digestのServer Core amd64制御コンテナから、製品exeで隔離対象コンテナへ接続する。

Server CoreにはWSLランチャーが同梱されるため、V-13の制御コンテナの書込み層内だけで、その固定パスの所有権・ACLを変更しランチャーを除去する。ホストのファイルやACLは変更しない。Go/WSL/Python/pyコマンドが解決されないことを検査した後、command成功、file_content配置、日本語・引用符・改行の送信、再実行時のchanged=falseを検証する。準備操作もログに残す。exeと一時鍵は読取り専用bind mountで渡し、鍵をイメージへ焼き込まない。finallyで制御コンテナと鍵コピーを削除し、既存fixtureが対象側と元の鍵を削除する。

新しい `.cache/validation/<run-id>/` に以下を保存する。

- manifest.json: ソースSHA、未コミット状態、制御環境、Go版、コマンドと終了値、exe SHA-256。
- test/vet/build/wrappers.log: 単体・ローカルWindows試験、静的検査、Windows amd64ビルド、固定起動長。長さはcmd接頭辞・引用符を含むUTF-16コード単位数（非BMPは2）、上限8,000。
- ssh/: 対象OS、PowerShell/OpenSSH版、実起動行、command/file_contentとtransportのケース別ログ、後始末結果。
- ssh/controller.json、controller.log: 制御コンテナのコマンド・終了値・後始末、環境・非存在コマンド・exeハッシュ、製品CLI結果。
- scenarios.csv: V-01〜18の判定。V-13は成功終了、完了マーカー、ビルド成功、SHA一致、後始末完了を照合する。それ以外は全サブケースの網羅を未確認として維持する。
- test-cases.csv: Go試験の個別PASS/FAIL/SKIP。親試験のPASSでSKIPを消さない。
- sha256.csv: 保存ファイルの相対パスとハッシュ。exeはmanifestで別途記録。

ログは秘密鍵を含めず、SSH fixtureが生成した秘密鍵はfinallyで削除する。公開前には疑似秘密と実資格情報の混入を確認する。生ログの自動公開は行わない。実行ごとのディレクトリを上書きせず保存し、修正時は関連ケースを再実行する。ローカル証跡はGit管理外で保持期限を設定していないため、長期証跡はレビュー後に `docs/validation/evidence/` へ選別して保存する。

## 未完了の受け入れ

検証計画との照合先は次のとおり。これは試験の所在であり、全サブケースの合格表ではない。

| シナリオ | ログ・試験の照合先 |
| --- | --- |
| V-01〜03 | test.log: config、inventory、cliの厳密入力・全件事前検証 |
| V-04 | test.log: engineのParallelFailure、CancellationAndDeadline |
| V-05〜06 | ssh/test.log: TestWindowsOpenSSH。実Ctrl+Cは別途必要 |
| V-07 | ssh/command-test.log: TestCommandWindowsのcmd/ssh、CRT_roundtrip |
| V-08〜09 | ssh/command-test.log: TestFileContentSSH、TestFileContentLinkSwapWindows |
| V-10〜11 | moduleの非UTF-8出力・日本語パス、transportの両出力・秘密非表示。入力改行形式等は個別照合が必要 |
| V-12 | test.log: result、cli。実exeの出力先障害は別途必要 |
| V-13 | ssh/controller.jsonとcontroller.log。クリーン制御コンテナで製品exeから実SSHを実行 |
| V-14 | ssh/command-test.log: 長いargv、file_content冪等性とバイト一致 |
| V-15 | test.log: ContentBoundary、JSONBoundaryAndOwnership、LastHostLastTaskFailsBeforeExecutor |
| V-16 | wrappers.logとssh/command-test.log: 実測起動行、7999/8000境界 |
| V-17 | test.logのCRTとssh/command-test.logのCRT_roundtrip |
| V-18 | ssh/command-test.logの受信境界・不正入力・EOF・LaunchAndDisconnect、ssh/test.logの送信停止・切断 |

このrunnerは証跡収集の基盤であり、全シナリオの受け入れは未完了。終了1はその事実を示す。個々のコマンド終了値はmanifestに独立して保存する。

必須の残作業は、製品CLIの実Ctrl+Cと出力先障害、鍵種別・遅延注入、V-01〜18の全サブケースとログの対応付け。SSH fixtureの成功や単体テストのSKIPをこれらの合格として扱わない。V-13以外に未確認が残るため、runnerは全体合格を返さない。

保存済み証跡の再集約は `./tests/validation/summarize.ps1 -EvidenceDirectory <run-dir>`。`./tests/validation/test-summary.ps1` は、欠落ログ、SHA不一致、未確定終了値、失敗、後始末未完了等を合格にしないことを検査する。後から証跡を選別した場合は、選別後のファイル集合に対してハッシュ索引を作成する。
