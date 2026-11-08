# ADR-0001: LinuxをSSH実行対象に追加する

- 状態: 採用（設計決定。Linux実行対応は未実装）
- 日付: 2026-11-08
- 要件: [Issue #23](https://github.com/coco-papiyon/wintable/issues/23)本文と実装計画。コメント0件。
- 調査基準: main `53382ac87653bed136de50ddcf6f644505defa8a`

## 背景と範囲

Windows制御端末からLinuxへ、現行と同じ公開鍵SSH・known_hosts検証・全件事前検証で実行する。初期Linux moduleはcommandのみ。Linux制御端末、sudo/become、package、systemd、再起動、汎用shellは別Issueとする。本IssueはADR・分割案・試験設計を納品し、コード、fixture、CI、子Issueの起票は行わない。

mainはcommandのみ標準CLIに登録済み。file_contentは入力検証のみで、[Issue #10](https://github.com/coco-papiyon/wintable/issues/10)はOPEN・別ブランチ実装中。[Issue #20](https://github.com/coco-papiyon/wintable/issues/20)のモジュール設計はmainへ統合済み。Linux file_contentをWindows実装済みという前提で共通化しない。

## 決定1: 対象環境と依存

| 項目 | 初期対象として採用する範囲 |
| --- | --- |
| 制御側 | 現行Windows 11 / Server 2022以降 amd64、Goはgo.mod固定版 |
| Linux対象 | Debian 13 amd64、通常ユーザー、OpenSSH、POSIX互換の `/bin/sh` ログインシェル |
| receiver | `/usr/bin/python3`、CPython 3.13系、標準ライブラリのみ |
| 実行権限 | SSHユーザーの実効権限。root接続・権限昇格をfixtureの標準にしない |
| 初期検証像 | Debian 13 slim系ベースにsshdとPythonを追加、systemd不要 |

Debianは安定版系列とPython標準依存を揃えやすいため採用。Ubuntuは近い環境で追加価値が小さく、Alpineはmusl・パッケージ・Python差の検証が増えるため初期保証から除外する。第二系列は後続の拡張候補。ARM64は配布バイナリ不要でも実証まで未対応。

コンテナの正確なbase digest、aptパッケージ版、Python patch版はfixture Bで取得・固定する。未取得のdigestを記載しない。B完了条件はDockerfileとlock記録への固定であり、これを満たすまでCの実SSH合格・対応公開Eへ進めない。系列選定は本ADRで確定しておりA/Bの着手を妨げない。

## 決定2: inventoryと互換性

inventory hostにリテラル `os` を追加し、`windows` / `linux` のみ受理、省略はwindows。空文字・未知OS・変数式は入力不正。defaults/groupにはOSを追加しない。tasksと結果JSONのversionは1を維持する。新CLIは既存v1を同じ意味で読む。旧CLIは未知キーを拒否するので、osを含む新inventoryは旧CLI非互換であることを文書化する。新機能を旧CLIへ黙って解釈させないため、この拒否を採用する。v2への一括移行は既存入力を移行させる費用が大きく、今回は採用しない。

接続前にos/module組合せ、ホスト別変数展開、対象パスとサイズを全件検査する。Linux file_content等の未登録組合せは入力不正（終了2）として全ホスト接続0件で拒否する。現在の未登録moduleの内部エラー扱いから、この明示的な能力判定へ移す。接続後は固定receiver自身がLinux・Python系列を照合し、不一致は子起動前にfailed/false。自動OS検出・Windowsへのfallbackはしない。ランタイム不存在など正常応答を得られない場合はfailed/nullとする。

将来入力例（現行CLIでは使用不可、接続用必須項目は既存通り）:

```json
{"os":"linux","address":"127.0.0.1","port":2222,"user":"runner","private_key_file":"keys/id","known_hosts_file":"keys/known_hosts","vars":{"exe":"/usr/bin/printf"}}
```

混在inventoryではWindows hostに `vars.exe=C:\\Windows\\System32\\whoami.exe`、Linux hostに `vars.exe=/usr/bin/whoami` を設定し、共通taskのexecutableを `${exe}`、argvを `[]` とする。変数の優先順位は現行通り。接続鍵パスは制御端末のfilepathで解決するが、対象パスは制御側OSのfilepathへ渡さない。

## 決定3: backend境界と固定receiver

| 現行の変更対象 | 後続実装の責務 |
| --- | --- |
| `internal/config/config.go` / `internal/inventory` | host OSの構造検証、taskは型と未知/重複キーを検査。対象パスの意味検証は展開後へ移す |
| `internal/engine/plan.go` | HostPlanにOS、OS/moduleでbackend選択、全件能力確認後のみ実行可能Planを返す |
| `internal/module/prepare.go` | 共通JSON生成とOS別Validate/Prepare分離。Windows CRT/UTF-16とLinuxバイト制限を分離 |
| `internal/module/wrapper.go` / `command.cs` | Windows実装を保持。Linux固定Python receiverを別資源として追加 |
| `internal/cli/engine.go` / `internal/engine/run.go` | module名のみの登録からOS/moduleの固定起動行・decoder登録へ。未登録を接続前に検出 |
| `internal/transport/ssh.go` / `internal/result` | 原則共用。SSH認証、stdin、制限、結果順・終了値の既存契約を維持 |
| `tests/windows-ssh` / `build.ps1` / 設計書 | Windows回帰維持、Linux試験と起動長の別検査、対応表追加 |

固定Pythonコードを製品へ埋め込み、固定の `/usr/bin/python3 -I -c '<固定コード>'` 起動行として配布する。引用はビルド時の固定コードだけに適用し、タスク値を起動文字列へ一切連結しない。要求は現在同様UTF-8 JSON stdinを一度直列化し、EOF後に検査・実行する。`-I`でPython環境の影響を抑える。SSHが経由するログインシェルは固定起動にのみ使用し、子は `Popen([executable, *argv], shell=False)` で直接起動する。receiverは未知/重複キー・型・UTF-8・NUL・過大要求を再検査し、拒否時に子を起動しない。

Go等の配布バイナリは追加Python依存を減らすが、転送・配置・署名/ハッシュ検証・更新・CPU別ビルドと清掃が新たに必要になるため保留。Pythonは事前導入を要求する代わりに初期のファイル配布機構を不要とする。OS別engine/SSHの複製は結果・キャンセル契約が分岐するため却下。receiver応答の意味は共通でも、OS固有エラーの確認は専用decoderで行う。

## 決定4: Linux command契約

executableは `/` 始まりのPOSIX絶対パス、NUL・制御文字・空・末尾 `/`・`.`/`..` セグメントを拒否する。`.exe`制限、ドライブ、CRT引用は適用しない。PATH探索・暗黙のシェルfallbackはしない。実行可能なバイナリと明示shebang付きスクリプトを許可する。利用者が明示的にシェルをexecutableへ指定することは可能だが、製品がargvをシェルコードに変換しない。argvは空配列・空要素・日本語・引用符・空白・改行・演算子をそのままUTF-8バイトへ変換し、NULと不正Unicodeを拒否する。子stdinは閉じ、cwd指定は提供せずログイン環境を継承する。子環境はSSHセッション由来（Python起動用の隔離指定とは区別）。

共通JSON要求2MiB以下を維持。Linux追加事前上限は各exe/引数のUTF-8バイト数32KiB以下、NUL込み合計64KiB以下、argv要素数4096以下（境界含む）。これは製品上限で、カーネル上限の保証ではない。環境変数・stack制限・ページサイズが対象依存なのでE2BIG/ENOENT/EACCES/ENOEXECは実行時start_failed、failed/false、exit_code=nullとする。Windowsの32,766 UTF-16、固定起動行8,000単位はLinuxに流用しない。Linux固定起動行は8KiB以下のUTF-8という別の製品制限をビルドとpreflightで検査し、対象の実起動はB/Cで証明する。

| 観測 | 結果 |
| --- | --- |
| 正常終了0 | status=changed、changed=true、exit_code=0（非冪等） |
| 子の非0終了 | failed、changed=null、exit_code=子終了値 |
| 子がsignal Nで終了 | failed、changed=null、exit_code=-N（128+Nへ変換しない） |
| 正常なinvalid_request/start_failed応答 | failed、changed=false、exit_code=null |
| 実行後receiver障害、不正応答、外側stderr/非0、応答超過 | failed、changed=null、exit_code=null |

子stdout/stderrはバイナリとして別々に並行回収し、各先頭1MiBを保持、超過後も排出しtruncated=true。応答は既存の5フィールド（exit_code、stdout/stderrのbase64、両truncated）、または単独errorのみ。外側応答3MiB以下、外側終了0・stderr空・単一JSON必須。成功応答を出すreceiverの終了と子終了を混同しない。出力非公開既定、秘密値を診断に載せない。通信断・timeoutはfailed/null、親cancelは実行中cancelled/null・未開始cancelled/false、接続失敗はunreachable/false・後続skipped、ホスト内順序・forks・自動再送なし・CLI終了優先130→3→2→1→0を維持する。切断で子孫停止を保証せず、fixtureはコンテナ全体破棄で清掃する。

## 決定5: file_contentは独立した第二段階

contentはUTF-8、BOM追加なし・改行保持、1MiB以下。バイト一致はok/false、新規（空も含む）・差分はchanged/true。親既存必須、対象・全祖先symlinkと非通常ファイルを拒否する。初期引数はpath/contentのみ、所有者/mode変更は提供しない。既存UID/GIDとmodeを保持できなければ確定前に拒否し、新規はSSHユーザー所有・mode 0600（umaskで広がらない）とする。ACL/xattrを持つ既存ファイルは属性保持を保証する実装ができるまで拒否する。WindowsのDACL/reparse point規則は流用しない。

Linuxではdirfd・no-followで祖先を辿り、同一ディレクトリの排他的tempへ書込・fsync後、既存は原子的replace、新規はno-replaceで確定する方式をDで実証する。パスの事前lstatのみでは競合を防げない。敵対的な祖先変更に対する境界、属性設定順、ディレクトリfsyncと確定APIはD着手時の詳細設計条件とする。比較時点CASは保証せず外部更新は最後の成功置換が残る。確定前の拒否はfailed/false、確定API開始以後や清掃失敗・通信断はfailed/null。実証できない競合・属性条件を対応済みに含めない。

## 実装計画とIssue分割案

各行は未起票の子Issue案。順序はADR → A/B → C → Eでcommand公開 → DとE更新。

| 案・目的 | 範囲 | 依存 | 受け入れ条件 |
| --- | --- | --- | --- |
| A: OSスキーマとbackend境界 | config/inventory/plan、OS別Prepare/登録、互換性文書 | 本ADR | 旧Windows入力・診断の回帰、混在変数展開、未知OS/不正パス/未対応組合せは接続0。v1新旧互換表と境界試験あり |
| B: Linux実SSH fixture | tests/linux-ssh、Dockerfile、固定lock、一時鍵・known_hosts、期限付き起動/清掃ランナー | 本ADR、Aと並行可 | Debian/Python/digest固定、通常ユーザー接続、誤認証・未知/変更鍵拒否、失敗時も専用リソース清掃。版とログ記録 |
| C: Linux command | Python固定receiver、OS確認、argv、decoder、CLI登録 | A/B | 下記L01〜L06を製品CLIと実sshdで合格、子バイト列/結果一致、Windows回帰合格。固定起動長と要求境界を検査 |
| D: Linux file_content | 安全な比較/置換、所有者/mode/属性・競合 | A/B/C、#10統合状況再確認 | L07合格、dirfd/置換方式を設計確定、属性保持と攻撃的リンク競合の証跡。不成立条件は拒否 |
| E: CI・配布・文書 | Windows制御側Linux実SSHジョブ、Windows回帰、サンプルと対応表 | B/C、file_content公開はD後 | 固定Goとimageで再現、出力/清掃証跡保存、失敗/skipを区別。WindowsとLinux試験の両方を公開条件とする |

Linux制御端末のビルド・パス・signal・配布検証、第二ディストリビューション、sudo/systemdは別案でありA〜Eへ暗黙に追加しない。

## コンテナ試験計画

Windows制御側の製品exe → loopbackの動的ポート → Linuxコンテナの実sshd → 固定Python receiver → 子プロセスという経路を通す。Docker Linuxモードまたは専用Linux Dockerホストを使う。後者は認証済みトンネルでWindows loopbackへ転送し、リモート公開ポートを全インターフェイスへ晒さない。Linux側だけのCLI試験はWindows制御側E2Eの代替にしない。Windowsコンテナとの同時稼働は仮定せずジョブを分ける。

fixtureはsshd foreground、パスワード/rootログイン無効、通常runnerユーザー。毎回専用ネットワーク・コンテナ・volume・一時鍵を作り、秘密鍵をimageへ焼かない。Docker 28以降をfixtureの基準とし、公開は `127.0.0.1:<動的port>:22`。ホスト公開鍵はdocker exec等の管理経路で取得し `[127.0.0.1]:port` のknown_hostsへ固定する。未検証のssh-keyscan値を信用しない。ready確認には60秒、試験全体には10分の上限、finallyでログ取得・専用リソースと鍵削除を行う。共有Docker資源のpruneはしない。

| ID / 層 | ケースと観測 | 合否基準 |
| --- | --- | --- |
| L01 単体/preflight | 旧入力、新os、未知/重複キー、混在変数、POSIX/Windowsパス、未対応module、各上限-1/同値/+1 | 不正時すべての接続・副作用0。Windows既存期待値維持 |
| L02 transport | 正常/誤鍵認証、未知/変更host鍵、分割UTF-8・2MiB stdin/EOF、接続再利用、両出力超過 | バイト/hash一致、拒否分類一致、再送0。stdinはreceiverと別の固定echo/hash probeでも観測 |
| L03 command | 空argv/空要素、空白/引用符/日本語/改行/演算子、非UTF-8出力、非0・signal | 専用probeがargvを長さ付きhex出力して期待バイトと比較。出力base64、-N、changed、CLI終了値一致 |
| L04 拒否/プロトコル | 不存在、権限不足、ENOEXEC/E2BIG、不正JSON/重複/過大、OS/Python不一致、不正応答 | 子起動マーカーなし。正常拒否false、応答不能null、sanitized error。製品E2Eと生receiver負例を区別 |
| L05 engine | 複数host/forks、失敗後skipped、送信中/実行中timeout/cancel、sshd切断 | 順序と結果契約一致、期限内解放、自動再送なし、残存子はコンテナ破棄で除去 |
| L06 上限/回帰 | JSON2MiB・argv制限・各出力1MiB・応答3MiB境界、Windows単体/固定起動長/実SSH | 両stream超過でもdeadlockなし、Windows既存期待値一致。合成入力と実CLI入力を分けて記録 |
| L07 file_content後続 | 新規→同値→差分、空/日本語/改行、symlink/祖先link、非通常、mode/UID/GID/ACL、権限、競合/清掃失敗 | 内容・属性・副作用を別probeで観測し決定5と一致。未実証経路は未確認 |

将来ランナー手順（現時点ではコマンド/fixture未実装）:

1. ソースSHAと固定GoでWindows製品をbuildし、`go test ./...`・`go vet ./...`とWindows起動長検査を実行する。
2. lockからLinux imageをbuildし、digest/依存版を照合。専用資源と鍵生成、sshd開始、管理経路のhost鍵でknown_hostsを作る。
3. 期限付きready確認後、L02のprobeとL03〜L06の製品exe試験を実行する。期待JSONを構造比較し、値と型・null・終了値・副作用を照合する。
4. 各ケースで成功/失敗/skip/未実行を記録。finallyを必ず通し、清掃結果も検査する。
5. 同じSHAのWindows実SSH回帰を別ジョブで実行し、両方合格時のみEの対応表を公開する。

証跡: ソースSHA、image digest、OS/architecture/kernel、sshd/Python/Go/Docker版、コマンド、期待/実測、終了値、清掃結果。秘密鍵・入力の秘密・任意出力を公開ログに含めない。コンテナはカーネル共有で、別カーネル、systemd、sudo、全ディストリビューション/実機保証にはならない。混在Windows/Linux実SSHを同一runで実行できなければ模擬混在backendと個別実SSHを使い、混在実機E2Eは未確認と明記する。

## 影響と未決事項

Python導入と指定ログインシェルが追加前提になる。v1の意味は保つがos付き入力は旧CLIで拒否される。Windowsの厳密検証を共通化の都合で緩めない。Linuxの環境依存argv上限は実行時拒否を残し、切断後の停止・変更状態は保証できない。

| 未決事項 | 解決方法 / 阻む段階 |
| --- | --- |
| 正確なdigestとPython/sshd patch版 | Bでregistry取得・依存固定・証跡保存。B完了とC/Eの実証を阻む |
| file_contentの具体的確定API・競合境界 | DでLinux APIとPython標準機能の適用をレビューし競合試験。D完了/公開を阻むがA〜Cは進行可 |
| 混在実機を動かす基盤 | EでWindows/Linuxジョブ配置を実証。混在実機対応宣言を阻む |

## 根拠資料と本Issueの確認

公式資料の確認日: 2026-11-08。

- [Debian 13 release information](https://www.debian.org/releases/trixie/index.en.html): 対象系列の選定根拠。
- [Python 3.13 subprocess](https://docs.python.org/3.13/library/subprocess.html): argv直接実行、バイナリ入出力、POSIX負のsignal終了値。
- [Linux execve(2)](https://man7.org/linux/man-pages/man2/execve.2.html): argv/環境のサイズ制約と起動失敗。Windows引用長とは別の制約。
- [Docker run](https://docs.docker.com/engine/containers/run/)、[port publishing](https://docs.docker.com/engine/network/port-publishing/): digest指定・ポート制限。28未満のlocalhost公開制約を踏まえ基準を設定。

| 要件 | 本書の根拠 |
| --- | --- |
| REQ-001 Linux実装計画 | 決定1〜5、変更対象表、影響と未決事項 |
| REQ-002 Issue分割案 | A〜Eの目的・範囲・依存・受け入れ条件 |
| REQ-003 コンテナ対象テスト方法 | 経路、L01〜L07、手順、合否・証跡・制約 |
| REQ-004 ADR | 背景、決定、却下/保留案、影響、未決事項、検証計画を本ファイルに保存 |

本Issueの検証は要件対応・コードとの照合・リンク整合で行う。Linuxコードやコンテナ試験の合格を意味しない。関連: [全体設計](../design/windows-ansible.md)、[モジュール設計](../design/modules.md)、[README](../../README.md)。
