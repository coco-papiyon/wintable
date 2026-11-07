# wintable

Windowsネイティブで動作するGo製の構成管理CLIです。CLI基盤、JSON v1の厳密な入力検証、結果集計・描画を実装済みです。SSH接続・モジュール実行は後続Issueで実装します。

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
