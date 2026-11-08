# Windowsコンテナ検証と証跡

Issue #11本文の対象変更に従い、Windowsコンテナを対象とする。過去の計画にあるWindows 11 / Server 2022のVMマトリクスは今回の合格条件へ転用しない。

PowerShell 7.3以上、Windows amd64、Go 1.27.3、WindowsコンテナモードのDockerとHyper-V分離を準備する。OpenSSHの準備、隔離ユーザー、鍵、ACL、後始末は[既存fixture](../../tests/windows-ssh/README.md)を参照。ホストを対象として初期化しない。

```powershell
./tests/validation/run.ps1
```

既定のServer Core 2022イメージはdigest固定。OpenSSHはホストのバイナリをコピーし、版とSHA-256をSSH manifestに記録する。制御側は現在Windowsホストであり、コンテナ制御側の確認を代替しない。

新しい `.cache/validation/<run-id>/` に以下を保存する。

- manifest.json: ソースSHA、未コミット状態、制御環境、Go版、コマンドと終了値、exe SHA-256。
- test/vet/build/wrappers.log: 単体・ローカルWindows試験、静的検査、Windows amd64ビルド、固定起動長。長さはcmd接頭辞・引用符を含むUTF-16コード単位数（非BMPは2）、上限8,000。
- ssh/: 対象OS、PowerShell/OpenSSH版、実起動行、command/file_contentとtransportのケース別ログ、後始末結果。
- scenarios.csv: V-01〜18のレビュー用索引。未確認を初期値とし、集約試験の成功だけではOKへ変更しない。
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
| V-13 | 未確認。クリーン制御コンテナで製品exeを実行する必要あり |
| V-14 | ssh/command-test.log: 長いargv、file_content冪等性とバイト一致 |
| V-15 | test.log: ContentBoundary、JSONBoundaryAndOwnership、LastHostLastTaskFailsBeforeExecutor |
| V-16 | wrappers.logとssh/command-test.log: 実測起動行、7999/8000境界 |
| V-17 | test.logのCRTとssh/command-test.logのCRT_roundtrip |
| V-18 | ssh/command-test.logの受信境界・不正入力・EOF・LaunchAndDisconnect、ssh/test.logの送信停止・切断 |

このrunnerは証跡収集の基盤であり、全シナリオの受け入れは未完了。終了1はその事実を示す。個々のコマンド終了値はmanifestに独立して保存する。

必須の残作業は、コンテナ制御側、Go/WSL/Pythonを含まないクリーン環境での製品exeによるSSH実行（V-13）、製品CLIの実Ctrl+Cと出力先障害、鍵種別・遅延注入、V-01〜18の全サブケースとログの対応付け。SSH fixtureの成功や単体テストのSKIPをこれらの合格として扱わない。現在のrunnerは全項目を未確認として出力するため、配布ゲートとして合格を返す機能はまだない。
