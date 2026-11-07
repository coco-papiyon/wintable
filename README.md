# wintable

Windowsネイティブで動作するGo製の構成管理CLIです。CLI基盤、JSON v1の厳密な入力検証、ホスト選択・変数展開・全件事前検証、結果集計・描画を実装済みです。SSH接続・モジュール実行は後続Issueで実装します。

- [Windows版Ansible相当ツールの設計](docs/design/windows-ansible.md)
- [検証計画と後続実装](docs/design/windows-ansible-validation.md)

初期仕様はWindows 11 / Windows Server 2022以降のamd64制御・対象、SSH公開鍵認証とknown_hosts必須、cmd.exe既定シェル、PowerShell 5.1以上、独自JSON v1、command/file_contentです。採否・上限・非対応範囲は設計書のI-01決定表を参照してください。

Go **1.27.3** に固定しています（外部依存なし）。Goの自動ツールチェーン取得には初回ネットワークアクセスが必要です。既存のGo 1.22.5ではビルドせず、go.modに従うツールチェーンを使用してください。

```powershell
$env:GOTOOLCHAIN = 'go1.27.3'
go version
go test ./...
go vet ./...
# 別OS上での開発時も対象はWindows amd64。専用シェル内で設定する。
$env:GOOS = 'windows'
$env:GOARCH = 'amd64'
$env:CGO_ENABLED = '0'
go build -trimpath -o dist/wintable.exe ./cmd/wintable
.\dist\wintable.exe help
.\dist\wintable.exe run --inventory inventory.json --tasks tasks.json
```

helpは終了0、不正引数は終了2。入力ファイルも検証し、不正入力は接続前に終了2となります。[サンプル](examples/)を参照してください。有効なrunも現段階では `execution_not_implemented` を標準エラーに出して終了3となり、ネットワークにはアクセスしません。`internal/cli.Run` のExecutor引数を後続engineへ接続する箇所として用意しています。Ctrl+Cのcontextを伝播します。

`--include-output` は既定falseです。有効にすると任意コマンドの出力に秘密が含まれ得ます。完全な自動マスキングは保証しません。実接続・PowerShell互換性・Windows CI・ZIP配布は未検証・未実装です。

結果集計・JSON／テキスト描画・終了判定は `internal/result` に実装済みです。
`cli.RunResults` に結果を返すExecutorを注入すると、ホスト名／task_index順の単一JSON文書またはテキストをstdoutへ、定型診断をstderrへ出します。
終了値の優先順位は130→3→2→1→0。出力障害は3、キャンセル併発時は130です。
`--include-output` 時のみ、各ストリーム先頭1MiBをbase64で含め、超過はtruncatedで示します。
実エンジン統合は後続Issueのため、通常の実バイナリ実行は引き続き終了3です。

## ホスト実行engine（I-05）

`engine.Runtime.Execute` は検証済みPlan、Connector、固定起動行、モジュール応答解釈器を受け取ります。辞書順キューから最大forksホストを処理し、各ホストの一接続をタスク定義順で再利用します。割当順は辞書順ですが、並列Connectの呼出し・完了順は保証しません。
接続は10秒、各タスクは接続後に独立したtimeoutを持ちます。失敗ホストの後続はskipped、他ホストは続行し、自動再試行しません。
接続失敗は先頭unreachable・後続skipped（changed=false）。実行中の通信断・期限切れはfailed・changed=null。親キャンセル時は確定済み結果を保持し、実行中はcancelled・null、未開始はcancelled・falseです。結果はホスト名・task_index順です。Close失敗は内部エラーですが、終了130が優先します。
Ctrl+Cで新規処理を止め接続を解放します。切断で対象プロセス停止は保証できません。変更不明時は対象状態を確認してから手動で再実行してください。
`cli.RunEngine` が全件事前検証から結果描画まで接続します。SSHと実モジュールは後続Issueのため、本番mainは引き続き未実装終了3です。模擬V-04/06を実機SSH検証の合格とは扱いません。

SSH接続の設定・依存監査・検証範囲は [SSH transport](docs/ssh-transport.md) を参照してください。
