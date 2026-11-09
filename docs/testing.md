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

## エージェントによる試験範囲（2026-11-10改訂）

実装・修正・通常レビューの完了条件は **全Go試験通過とLinux実SSH合格** とする。実装のたびに配布物の全受入まで進めない。文書・スキルのみの変更は参照・差分・形式の確認でよい。

| 区分 | 必須範囲 | 完了後の扱い |
| --- | --- | --- |
| 実装・修正・通常検証 | go test ./... -count=1 とLinux実SSH（transportおよび製品モジュール） | この範囲の結果を報告して終了 |
| 配布物作成 | 全Go試験、両OS実SSH、複数CLIの並列取得試験、監査、license・ZIP内容・ハッシュ・導入・証跡ゲート | 同一SHA・配布exe・ZIPを対応付け、全必須結果を確認 |
| 文書・スキルのみ | 差分、参照先、スキル形式 | 製品試験は不要 |

通常実装ではWindows実SSH、複数CLIによる並列取得の実機試験、govulncheck、ZIP生成・検査、配布受入集約を起動しない。新機能や共通コード変更であっても、自動的に配布検証へ拡大しない。Issueに配布向け受入条件があっても、通常実装段階では配布時確認として区別して報告する。ユーザーが今回の作業で明示的に全受入を指定した場合はその指定に従う。

全Go試験に含まれる境界・最大値・模擬並列テストは実行する。-skipで全Go試験を省略しない。別途の最大値・障害注入は該当コード修正の確認に必要なものだけ実施し、Windows実SSHや配布一式の実施条件にはしない。現行の-shortは軽量化スイッチではない。

Windows実プロセス試験は、子プロセスから一時ディレクトリとその祖先を検査し、ファイルを書き込める実行環境を必要とする。エージェントのサンドボックス内で `TestFileContentWindows` が `write_failed` になった場合は、環境の承認付き実行経路で同じ試験を再実行して権限制約と製品不具合を切り分ける。承認付き実行は管理者実行を意味しない。ACLの変更や試験のskipで回避せず、両方の実行条件と結果を記録する。必要な承認を得られず再実行できない場合は未確認とする。

### 通常実装の実行方法

```powershell
# Windows fixtureが未設定の独立プロセスで実行（既存環境を無断変更しない）
go test ./... -count=1
# ZIP・監査を作成せずLinux製品試験に必要なexeだけビルド
go build -o dist/wintable.exe ./cmd/wintable
# transportに加え製品command / file_contentも確認する
pwsh -NoProfile -File tests/linux-ssh/run.ps1 -DockerContext desktop-linux -ProductTestScript tests/linux-ssh/product.ps1
```

Linux runnerだけの起動ではtransportのみのため、製品試験スクリプトも指定する。新モジュールのLinux実SSHケースが既存product.ps1の対象に含まれない場合は、そのケースをLinux fixture上で実施する経路を追加・選択する。接続成功だけをLinux製品試験合格と扱わない。

product.ps1は文書例を含み、配布manifestを要求する場合がある。未梱包exeで実行する際は現在の呼出し契約を確認し、必要ならLinux製品試験用の独立した経路を整備する。通常実装を通すためにdistribution.ps1 / package.ps1 / complete-local.ps1を呼び、監査・ZIP作成へ進めない。必要なLinux試験が実行できなければ未確認として報告する。

### 配布物作成時

[scripts/distribution.ps1](../scripts/distribution.ps1)による監査・license・ZIP検査と、[CI手順](ci.md)の両OS実SSH・同一SHAの証跡集約を実施する。複数CLIを同時に起動する並列取得では保存先の衝突、内容一致、各プロセスの結果・終了値、一時ファイル清掃を確認する。Go内の模擬並列試験で代用しない。対象機能のケース・実行方法が未整備なら配布検証未完了と報告する。

既存CIは実装時の手動確認とは別の自動実行であり、この文書変更ではworkflowの起動条件を変更しない。自動CIの監査等が起動しても、実装エージェントが同じ配布検証を手動で重ねる必要はない。配布物作成成功を公開操作の許可と扱わない。

### 結果の記録

対象SHA、全Go試験、Linux実SSHのコマンド・結果と未確認範囲を報告する。配布時のみの項目は「配布物作成時に実施」と記載し、通常実装の不合格理由にしない。必要な全Go試験またはLinux実SSHが未確認なら実装確認完了にしない。通常確認合格を配布可・両OS全受入合格と表現しない。同一SHAの成功証跡は範囲を確認して再利用し、失敗・追加変更・未解決懸念がなければ反復しない。

## ローカル委譲commandの試験

全Go試験には `TestDelegationSchema`、`TestLocalCommandWindows`、`TestDelegateCLIWindows` を含みます。Windows実プロセスでCRT引数・32,766 UTF-16上限・EOF・大量出力・起動失敗・期限切れ・中断と子孫清掃を確認し、CLIで元hostごとの実行、変数展開、全件事前検査、SSH混在順序とskipを確認します。Linux実SSHでは既存のcommand/file_content/fetchへの回帰を確認します。
