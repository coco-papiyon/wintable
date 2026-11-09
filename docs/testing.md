# 試験一覧と変更に応じた実行方針

2026-11-09整理。現行mainのコード・スクリプト・CIと既存証跡を調査したもの。所要時間の再計測はしていない。

## 試験内容と方法

| 分類 | 内容 | 方法・主な場所 |
| --- | --- | --- |
| 入力 | 厳密JSON、未知・重複キー、型、OS、パス、不正入力時に未接続 | config / cliのGoテスト |
| ホスト・変数 | 選択、順序、優先順位、未定義変数、展開後全件検証 | inventory / engineのGoテスト |
| 実行制御 | 並列数、順序、失敗後skip、接続再利用、期限、中断、清掃 | engineの模擬Connector / Session |
| 結果 | status / changed、集計、順序、JSON / text、終了値、出力失敗、秘匿 | result / cliのGoテスト |
| SSH通信 | 転送、EOF、鍵・認証拒否、再利用、期限、中断、通信断 | transportのGo製ローカルSSHサーバー |
| Windows command | CRT引数、日本語・特殊文字、終了値、出力バイト、不正入力、EOF | cmd.exe / PowerShell 5.1 / C# CRT fixtureを実起動 |
| Windows file_content | 新規・差分・同値、UTF-8・改行・BOM、時刻、ロック、ACL、リンク、競合、清掃 | 実ファイルAPI・固定ラッパー |
| Windows実SSH | モジュール、OpenSSHの鍵種別、障害、製品CLIのCtrl+C・broken stdout | Server Core 2022の使い捨てDockerコンテナ |
| Linux実SSH | command、file_content、属性・ACL・競合・障害注入・確定前後の切断 | Debian 13 / OpenSSH / CPythonの使い捨てDockerコンテナ |
| 配布・導入 | ZIP内容、ハッシュ、license、文書、非管理者導入、日本語パス、配布exe | PowerShellとクリーンWindowsコンテナ |
| 検証基盤 | 証跡欠落・重複・skip・SHA不一致・改ざん拒否、fixture障害時の清掃 | 合成証跡のスクリプト試験、fixture障害注入 |
| 静的検査 | 依存整合、静的検査、脆弱性監査、ビルド | go mod verify / go vet / govulncheck / go build |

## 最大値・境界試験

| 対象 | 境界 | 試験層 |
| --- | --- | --- |
| content | 1MiBとその前後 | Go送信準備、Windows / Linux受信・保存 |
| JSON | 2MiB−1 / 2MiB / 2MiB＋1 | Go直列化、raw receiver、SSH |
| 固定起動行 | UTF-16 7999 / 8000 / 8001単位 | Go長さ判定、実cmd / SSH |
| Windows子引数 | 32765 / 32766 / 32767単位 | CRT引用、実起動 |
| stdout / stderr | 各1MiB前後・超過 | Goバッファ、実プロセス、SSH、drain |
| Linux引数 | OSのE2BIG | raw receiver、製品exe |
| forks | 1 / 3 / 64 | 模擬接続による並列制御 |

Go内の判定と、実プロセス・実SSHへの最大入力は負荷が異なる。安価なGo境界テストは通常も残す。

## 実行入口と現在の自動実行

| 入口 | 範囲・タイミング |
| --- | --- |
| go test ./... | GoテストとWindows実プロセス・最大値試験。実SSHはfixture変数がなければskip |
| .github/workflows/ci.yml | 全push / PR。全Goテスト、vet、起動長、build、監査、配布ゲート回帰 |
| scripts/distribution.ps1 | 配布候補作成。全Goテスト、vet、起動長、監査、build、license、ZIP検査 |
| tests/validation/run.ps1 | 全Goテスト、起動長、コンテナ内file_content、Windows実SSH、導入、証跡集約 |
| tests/windows-ssh/run.ps1 | Windowsコンテナ準備、モジュール・CLI・transportの実SSH |
| tests/linux-ssh/run.ps1 | Linuxコンテナ準備と独立probeによるtransport。-ProductTestScript tests/linux-ssh/product.ps1で製品モジュールと文書例も実施 |
| tests/linux-ssh/verify.ps1 | 通常fixtureと起動・期限・清掃失敗、版不一致等の異常系 |
| .github/workflows/ssh-acceptance.yml | main等へのpush / 手動。配布候補作成とWindows実SSH |
| tests/ci/complete-local.ps1 | 同一SHAのActions配布物を取得し、ローカルLinux実SSHと両OS証跡を集約 |

詳細は[CI](ci.md)、Windowsケース対応は[coverage.json](../tests/validation/coverage.json)、Linux必須ケースは[linux-required.json](../tests/ci/linux-required.json)。

## 所要時間と重複

既存の[PR35ログ](validation/pr35-current/final/test.log)ではmodule全体57秒（command 27秒、file_content 28秒）、transport 15秒。[SSHモジュールログ](validation/pr35-current/final/ssh/command-test.log)では111秒、[実SSH transport](validation/pr35-current/final/ssh/test.log)では38秒。参考値であり、合計所要時間や現行環境の保証値ではない。準備・build・監査・清掃は別途必要。

最大値転送以外に、PowerShellの反復起動、EOF・期限・通信断待ち、コンテナ準備も時間を使う。main pushの通常CIと候補作成、候補作成と受入runnerで全Goテストが重複する。Windows SSH runner内のcommandはcmdとSSH双方で実行し、file_contentもローカル・コンテナ・SSHの複数層で実行する。

## 今後のエージェントによる試験選択

これは手動・スキル実行時の選択方針。既存CI・公開ゲートの必須条件は変更しない。

| 区分 | 選択条件 | 範囲 |
| --- | --- | --- |
| 通常 | 通常の実装・修正・レビュー | 変更したGoパッケージと利用側、小入力の正常・異常系、安価なGo境界、必要なvet / build |
| 関連変更時 | 上限・引用・直列化・受信・出力・transport・ファイル確定等を変更、または該当要件を新設 | 対応する実プロセス最大値、実SSH、ACL・競合・切断等を追加 |
| 配布前 | 配布物の作成・公開判定、全受入の明示依頼 | 既存の全境界・両OS実SSH・ZIP導入・監査・証跡ゲート |

文書・スキルだけの変更ではリンク・差分・スキル検証を行い、製品試験は起動しない。実装検証でIssueの必須要件に境界・実SSHがある場合はその要件を優先する。未実施は未確認として報告し、通常試験合格を全受入合格と表現しない。

### 現行コードでの選択方法

testing.Short()による切替は未実装のため、-shortだけでは軽量化しない。-runで対象を選ぶ。Windows file_contentは正常・最大値・ACL・競合が単一関数に混在しているため、関連変更時は関数全体を実施する。Windows commandは一部サブテストを選べる。

```powershell
# 共通Go処理（必要なパッケージにさらに絞ってよい）
go test ./internal/config ./internal/inventory ./internal/engine ./internal/result -count=1
# CLI：使い捨てSSH fixtureを設定していない通常環境
go test ./internal/cli -count=1
# モジュールのGo検証：実プロセス・実SSHの親関数を除外
go test ./internal/module -count=1 -skip '^(TestCommandWindows|TestFileContentWindows|TestFileContentLinkSwapWindows|TestCommandSSHLaunchAndDisconnect|TestFileContentSSHLaunchAndDisconnect|TestFileContentSSH|TestLinuxCommandSSH|TestLinuxFileContentSSH)$'
# 小入力によるWindows command代表試験（fixture未設定）
go test ./internal/module -count=1 -run '^TestCommandWindows$/^cmd$/^(CRT_roundtrip|invalid_before_execution)$'
# ファイル受信・確定を変更した場合：現状は最大値等も含む
go test ./internal/module -count=1 -run '^TestFileContentWindows$'
```

実行前にfixture変数を確認する。通常試験のために既存fixture設定を無断上書きせず、独立したプロセス環境を使う。選別コマンドは将来のテスト追加時に見直す。現行CIを軽くするには、別途テスト分割とworkflow変更が必要。

### 関連変更と追加試験の対応

| 変更 | 追加する試験 |
| --- | --- |
| 上限・UTF-8・JSON・変数展開 | 対応する前後境界、最終ホスト・最終タスクでの事前拒否。受信契約変更ならraw / 実SSHも |
| command引用・起動コード | CRT、起動長、子引数限界。LinuxはE2BIG。該当OSの実SSH |
| transport・出力取得 | EOF、両stream大量出力、drain、期限・cancel・disconnect、再送なし、鍵検証 |
| file_content・共通receiver | content / JSON最大値、同値・差分、ACL・リンク・競合・清掃、確定前後の障害。影響OSの実SSH |
| OS選択・backend・engine | OS事前検証、混在模擬、順序・失敗伝播。起動経路に影響すれば該当OSの実SSH |
| 配布・workflow・証跡検査 | 変更したゲートの合成回帰。製品経路・公開要件変更なら対応する実受入 |

同一SHAで既に成功した検証は根拠を確認して再利用し、同じ試験を反復しない。異なるSHAの結果は影響がないと説明できる範囲だけ補助根拠にする。失敗・追加変更・未解決懸念があれば関連範囲を再実行する。
