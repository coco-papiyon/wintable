# モジュール一覧と設計

## 基準と読み方

[Issue #20](https://github.com/coco-papiyon/wintable/issues/20)の設計成果物。確認日は2026-11-08、基準mainは `307b8329359c60ffba47a18b2fe883e3212f72e9`。入力スキーマや実行コードは変更しない。

- **実行対応済み**: 標準CLIに固定ラッパーと応答解釈が登録されている。全OSの実機検証済みという意味ではない。
- **入力検証のみ**: スキーマと送信準備はあるが標準CLI実行経路に未登録。
- **実装中**: 別ブランチに実装がありmain未取り込み。
- **候補**: 今後の設計案。P1（基本構成）、P2（運用拡張）、P3（基盤拡張後）は優先度案で、実装確約ではない。
- **保留／対象外**: 追加調査が必要／Windows向け初期範囲から除外。

現行仕様の正本は[全体設計](windows-ansible.md)、[command詳細](../command-module.md)、検証範囲の正本は[検証計画](windows-ansible-validation.md)。本書は対応状況と将来設計の入口とする。

## Ansibleの代表用途一覧

「一般的」はWindows構成管理の代表用途を選んだもので、全モジュールの完全一覧や利用頻度ランキングではない。公式索引の確認版は [ansible.builtin 2.21.6.post0](https://docs.ansible.com/projects/ansible/latest/collections/ansible/builtin/index.html)、[ansible.windows 3.9.2](https://docs.ansible.com/projects/ansible/latest/collections/ansible/windows/index.html)、[community.windows 3.3.0](https://docs.ansible.com/projects/ansible/latest/collections/community/windows/index.html)。以下のFQCNは個別公式仕様へリンクする。wintableは独自JSONでありAnsibleとの引数・戻り値の完全互換を目標にしない。

| 用途 | Ansible FQCN（公式） | wintable名 | 状態・優先度と理由 | 互換性・差異 |
| --- | --- | --- | --- | --- |
| プロセス | [ansible.windows.win_command](https://docs.ansible.com/projects/ansible/latest/collections/ansible/windows/win_command_module.html) | command | 実行対応済み | CRT対応exeのみ。creates/removes/chdirなし |
| PowerShell | [ansible.windows.win_powershell](https://docs.ansible.com/projects/ansible/latest/collections/ansible/windows/win_powershell_module.html) | powershell | 保留：任意コードの結果契約が必要 | スクリプトのchanged自己申告を現行に導入しない |
| シェル | [ansible.windows.win_shell](https://docs.ansible.com/projects/ansible/latest/collections/ansible/windows/win_shell_module.html) | shell | 保留：引用と実行制限の再設計 | commandでシェル構文を解釈しない |
| 内容配置・転送 | [ansible.windows.win_copy](https://docs.ansible.com/projects/ansible/latest/collections/ansible/windows/win_copy_module.html) | file_content / copy | 前者は実行対応済み、後者P2候補 | contentのみ。バイナリ・ディレクトリ転送は別設計 |
| ファイル・ディレクトリ | [ansible.windows.win_file](https://docs.ansible.com/projects/ansible/latest/collections/ansible/windows/win_file_module.html) | file | P1候補：配置先を準備 | touch・再帰削除は初期案から除外 |
| テンプレート | [ansible.windows.win_template](https://docs.ansible.com/projects/ansible/latest/collections/ansible/windows/win_template_module.html) | template | 保留：描画言語と秘密管理が必要 | 現行の文字列変数置換はJinja2ではない |
| 行編集 | [ansible.windows.win_lineinfile](https://docs.ansible.com/projects/ansible/latest/collections/ansible/windows/win_lineinfile_module.html) | line | P2候補：既存設定の局所変更 | 初期案はリテラル完全一致のみ |
| パッケージ | [ansible.windows.win_package](https://docs.ansible.com/projects/ansible/latest/collections/ansible/windows/win_package_module.html) | package | P2候補：アプリ配布 | MSI限定から開始、任意provider互換なし |
| サービス | [ansible.windows.win_service](https://docs.ansible.com/projects/ansible/latest/collections/ansible/windows/win_service_module.html) | service | P1候補：基本運用 | 既存サービスの状態・起動種別のみ |
| ユーザー | [ansible.windows.win_user](https://docs.ansible.com/projects/ansible/latest/collections/ansible/windows/win_user_module.html) | user | 保留：秘密入力の保護が必要 | ローカル限定、ドメイン管理は対象外 |
| グループ | [ansible.windows.win_group](https://docs.ansible.com/projects/ansible/latest/collections/ansible/windows/win_group_module.html) | group | P2候補：ローカル権限整理 | SID解決を明示、AD対象外 |
| ACL | [ansible.windows.win_acl](https://docs.ansible.com/projects/ansible/latest/collections/ansible/windows/win_acl_module.html) | acl | 保留：継承・拒否ACEの安全設計 | file_contentのACL保持と権限管理を区別 |
| レジストリ | [ansible.windows.win_regedit](https://docs.ansible.com/projects/ansible/latest/collections/ansible/windows/win_regedit_module.html) | registry | P1候補：構成管理の基礎 | 初期案は値単位、再帰キー削除なし |
| Server機能 | [ansible.windows.win_feature](https://docs.ansible.com/projects/ansible/latest/collections/ansible/windows/win_feature_module.html) | feature | P3候補：Server構成 | Client向け機能とは分離 |
| Client追加機能 | [ansible.windows.win_optional_feature](https://docs.ansible.com/projects/ansible/latest/collections/ansible/windows/win_optional_feature_module.html) | optional_feature | P3候補：Client構成 | OS別の機能名・供給元調査が必要 |
| 更新 | [ansible.windows.win_updates](https://docs.ansible.com/projects/ansible/latest/collections/ansible/windows/win_updates_module.html) | updates | 保留：長時間実行・再起動基盤が必要 | 現行timeout内の再試行で代替しない |
| 再起動 | [ansible.windows.win_reboot](https://docs.ansible.com/projects/ansible/latest/collections/ansible/windows/win_reboot_module.html) | reboot | P3候補：更新の前提 | 切断を成功とみなさずboot識別と再接続が必要 |
| 環境変数 | [ansible.windows.win_environment](https://docs.ansible.com/projects/ansible/latest/collections/ansible/windows/win_environment_module.html) | environment | P1候補：アプリ設定 | 永続値のみ、既存プロセスへ反映しない |
| PATH | [ansible.windows.win_path](https://docs.ansible.com/projects/ansible/latest/collections/ansible/windows/win_path_module.html) | path | P2候補：リスト管理 | environmentと同一変数を同時管理しない |
| ダウンロード | [ansible.windows.win_get_url](https://docs.ansible.com/projects/ansible/latest/collections/ansible/windows/win_get_url_module.html) | download | P2候補：配布の前提 | HTTPS・SHA256必須から開始 |
| アーカイブ展開 | [community.windows.win_unzip](https://docs.ansible.com/projects/ansible/latest/collections/community/windows/win_unzip_module.html) | archive | P2候補：配布 | ZIPのみ、パストラバーサル拒否 |
| 定期実行 | [ansible.windows.win_scheduled_task](https://docs.ansible.com/projects/ansible/latest/collections/ansible/windows/win_scheduled_task_module.html) | scheduled_task | 保留：実行主体・秘密の設計が必要 | SYSTEM限定案を先に評価 |
| 情報収集 | [ansible.windows.setup](https://docs.ansible.com/projects/ansible/latest/collections/ansible/windows/setup_module.html) | facts | P1候補：OS判定 | 結果データ用スキーマ追加が先決 |
| ファイル情報 | [ansible.windows.win_stat](https://docs.ansible.com/projects/ansible/latest/collections/ansible/windows/win_stat_module.html) | stat | P1候補：状態確認 | 読取専用、リンク追跡なし |
| 接続確認 | [ansible.windows.win_ping](https://docs.ansible.com/projects/ansible/latest/collections/ansible/windows/win_ping_module.html) | ping | P1候補：実行経路確認 | ICMPではなく固定ラッパーの応答確認 |

Linuxで代表的な [ansible.builtin.command](https://docs.ansible.com/projects/ansible/latest/collections/ansible/builtin/command_module.html)、[copy](https://docs.ansible.com/projects/ansible/latest/collections/ansible/builtin/copy_module.html)、[file](https://docs.ansible.com/projects/ansible/latest/collections/ansible/builtin/file_module.html)、[service](https://docs.ansible.com/projects/ansible/latest/collections/ansible/builtin/service_module.html)、[user](https://docs.ansible.com/projects/ansible/latest/collections/ansible/builtin/user_module.html)、[cron](https://docs.ansible.com/projects/ansible/latest/collections/ansible/builtin/cron_module.html) は上記Windows用途の比較対象であり、そのまま実装しない。apt/dnf・systemd・POSIX所有者/モードはWindows初期対象外。クラウド、ネットワーク機器、AD、IIS固有管理は専門collectionと別要件が必要なため本一覧の設計対象外。

名称の確認では、community.windows側にもwin_lineinfile/win_scheduled_taskが掲載され、ansible.windows側にも同名機能が存在した。本書はansible.windows側を比較先とし、community.windows.win_scheduled_task_statを新規名案に使用しない。索引でDEPRECATEDと明記されたcommunity.windowsのIIS群も新規候補にしない。latestの内容は変動するため実装時に個別資料の版・移行案内を再確認する。

## 現行の共通契約

根拠: `internal/config/config.go`、`internal/module/prepare.go`、`internal/cli/engine.go`、`internal/engine`、`internal/result` と関連テスト。

JSON v1は未知キー・重複キー・型違いを拒否する。対象ホスト選択、文字列変数展開、展開後の全ホスト・全タスクの引数とサイズ検証を最初の接続前に済ませる。変数はdefaults→所属group→host→vars-fileの順で上書きし、所属groupはグループ名の辞書順で適用し、同一キーは後のgroupで上書きする。`${name}`の一回置換で、再帰展開や式評価はしない（詳細は全体設計）。

タスク値をコードへ埋め込まず、固定ラッパーへBOMなしUTF-8 JSON stdinとして一度だけ直列化して送る。EOF後に実行する。全JSONは2MiB以下、固定起動行はOpenSSH/cmdの接頭辞込み8,000 UTF-16単位以下。両上限は同値を許可する。対象はWindows 11 / Server 2022以降amd64、PowerShell 5.1以上、公開鍵SSHとknown_hosts必須、既定cmd構成。become、check mode、handlers、Ansible playbookは現行非対応。

接続10秒、タスクtimeoutは接続後に独立。各ホストは定義順、ホスト間はforksで並列。ホスト失敗後は後続skipped、他ホストは続行する。自動再送しない。未開始はchanged=false、実行中の通信断・期限切れはfailed/null、キャンセルは実行中cancelled/null・未開始cancelled/false。切断後の対象プロセス停止は保証しない。

結果はホスト名・task_index順。changedはtrue（変更あり）、false（変更なし）、null（不明）。CLI終了値の優先順位は130（キャンセル）→3（内部・出力障害）→2（入力不正）→1（タスク失敗等）→0。子プロセスの終了値とは別である。任意コマンド出力は既定非公開、`--include-output`時にstdout/stderr各先頭1MiBをbase64化し、超過をtruncatedで示す。

## command：実行対応済み

| 引数 | 型・必須・既定 | 契約 |
| --- | --- | --- |
| executable | string、必須、既定なし | ドライブ絶対パスの.exe。UNC/device、制御文字、cmd.exeを拒否 |
| argv | string配列、必須、既定なし | 空配列・空要素可、NUL不可。CRT規則で引用 |

`ProcessStartInfo`のUseShellExecute=falseで直接起動。シェル演算子は文字列であり、バッチ/.com/.ps1や独自引数解析exeは対応外。作業ディレクトリ指定・子stdin入力はなく子stdinは閉じる。非冪等で、正常起動して終了0ならstatus=changed / changed=true、非0ならfailed/changed=nullで子exit_codeを返す。拒否・起動失敗・不正応答は失敗とし、詳細なnull判定とプロトコルはcommand詳細を正本とする。

最終子起動行は32,766 UTF-16単位以下（終端NUL除外、非BMPは2単位）。stdout/stderrは各1MiB保持後も読み捨てて回収し、固定ラッパー応答には最大3MiBのtransport枠を使う。不正base64、外側stderr/非0、応答超過等はfailed/null。外側PowerShellと子の終了値を混同しない。

現行のtasks.json例（inventoryにwindowsグループが必要）:

```json
{"version":1,"hosts":"windows","tasks":[{"name":"実行者確認","module":"command","args":{"executable":"C:\\Windows\\System32\\whoami.exe","argv":[]}}]}
```

成功はstatus=changed / changed=true / exit_code=0。失敗例として存在しない `C:\\missing.exe` は起動失敗、`cmd.exe`指定は接続前の入力不正、32,767単位の子起動行は接続前の上限超過。実機確認はWindows 11ローカルとServer Core 2022コンテナSSHまでで、Windows 11のSSH対象・フルServer 2022は未確認。詳細の試験項目・証跡はcommand詳細参照。

## file_content：実行対応済み

main `fb1e92e02754def6b3d3c290c79573199282f843` で再確認。標準CLIのWrappersとDecodeResultに登録済み。[Issue #10](https://github.com/coco-papiyon/wintable/issues/10)はPR #21で統合されCLOSED。採用済み契約・試験範囲の正本は[仕様書](../file-content-module.md)。Linuxも実装済み。[Linux file_content](../linux-file-content.md)にAPI・属性・祖先の信頼境界を記載する。

pathは必須stringのドライブ絶対パス、contentは必須string（空可）、既定値なし。contentのUTF-8は1MiB以下、JSON全体2MiB以下。BOMを付加せず、LF/CRLF・末尾改行を保持しバイト比較する。同値はok/false、未存在は空内容でも新規作成でchanged/true、差分置換はchanged/trueを返す。

親ディレクトリは既存必須。対象・祖先のreparse point、通常ファイル以外、曖昧なWin32パスを拒否する。同一ディレクトリの排他的tempへ書込み・flush後、既存はFile.Replace、新規は上書きしないMoveで確定する。既存DACL保持、所有者管理なし。比較時点のCASは保証せず並行置換は最後の成功内容が残る。確定前の拒否はfailed/false、確定API以降の失敗・清掃失敗・通信断は変更不明null。自動再送なし。PR #21では同期した通常ファイル→symlink切替をコンテナで検証し、同値・異値とも拒否・リンク先不変・tempなしを確認済み。全外部競合・全ACLエラー経路、Windows 11 SSH・フルServerの個別確認は保証範囲外。

現行Windowsでの実行例:

```json
{"version":1,"hosts":"windows","tasks":[{"name":"設定配置","module":"file_content","args":{"path":"C:\\app\\config.txt","content":"日本語\r\n"}}]}
```

## 将来候補の共通設計案

以降はすべて提案であり現行JSON v1で受理されるモジュールではない。固定コード＋stdinデータの境界、接続前全件検証、出力非公開と上限、失敗後のホスト停止を継承する。各引数は表記した必須以外は任意、既定は明記の通り。state列挙以外・未知キーは拒否する。対象OSは現行と同じで、管理操作にはSSHユーザー自身の必要権限を要求し、昇格は行わない。

以下のstatus/changed表記は現行結果契約を継承する。状態を読んで差分がなければok/false、変更して再読取が期待値と一致したらchanged/true。副作用前の拒否はfailed/false、書込み後の確証喪失はfailed/null。複数資源のトランザクションや自動ロールバックは保証しない。読み取り専用は常にchanged=false。facts/statの構造化データとreboot_requiredは現行結果契約にないため、上限・秘密除外を含む結果スキーマ拡張を別途決定する。

### P1：基本構成

| 名前・用途 | 主要引数案（型・必須・既定）と状態 | 冪等性・実行方式 | 権限・失敗条件・検証シナリオ |
| --- | --- | --- | --- |
| file：配置先管理 | path:string必須、state:string=directory（directory/absent） | 固定.NET APIで存在・種別比較。absentは空ディレクトリか通常ファイルのみ | 親への書込権限。親なし・非空削除・リンクは失敗。新規/再実行/削除/リンク/拒否を検証 |
| service：既存サービス | name:string必須、state:string必須（started/stopped）、start_mode:string任意・省略時保持（auto/manual/disabled） | ServiceControllerとCIMで差分更新・状態待機 | サービス制御権限。未存在・disabledとstartedの矛盾・待機期限は失敗。停止→起動/同値/権限不足を検証。restartedは非冪等のため初期案に含めない |
| registry：値管理 | path:string必須、name:string必須（空は既定値）、state:string=present、type:stringとdata:stringはpresent時必須（string/dword）、view:string=64（32/64） | RegistryKeyで型と値を比較、absentは値のみ削除 | 対象キー書込権限。HKLM/HKCU限定、dword範囲外・アクセス拒否は失敗。型違い/同値/32・64分離/削除を検証。親キー作成は初期案で拒否 |
| environment：永続変数 | name:string必須、scope:string必須（user/machine）、state:string=present、value:stringはpresent時必須 | 永続環境の値を比較して設定。absentは削除 | machineは管理権限。PATH名は拒否し専用候補へ。空と未存在/同値/拒否を検証。既存プロセスの環境は更新しない |
| facts：OS確認 | subset:string配列=[os,architecture]、状態なし | 固定APIで許可項目だけ収集、changed=false | 一般読取権限。未対応subset・取得不能は失敗。OS別/非管理者/出力上限を検証。結果スキーマ拡張が依存 |
| stat：ファイル確認 | path:string必須、checksum:bool=false、状態なし | 属性・サイズと任意SHA256を収集、未存在も成功 | 読取権限、リンクは追跡せず拒否。未存在/空/大量/アクセス拒否を検証。ハッシュ対象上限と結果スキーマを先に決定 |
| ping：実行確認 | 引数なし、状態なし | 固定応答を検査、changed=false | 現行SSH・PowerShell要件。応答不正・期限超過は失敗。正常/ホスト鍵不一致/応答破損を検証 |

### P2：運用拡張

| 名前・用途 | 主要引数案（型・必須・既定）と状態 | 冪等性・実行方式 | 権限・失敗条件・検証シナリオ |
| --- | --- | --- | --- |
| copy：バイナリ配置 | src:string/dest:string必須、stateはpresent固定 | 制御側ファイルを読取、ハッシュ比較と安全置換 | src読取・dest書込。file_contentの置換基盤に依存。分割転送方式と総容量は未確定（現行2MiB枠を無断拡大しない）。同値/バイナリ/途中切断/容量超過を検証 |
| line：局所編集 | path:string/line:string必須、state:string=present（present/absent） | UTF-8限定、完全一致行の有無で判定し安全置換 | 書込権限、既存ファイルのみ。複数一致・混在改行は初期案で拒否。追加/再実行/削除/不正UTF-8を検証。正規表現と挿入位置は保留 |
| package：MSI配布 | path:string/product_id:string必須、state:string=present（present/absent） | MSI APIでproduct_id存在比較、固定msiexec引数 | 管理権限、pathはローカルMSI。再起動要求は記録し自動再起動なし。別版検出・競合インストール・異常終了は失敗。新規/同値/削除/3010を検証。再起動要求の結果契約が依存 |
| group：ローカルグループ | name:string必須、state:string=present（present/absent）、members:string配列=[] | LocalAccountsで存在・指定メンバー包含を比較し追加 | 管理権限。SID不明・組込みグループ削除は失敗。再実行/未解決SID/削除を検証。未指定メンバー削除は行わない |
| path：PATH要素 | elements:string配列必須、scope:string必須（user/machine）、state:string=present（present/absent） | 大文字小文字・末尾区切り正規化後に差分追加/除去、他要素の順序保持 | machineは管理権限。空要素・区切り文字入りは拒否。同値/重複/順序/他要素保持を検証。%変数%の比較規則は実装前に確定 |
| download：配布取得 | url:string/dest:string/sha256:string必須、stateはpresent固定 | HTTPS取得をtempへ保存しハッシュ一致後に置換、既存ハッシュ同値ならfalse | ネットワークと書込権限。TLS不正・リダイレクト・不一致・容量超過は失敗。認証URLは初期対象外。正常/同値/切断/不一致を検証。上限・proxy・証明書方針の調査が依存 |
| archive：ZIP展開 | src:string/dest:string必須、stateはpresent固定 | ZIP全エントリの期待ハッシュと宛先を比較、隔離tempで展開 | 読取・書込権限。絶対パス/..脱出/リンク/重複/zip bomb拒否。再実行/破損/脱出/部分確定失敗を検証。サイズ・件数上限とディレクトリ確定方式は実装前に決定。余剰ファイルは削除しない |

### P3・保留の判断と依存

feature/optional_feature案は `names:string配列`必須、`state:string=present（present/absent）`、ソース指定なしから開始する。固定PowerShellでServerはGet/Install/Uninstall-WindowsFeature、ClientはGet/Enable/Disable-WindowsOptionalFeatureを使い事前・事後状態を比較する。管理権限必須で、OS不一致・機能名不明・供給元不足は失敗。再起動を自動実行せず要求を記録する。正常/同値/削除/再起動要求を各OSで検証する。機能間依存と再起動要求の結果仕様の確定後に着手する。

reboot案は `timeout_seconds:integer=600`、`message:string="wintable reboot"`、stateなし。管理権限と再起動権限が必要。再起動要求後にSSH再接続しboot識別変更と準備完了を確認してtrueとする。要求前拒否はfalse、要求後の再接続期限超過はnull。常に実行する非冪等操作として明示する。再起動禁止環境/起動識別不変/鍵変更/接続復帰を検証する。現行engineの一接続・失敗停止モデルを拡張する必要があるため最後に設計を確定する。

user/acl/template/powershell/shell/scheduled_task/updatesは採用未決定。順に秘密の非露出と更新比較、ACE継承と拒否優先、テンプレート言語と制御側読取、任意コードの結果検証、引用・シェル境界、実行主体と資格情報、長時間ジョブと再開点が未確定である。これらを調査して個別Issueで引数と受入試験を決定するまで、現行対応やP1/P2の確約に含めない。

推奨順序は、file_contentの取り込みと安全置換の検証→P1のfile/service/registry/environment→読取結果基盤とfacts/stat/ping→転送・ハッシュ基盤とP2→再起動要求の結果契約→機能管理と再接続基盤。各段階でWindows 11 SSHとフルServer 2022の試験範囲を明示し、コンテナ試験だけで全環境合格としない。

Linux command backendは [Linux command仕様](../linux-command.md) を参照。設計根拠は [ADR-0001](../adr/0001-linux-target-support.md)。Linux file_contentも実装済み。対応公開・CIはIssue #29/#31の[公開ゲート](../ci.md)に従う。
