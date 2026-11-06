# Windows上で動作するGo製構成管理CLIの設計

対象: [Issue #1](https://github.com/coco-papiyon/wintable/issues/1)  
資料確認日: 2026-11-07

## 要件と設計の位置付け

| ID | Issue本文の要件 | 対応箇所 |
| --- | --- | --- |
| REQ-001 | Windows版Ansibleを設計する | 本書の入力・構成・実行・エラー仕様と[検証計画](windows-ansible-validation.md) |
| REQ-002 | Windows上で動作する制御側を作成する方針 | 対象環境、接続、Windows運用 |
| REQ-003 | Go言語を使用する | パッケージ、インターフェース、ビルド・配布 |

本Issueでは実行コードを作らず、後続実装が参照できる設計を成果物とする。以下は初期実装の提案仕様であり、利用者が承認した製品要件とは区別する。Ansible本体の移植や完全互換を保証しない。「サーバ」は処理を指示する制御ノードを意味し、管理対象ホストとは別である。

## 対象環境と互換範囲

初期案はWindows 11 / Windows Server 2022以降のamd64制御ノードから、同じOS範囲のWindows管理対象を操作する単発CLI。WSL、Python、コンテナ、常駐サービス、DBを必須にしない。通常ユーザーで起動し、対象操作に必要な権限は接続ユーザーに与える。自動昇格は行わない。arm64、Linux管理、GUI、スケジューラは後続候補。

| 機能 | 初期案 |
| --- | --- |
| Inventory / タスク | 独自JSON v1、静的ホストとグループ、順次タスク |
| 変数 | Inventory defaults → group vars → host vars → CLI varsの順で上書き |
| モジュール | `command`、UTF-8テキストを管理する`file_content` |
| Ansible YAML / Jinja / collections | 非対応。変換なしで読まない |
| roles / handlers / facts / become / vault / 動的Inventory | 非対応 |
| check mode / 自動再試行 / ロールバック | 非対応。指定時は入力エラー |

公式AnsibleはWindowsネイティブ制御ノードをサポートしないため、本ツールは独立した制御側をGoで作る。Windows管理ではSSH、PSRP、WinRMが選択肢となる。[Ansibleインストール要件](https://docs.ansible.com/projects/ansible/latest/intro_installation.html)、[Windows管理](https://docs.ansible.com/projects/ansible/latest/os_guide/intro_windows.html)を参照。

## 接続方式

| 候補 | 利点 | 制約と判断 |
| --- | --- | --- |
| SSH | 公開鍵認証、Goクライアント、ホスト鍵照合を共通化できる | 対象にOpenSSH Serverが必要。初期案に採用 |
| WinRM / PSRP | Windowsの既存リモート管理と統合しやすい | 認証方式、証明書、ドメイン環境の検証が増えるため後続候補 |

対象はOpenSSH Server、TCP 22、公開鍵認証、PowerShell 5.1以上を準備する。SSHの既定シェルはWindowsのcmd.exeに固定して検証し、別シェルは初期対応外とする。リモート起動は固定形式の`powershell.exe -NoLogo -NoProfile -NonInteractive -EncodedCommand <base64>`を使う。スクリプトはUTF-16LEで符号化する。タスク値はJSONとして符号化して固定ラッパー内で復元し、PowerShellコードへ直接補間しない。

Go依存候補は`golang.org/x/crypto/ssh`と`ssh/knownhosts`。導入時にバージョン、脆弱性、ライセンスを確認してgo.mod/go.sumへ固定する。WinRM採用時はHTTPSと証明書・ホスト名検証を必須とし、検証無効化は提供しない。現時点ではWinRMライブラリを採用しない。[Windows SSHの前提](https://docs.ansible.com/projects/ansible/latest/os_guide/windows_ssh.html)も参照するが、Ansibleの対応宣言を本製品の実機検証の代わりにしない。

## 入力とCLI

以下は後続実装用の完全な入力例。すべてUTF-8、JSONの未知フィールド・重複キー・末尾データをエラーにする。YAMLは受け付けない。

Inventory `inventory.json`:

```json
{
  "version": 1,
  "defaults": {"vars": {"message": "hello"}},
  "groups": {"windows": {"hosts": ["win01"], "vars": {}}},
  "hosts": {
    "win01": {
      "address": "win01.example.test",
      "port": 22,
      "user": "automation",
      "private_key_file": "./keys/automation",
      "known_hosts_file": "./known_hosts",
      "vars": {"message": "こんにちは"}
    }
  }
}
```

タスク定義 `tasks.json`:

```json
{
  "version": 1,
  "hosts": "windows",
  "tasks": [
    {"name": "接続確認", "module": "command", "args": {"executable": "C:\\Windows\\System32\\whoami.exe", "argv": []}},
    {"name": "メッセージを配置", "module": "file_content", "args": {"path": "C:\\Temp\\wintable-message.txt", "content": "${message}\n"}}
  ]
}
```

起動例（未実装）:

```powershell
.\wintable.exe run --inventory .\inventory.json --tasks .\tasks.json --limit win01 --forks 5 --timeout 60s --format json
```

`hosts`はホスト名、グループ名、予約語`all`の完全一致のみ。ホスト名とグループ名の衝突、`all`という定義名は禁止。`--limit`はカンマ区切りの完全一致ホスト名で、選択集合との積を取る。未定義名、空集合、重複定義、存在しないグループメンバーは接続前にエラー。複数グループに属するホストはグループ名の辞書順で変数を上書きし、ホストの実行は一度だけ。

変数は文字列のみ、名前は`[A-Za-z_][A-Za-z0-9_]*`。`${name}`を引数値で一回だけ展開し、`$${`はリテラル`${`。未定義は接続前エラー。キー、ホスト名、接続情報、モジュール名は展開しない。CLIの`--vars-file`は文字列辞書JSONを読み、最後に上書きする。秘密を変数で運ばない。

Inventory内の相対パスはInventoryのディレクトリ、CLIファイル引数は作業ディレクトリを基準とする。対象の`path`と`executable`はWindows絶対パスのみ。制御側の`filepath`で対象パスを変換しない。`--forks`は1〜64（既定5）、`--timeout`は正の時間（既定60秒）。引数の型、必須値、モジュール固有の未知引数を全ホスト分検証してから接続する。

## パッケージと責務

```text
cmd/wintable          CLIフラグ、シグナル、終了コード
internal/config       JSON厳密読込、スキーマ検証
internal/inventory    選択、変数マージ・展開
internal/engine       並列制御、ホスト内順序、集計
internal/transport    SSH接続、ホスト鍵、セッション寿命
internal/module       command、file_content、PowerShellラッパー
internal/result       結果型、JSON/テキスト出力、出力制限
```

Goの境界案（実装時に詳細型を定義）:

```go
type Connector interface {
    Connect(context.Context, Host) (Session, error)
}
type Session interface {
    Run(context.Context, Request) (Response, error)
    Close() error
}
type Module interface {
    Validate(map[string]any) error
    Execute(context.Context, Session, map[string]any) TaskResult
}
```

`Host`は検証済み接続情報、`Request`は固定ラッパーと符号化済みデータ、`Response`は通信状態とプロセス結果を分離する。engineはSSH実装を参照せず、模擬Sessionを注入できる。context、標準JSON、flag、signal、同期は標準ライブラリを優先する。

## 実行フローと失敗時動作

1. 入力全体を読込・検証し、ホスト選択と全タスクの変数を確定する。副作用はまだ起こさない。
2. ホスト名の辞書順のキューから最大forks個を処理する。各ホストは一接続を開き、タスクを定義順で実行する。
3. 接続タイムアウトは10秒。各タスクは接続後からtimeoutを適用し、出力読込も期限に含める。
4. 失敗したホストの後続タスクはskippedにし、他ホストは続行する。接続はdefer相当で必ず閉じる。
5. Ctrl+Cで新規処理を止め、セッションを閉じ、未完了タスクをcancelledとして集計する。切断で対象プロセスの停止を保証できないため、途中結果は状態不明として記録する。
6. 全ホスト結果をホスト名・タスク順に集計し、終了する。自動再試行はしない。タイムアウト後の再実行は利用者が対象状態を確認して行う。

結果はホスト/タスク名、`status`（ok/changed/failed/unreachable/skipped/cancelled）、`changed`（true/false/null）、プロセス終了コード（取得不能時null）、経過時間、エラー分類を持つ。通信断・期限切れで変更有無が不明ならchanged=null。接続・認証・ホスト鍵失敗はunreachable、接続後のモジュール失敗はfailed。未実行はchanged=false。ホスト集計と全体集計は各statusの件数を返す。

終了コード: 0=全タスク成功（changedも成功）、1=failed/unreachableあり、2=入力・CLIエラー、3=制御側内部エラーまたは結果出力失敗、130=キャンセル。競合時の優先順位は130→3→2→1→0。JSONは標準出力に単一文書、診断は標準エラーへ出す。テキスト出力でも判定は同じ。

## 最小モジュール

`command`: executableと文字列配列argvを受け取る。シェル構文は解釈せず、対象で.NET ProcessStartInfoを使って起動する。PowerShell 5.1にはArgumentListがないため、Windows CRT規則に従う引数エンコーダを実装し、空引数、空白、引用符、末尾バックスラッシュを検証する。独自引数解析を行う実行ファイルは対応外として明記する。終了0はchanged=true、非0はfailedかつchanged=null。コマンドは冪等性を保証しない。

`file_content`: pathとcontentのみ。親ディレクトリは既存必須。UTF-8 BOMなしの期待バイト列と既存ファイルを比較し、一致ならok/changed=false。差分があれば同一ディレクトリに一時ファイルを作り、既存ファイルは置換、新規は移動してchanged=true。改行は入力通りで自動CRLF変換しない。ディレクトリ、reparse point、symlinkは拒否する。ACL変更・バイナリ・所有者管理は非対応。置換に失敗したらfailedにし、一時ファイルを可能な限り削除する。ロールバックや複数ホスト間の原子性は保証しない。既存ACL保持と同時更新はWindows検証の受け入れ条件とする。

## 認証とWindows運用

秘密鍵はファイル参照のみ。known_hostsによる照合を必須にし、未知・変更鍵を自動登録しない。パスフレーズは環境変数`WINTABLE_KEY_PASSPHRASE`から取得し、値をCLI引数・結果・ログへ出さない。接続設定の丸ごと出力は禁止し、鍵内容やパスフレーズを含む下位エラーは定型コードへ変換する。利用者は秘密鍵と環境へのアクセスをOS権限で制限する。

任意コマンドの出力に秘密が含まれ得るため、既定ではstdout/stderr本文を結果へ含めない。明示的な`--include-output`時のみ出力し、この選択をヘルプに説明する。各ストリーム最大1MiBを保持し、超過分は破棄してtruncated=true。読み取り自体は継続してデッドロックを防ぐ。任意出力の完全な自動マスキングは保証しない。

設定はUTF-8（先頭BOMは許容）、LF/CRLFを許容。PowerShellラッパーの応答はUTF-8 JSON。外部コマンドの出力はバイト列として回収し、出力要求時はbase64で格納するため、コードページによる破損を避ける。日本語や空白を含む制御側・対象側パスを検証する。管理対象にはwintableエージェントを導入しないが、OpenSSHとPowerShellの事前準備が必要。

## ビルドと配布

後続実装はGo 1.27系の最新修正版を候補とし、採用時点のサポートと依存互換性を再確認してCIで固定する。現時点ではgo.modもバイナリもない。[Go公式導入手順](https://go.dev/doc/install)、[Windows対応](https://go.dev/wiki/Windows)、[配布版](https://go.dev/dl/)を参照。

実装後のPowerShell用ビルド案:

```powershell
go test ./...
go vet ./...
$env:GOOS = 'windows'
$env:GOARCH = 'amd64'
$env:CGO_ENABLED = '0'
go build -trimpath -o dist/wintable.exe ./cmd/wintable
```

Windows CIでテスト後、exe、サンプルJSON、利用手順、依存ライセンス、SHA-256をZIP配布する。導入は展開と任意のPATH登録、更新は実行停止後のexe置換。管理者権限をインストール条件にしない。Go本体と依存のライセンスを一覧化し、脆弱性スキャンを配布条件にする。

## 決定事項と未確定事項

Issueで確定しているのは設計、Windows制御側、Goの3点。独自JSON、SSH、対象OS、モジュール、CLI仕様は本書の提案として一貫した最小案を示した。後続実装開始時に利用者と初期範囲を確定する。実運用の管理対象、ドメイン認証、既存Playbook移行、WinRM、arm64、署名配布は未確定。後続の順序と合格条件は[検証計画](windows-ansible-validation.md)に記載する。
