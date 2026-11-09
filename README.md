# wintable

Windowsネイティブで動作するGo製の構成管理CLIです。JSON v1の厳密な入力検証、全件事前検証、SSH公開鍵接続、commandモジュール、結果集計・描画を実装しています。Windows/Linuxのcommand・file_contentと、Windows制御端末へファイルを取得するfetchを実装しています。

- [ZIPの導入・SSH準備・利用手順](docs/usage.md)
- [fetchによるリモートファイル取得・保存先・上書き方針](docs/fetch-module.md)
- [Windows CI・監査・ZIP作成と公開条件](docs/release.md)

Windows CIと配布候補生成を整備しています。[Issue #11の全必須検証は合格済み](https://github.com/coco-papiyon/wintable/issues/11#issuecomment-6717454311)です。本体は[MITライセンス](LICENSE)です。Go・依存ライブラリのライセンスと通知も配布物に保持します。[最終ZIPのV-13と配布commitの全18シナリオを確認済み](docs/validation/issue12-audit-fixed/README.md)です。公開前に実ログ・最新監査・ライセンスの人手レビューを行います。

- [Windows版Ansible相当ツールの設計](docs/design/windows-ansible.md)
- [検証計画と後続実装](docs/design/windows-ansible-validation.md)
- [Windowsコンテナの証跡収集と未確認項目](docs/validation/windows-container.md)

初期仕様はWindows 11 / Windows Server 2022以降のamd64制御・対象、SSH公開鍵認証とknown_hosts必須、cmd.exe既定シェル、PowerShell 5.1以上、独自JSON v1、command/file_contentです。採否・上限・非対応範囲は設計書のI-01決定表を参照してください。

Go **1.27.3** に固定しています。SSHにはgo.modで固定したgolang.org/x/cryptoを使用します。Goの自動ツールチェーン取得には初回ネットワークアクセスが必要です。go.modに従うツールチェーンを使用してください。

```powershell
$env:GOTOOLCHAIN = 'go1.27.3'
go version
go test ./...
go vet ./...
# 別OS上での開発時も対象はWindows amd64。専用シェル内で設定する。
$env:GOOS = 'windows'
$env:GOARCH = 'amd64'
$env:CGO_ENABLED = '0'
.\build.ps1 # 固定ラッパーの起動長検査を含む
.\dist\wintable.exe help
.\dist\wintable.exe run --inventory inventory.json --tasks tasks.json
```

helpは終了0、不正引数は終了2。入力ファイルも検証し、不正入力は接続前に終了2となります。[サンプル](examples/)を参照してください。commandとfile_contentのrunは実際にSSHへ接続します。file_contentはBOMなしUTF-8のバイト比較を行い、差分があるときだけ安全な置換を行います。詳細は[file_contentモジュール](docs/file-content-module.md)を参照してください。Ctrl+Cのcontextを伝播します。

`--include-output` は既定falseです。有効にすると任意コマンドの出力に秘密が含まれ得ます。完全な自動マスキングは保証しません。commandの実機検証範囲と未確認事項は [commandモジュール](docs/command-module.md) を参照してください。CIとZIP配布の条件は [CI手順](docs/ci.md)、保証範囲は [対応表](docs/support.md) を参照してください。

結果集計・JSON／テキスト描画・終了判定は `internal/result` に実装済みです。
`cli.RunResults` に結果を返すExecutorを注入すると、ホスト名／task_index順の単一JSON文書またはテキストをstdoutへ、定型診断をstderrへ出します。
終了値の優先順位は130→3→2→1→0。出力障害は3、キャンセル併発時は130です。
`--include-output` 時のみ、各ストリーム先頭1MiBをbase64で含め、超過はtruncatedで示します。
通常の実バイナリも同じengineと結果描画を使います。

## ホスト実行engine（I-05）

`engine.Runtime.Execute` は検証済みPlan、Connector、固定起動行、モジュール応答解釈器を受け取ります。辞書順キューから最大forksホストを処理し、各ホストの一接続をタスク定義順で再利用します。割当順は辞書順ですが、並列Connectの呼出し・完了順は保証しません。
接続は10秒、各タスクは接続後に独立したtimeoutを持ちます。失敗ホストの後続はskipped、他ホストは続行し、自動再試行しません。
接続失敗は先頭unreachable・後続skipped（changed=false）。実行中の通信断・期限切れはfailed・changed=null。親キャンセル時は確定済み結果を保持し、実行中はcancelled・null、未開始はcancelled・falseです。結果はホスト名・task_index順です。Close失敗は内部エラーですが、終了130が優先します。
Ctrl+Cで新規処理を止め接続を解放します。切断で対象プロセス停止は保証できません。変更不明時は対象状態を確認してから手動で再実行してください。
`cli.RunEngine` が全件事前検証から結果描画まで接続します。本番mainは固定command/file_contentラッパーとSSHConnectorを使います。模擬V-04/06を実機SSH検証の合格とは扱いません。

SSH接続の設定・依存監査・検証範囲は [SSH transport](docs/ssh-transport.md) を参照してください。

関連: [モジュール一覧と設計](docs/design/modules.md)。

Linux対象の追加設計: [ADR-0001: LinuxをSSH実行対象に追加する](docs/adr/0001-linux-target-support.md)。実装と対応公開の状態は [対応表](docs/support.md) を参照してください。

## inventory OSとCLI互換性

hostの `os` はリテラル `windows` / `linux` のみ。省略は `windows`。
空文字・null・変数式・未知値、defaults/groupのosは拒否する。
対象パスはホスト別変数展開後にOS/module backendが検査する。
制御側の鍵・known_hostsのパス解決とは独立している。

| inventory v1 | OS追加前CLI | 本CLI |
| --- | --- | --- |
| os省略 | Windows両module | Windows両module（従来と同じ） |
| os: windows | 未知キーで終了2 | Windows両module |
| os: linux | 未知キーで終了2 | command/file_content（file_contentは祖先権限等の制約あり） |

inventory/tasksのversion=1、結果形式は維持する。Linux command backendを実装済み。
対象はDebian 13 amd64、OpenSSH、POSIX互換/bin/sh、/usr/bin/python3のCPython 3.13系。
Windows制御端末から使用する。Pythonは事前導入が必要で自動導入しない。Linux file_contentは実装済み。安全性の条件は [Linux file_content](docs/linux-file-content.md) を参照。対応公開は同一SHAの両OS必須試験と集約ゲート合格を条件とする。標準runnerを先に検証し、Linux実SSH等は既存PCで必要時に実行する。手順は [CI](docs/ci.md) を参照。
実行契約と検証範囲は [Linux command](docs/linux-command.md) を参照。
詳細は [backend境界](docs/backend-boundary.md) を参照。

Linux例: `examples/linux-inventory.json` / `examples/linux-command.json`。
混在例: `examples/mixed-inventory.json` / `examples/mixed-command.json`。
接続情報を編集し、次のように実行します。

```powershell
./dist/wintable.exe run --inventory examples/linux-inventory.json --tasks examples/linux-command.json --format json
```

[対応表・制限](docs/support.md) / [CI・配布・試験手順](docs/ci.md)。
混在単一runの実機E2Eは未確認。切断後の子孫停止は保証しません。

fetchの通信失敗時は、ローカル確定前ならchanged=false、確定後ならtrueを保持します。取得内容は --include-output でも結果へ出しません。詳細は[fetch](docs/fetch-module.md)を参照してください。
