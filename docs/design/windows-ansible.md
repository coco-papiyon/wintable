# Windows上で動作するGo製構成管理CLIの設計

対象: [Issue #1](https://github.com/coco-papiyon/wintable/issues/1)  
資料確認日: 2026-11-07

## 要件と設計の位置付け

| ID | Issue本文の要件 | 対応箇所 |
| --- | --- | --- |
| REQ-001 | Windows版Ansibleを設計する | 本書の入力・構成・実行・エラー仕様と[検証計画](windows-ansible-validation.md) |
| REQ-002 | Windows上で動作する制御側を作成する方針 | 対象環境、接続、Windows運用 |
| REQ-003 | Go言語を使用する | パッケージ、インターフェース、ビルド・配布 |

Issue #1は設計成果物を対象とした。以下の提案はIssue #3（I-01）の実装で末尾の決定表のとおり初期仕様に採用した。仕様採用と実接続の検証成功は区別する。Ansible本体の移植や完全互換を保証しない。「サーバ」は処理を指示する制御ノードを意味し、管理対象ホストとは別である。

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

対象はOpenSSH Server、TCP 22、公開鍵認証、PowerShell 5.1以上を準備する。SSHの既定シェルはWindowsのcmd.exeに固定して検証し、別シェルは初期対応外とする。リモート起動は固定形式の`powershell.exe -NoLogo -NoProfile -NonInteractive -EncodedCommand <base64>`を使う。固定ラッパーのスクリプトだけをUTF-16LE/base64で符号化する。タスク値はこのコマンドラインへ含めず、下記のSSH標準入力で渡す。PowerShellコードへ直接補間しない。

### タスクデータの転送とサイズ制限

1タスクにつきSSHのexecセッションを1つ開く（SSH接続自体はホスト内で再利用）。exec要求のコマンドは同じモジュールなら全ホスト・全引数で同一の固定ラッパーとし、PTYは要求しない。展開済みのmodule/argsを含む単一JSONオブジェクトを、BOMなしUTF-8でセッションのstdinへ書き、全バイト送信後にstdinだけを閉じてEOFを送る。ラッパーは`Console.OpenStandardInput()`から上限付きでEOFまで読み、厳密UTF-8として復号してJSONを検証し、処理を開始する。既定コードページやPowerShellの行単位読込には依存しない。タスク値を環境変数、EncodedCommand、一時スクリプトへ埋め込まない。

送信とstdout/stderrの読取を並行し、stdin送信・EOF待ちもタスクのtimeoutに含める。途中切断・期限切れは再送せず、変更有無をnullとする。受信側はサイズ超過、不正UTF-8、JSON不正ならモジュールを実行せずfailedを返す。入力データや生の解析エラーをログへ出さない。

以下は初期実装用の上限案であり、全選択ホストの全タスクを変数展開した後、**最初のSSH接続より前**に検査する。いずれかの入力上限超過は終了2、接続0回とし、ホスト名・タスク番号・上限・実サイズだけを診断する。JSONのサイズは空白なしの実際に送るシリアライズ結果で測定し、エスケープによる増加も含める。

| 対象 | 許容上限（上限と同値は受理） | 測定方法 |
| --- | --- | --- |
| `file_content.content` | 1,048,576バイト（1MiB） | 変数展開後のBOMなしUTF-8。書込予定バイト列の長さ |
| 1タスクのstdin JSON全体 | 2,097,152バイト（2MiB） | module/args等を含むUTF-8送信バイト数。受信側も上限+1バイトまでで超過検出 |
| 固定ラッパーのリモート起動行 | 8,000 UTF-16コード単位 | cmd.exe起動の接頭辞・引用符を含む、対応OpenSSH構成で実際に解釈される行全体。base64化後に測定 |
| `command`の子プロセス起動行 | 終端NULを除いて32,766 UTF-16コード単位 | executableの引用符・区切り・CRT規則で符号化した全argvを含む最終行 |

cmd.exeの上限8,191文字に対し、固定起動行には8,000の保守的上限を置く（[Microsoftの制限説明](https://learn.microsoft.com/en-au/troubleshoot/windows-client/shell-experience/command-line-string-limitation)、確認日2026-11-07）。接頭辞を推測して省略せず、後続実装で対象OpenSSH版の起動形式を確認して計数式を固定する。固定ラッパーにはタスク値・パス・環境変数展開を含めない。ラッパー自体が上限を超えた場合は利用者の入力エラーではなく配布物の不備として、接続前に終了3とする。ビルド時と実行時の両方で検査し、実機でも起動を確認する。ラッパーを分割送信する等の未設計の回避動作は行わない。

stdinを使っても`command`が起動する子プロセスには別の長さ制限があるため、制御側は対象と同じCRT引数エンコーダで最終行を事前計算し、受信側も再検査する。32,766の根拠は[CreateProcessWの32,767文字（終端NUL込み）制限](https://learn.microsoft.com/en-us/windows/win32/api/processthreadsapi/nf-processthreadsapi-createprocessw)。cmd.exeやバッチファイル等、さらに短い独自制限・シェル構文を持つ呼出しは初期のcommand対応外とする。10KiBのASCII contentや8,191文字を超えるargvデータ自体はSSH起動行を伸ばさず、上記各上限内なら転送できる。

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
internal/cli          テスト可能な引数検証とExecutorへのcontext引渡し
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

`Host`は検証済み接続情報、`Request`は固定ラッパーの起動行と別フィールドのstdin用UTF-8 JSONバイト列、`Response`は通信状態とプロセス結果を分離する。transportはstdinの送信・EOFと出力の並行読取を担当する。engineはSSH実装を参照せず、模擬Sessionを注入できる。context、標準JSON、flag、signal、同期は標準ライブラリを優先する。

## 実行フローと失敗時動作

1. 入力全体を読込・検証し、ホスト選択と全タスクの変数を確定する。全ホスト分の送信JSON、content、子プロセス起動行のサイズと固定ラッパー起動長を検査する。副作用はまだ起こさない。
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

I-01ではgo.modの言語版を1.27.0、toolchainをGo 1.27.3に固定する。2026-11-07に[公式配布JSON](https://go.dev/dl/?mode=json)の安定版、[サポート方針](https://go.dev/doc/devel/release)、[Windows対応](https://go.dev/wiki/Windows)を確認した。1.27はサポート期間内で、対象OS/amd64はGoのWindows対応範囲に含まれる。標準ライブラリのみで外部依存の最低Go版・ライセンス競合はない。SSH依存は導入時に再確認する。[Go公式導入手順](https://go.dev/doc/install)も参照。

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

### I-01 初期仕様の採否（2026-11-07）

Issue #3 の実装依頼と本文計画に基づき、次を採用する。設計提案からの仕様変更はなく、CLIの省略値と基盤の暫定動作を具体化した。

| 項目 | 採否・理由 | 検証への影響 |
| --- | --- | --- |
| OS | Windows 11 / Server 2022以降のamd64制御・対象を採用。検証対象を限定 | 実機は両OSが必要。Goビルドだけでは合格にしない |
| 接続 | SSH公開鍵、known_hosts必須、cmd.exe既定、PowerShell 5.1以上を採用。固定起動形式で検証可能 | V-05〜10/14/16/18。WinRM/PSRP・他シェルは非採用 |
| 入力・モジュール | 独自JSON v1、静的Inventory、文字列変数、command/file_contentを採用。最小範囲を明確化 | V-01〜03/07〜10/14〜18。読込と実行は後続Issue |
| 上限 | content 1MiB、stdin JSON 2MiB、固定起動行8,000 UTF-16単位、子プロセス32,766単位、各出力保持1MiBを採用。測定・同値受理・超過時の区分は本書どおり | V-11/15〜18。入力超過=2、配布物不備=3。検査実装は後続Issue |
| 非対応 | Ansible YAML/Jinja/collections、roles/handlers/facts/become/vault、動的Inventory、check mode、再試行、ロールバック、Linux対象、arm64、GUIを非採用。初期実装を限定 | CLIで対応しないフラグは終了2。JSON内指定の拒否は後続入力検証 |
| CLI | inventory/tasks必須。forks=5（1〜64）、timeout=60s（正のduration）、format=text（text/json）、include-output=false、limit省略=選択ホスト全体、vars-file省略=CLI変数なしを採用 | helpと単体テストで既定値・型・範囲を検証 |
| Go | 公式情報確認後Go 1.27.3を採用。標準ライブラリのみ | 同版でtest/vet/Windows amd64 buildを行う |

基盤段階では有効なrunも `execution_not_implemented` と終了3を返す。JSON解析・接続・実行・結果描画は未実装で、成功結果は生成しない。`cmd/wintable` はsignal.NotifyContextを作り、`cli.Run` が検証後Executorへ渡す。後続engine統合時にnil Executorを置き換える。

依存方向はcmd → cli → 注入Executor（将来engine）、engine → config/inventory/module/transport/result、module → transport/result、inventory → configとする。transport/resultは上位層に依存しない。未実装パッケージはdoc.goで境界のみを示す。Host/Request/ResponseとConnector/Sessionはtransport、TaskResultはresultに配置した。TaskResultのtask_indexは0始まり、duration_msはミリ秒、changed/exit_codeのnilはJSON null、未実行はchanged=falseとexit_code=null。任意の下位エラーや接続資格情報を結果へ入れない。

実運用対象、ドメイン認証、Playbook移行、WinRM、arm64、署名配布は将来検討として保留し、初期範囲には含めない。後続の順序と合格条件は[検証計画](windows-ansible-validation.md)に記載する。

### I-02 入力検証の具体化

- vars-fileはversionラッパーなしの文字列辞書。Inventoryのdefaults/groups/varsは省略可、hostsは非空で必須。各hostのaddress/user/private_key_file/known_hosts_fileは必須、port省略時は22。tasksのhostsと非空tasks、各taskのname/module/args、module固有の全引数は必須。空のargvとcontent、変数値は許容する。
- 対象Windowsパスはドライブ文字・コロン・バックスラッシュで始まる絶対パスのみ受理する。UNCとデバイス名前空間は初期v1では非対応。対象パスには制御側filepathを適用しない。
- CLIファイル引数は作業ディレクトリから絶対化し、Inventoryの鍵・known_hosts相対参照はInventory配置先から解決する。参照先の存在・ファイル種別・読込権限・鍵形式は後続の認証準備で検査する。JSON検証で鍵内容を読み込まない。
- 診断は固定文言とファイル種別・スキーマ位置を返す。任意の辞書キーは入力順（構文解析）または辞書順（スキーマ検証）の0始まりmember番号で示し、入力本文やキー・値を表示しない。
- サンプルはexamples/inventory.json、tasks.json、vars.json。サイズ上限は変数展開後の後続検査に属し、入力ファイル自体の追加上限は設けない。ホスト選択・名前の衝突・変数展開はIssue #5の範囲。

### I-04 結果出力の契約

`result.Aggregate` はタスク結果のコピーをホスト名の辞書順、0始まりのtask_index順に集計する。
JSONは `{"hosts":[{"host":"win01","tasks":[{"host":"win01","task":"example","task_index":0,"status":"ok","changed":false,"exit_code":0,"duration_ms":1,"stdout_truncated":false,"stderr_truncated":false}],"counts":{"ok":1,"changed":0,"failed":0,"unreachable":0,"skipped":0,"cancelled":0}}],"counts":{"ok":1,"changed":0,"failed":0,"unreachable":0,"skipped":0,"cancelled":0}}` の単一文書。
空結果はhosts=[]、全件数0。件数はstatusを数え、changed値から推測しない。changed/exit_codeのnull、duration_msを保持する。
空のホスト名、不正status、負のtask_index/duration_ms、同じhost/task_indexの重複は内部エラーとし、部分集計を出さない。
テキストも同じ集計結果から全タスク・各ホスト件数・全体件数を出す。

`result.PublicError` が許可するコードは input_error、connection_failed、authentication_failed、host_key_failed、task_failed、deadline_exceeded、connection_lost、cancelled、internal_error、output_failed、execution_not_implemented。
transport/moduleは既知の原因をこのコードへ分類し、下位のerror.Error()を渡さない。未知コードや未知errorはinternal_errorへ変換する。タスク名・ホスト名は結果識別子として表示する。
`result.Finish` は描画・定型診断・終了判定をまとめる。失敗/unreachableは1、input_errorは2、内部/出力障害は3、cancelled結果・親contextキャンセルは130。deadline_exceededだけでは130にしない。
`result.ExitCode` は130→3→2→1→0を適用し、不正なExecutor終了値は3にする。CLIのhelp/診断の短い書き込みも出力障害として扱う。
書き込み失敗では途中のJSONが残り得る。同じstdoutへ代替JSONや診断を追記しない。

`result.NewCapture(includeOutput)` はストリーム別に使うWriter。保持上限は1,048,576バイト、同値ではtruncated=false、超過後も全バイトを受け取って破棄する。
既定では本文を保持せず、描画時にも既存の本文フィールドを除去する。明示指定時は不正UTF-8を含む生バイトをbase64化し、stdout/stderrを独立に制限する。描画境界でも既存base64を検査・上限に切り詰める。不正base64は内部エラー。
任意コマンド出力の秘密の自動マスキングは保証しない。

`cli.RunResults` は検証済みOptionsを受けるResultExecutorからタスク配列と分類済みerrorを取得し、指定format/include-outputでFinishへ渡す。既存Runの整数Executorも維持する。
実エンジンの接続は#7の範囲であり、実バイナリのnil Executorは引き続き終了3。
