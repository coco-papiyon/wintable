# file_contentモジュール

`path`と`content`だけを受け付けます。例:

```json
{"name":"config","module":"file_content","args":{"path":"C:\\app\\config.txt","content":"日本語\r\n"}}
```

contentはBOMを付加しない厳密UTF-8へ符号化します。LF/CRLF、末尾改行、引用符、非BMP文字を変換しません。既存ファイルとの比較は復号せずバイト列で行います。同値は`ok/false`で、更新時刻を変えず一時ファイルも作りません。未存在の空ファイルは新規作成です。

固定受信コードは`receiver.cs`の厳密JSON解析と上限付きstdin読取りをcommandと共有します。contentは1MiB、JSONは2MiBまでで、受信側も独立に検査します。未知・重複キー、型不正、末尾データ、BOM、不正UTF-8/サロゲートは副作用の前に拒否します。Go側も不正UTF-8を拒否し、全ホスト・タスクの展開後検証を済ませてから接続します。

## 置換と競合

対象・親・祖先のディレクトリ/reparse pointを検査します。祖先はルートから順に`CreateFile(OPEN_REPARSE_POINT | BACKUP_SEMANTICS)`で開き、ハンドルから属性を調べます。DELETE共有を許可しないハンドルを確定まで保持するので、検査済み祖先の名前を競合処理で付け替える操作は共有違反になります。通常ファイルの対象以外は拒否します。ドライブ絶対パスに限定し、曖昧な末尾の空白/ピリオド、空要素、`.`/`..`、Win32予約デバイス名も拒否します。

既存対象はOPEN_REPARSE_POINT付きで開き、同じハンドルの属性でディレクトリ/reparse pointを拒否してからバイト比較します。書込み共有は許可せず、ファイル不在（ERROR_FILE_NOT_FOUND）の場合だけ新規として扱います。確定前の再検査も同じ方法で行います。同一ディレクトリにGUID名の一時ファイルをCreateNewで排他的に作り、全バイトを書き、Flush(true)・close後に確定します。清掃対象は作成に成功した自分の一時ファイルだけです。

既存は`File.Replace(temp,path,null,false)`、新規は上書きしない`File.Move`を使います。ACLマージ失敗を無視する指定や直接truncateする代替処理はありません。[ReplaceFileの保持対象](https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-replacefilew)にはDACLが含まれます。所有者管理はモジュールの範囲外です。Server Coreの継承DACLでは、ReplaceFile後に権限ACE・継承保護を変えず、OSがSE_DACL_AUTO_INHERITED（SDDLのAI）を付加する場合があります。試験はこの由来フラグだけを正規化して比較し、ACE・順序・保護フラグはそのまま比較します。

同時更新は比較時点に対するCASではありません。通常ファイル同士の並行置換は最後に成功した完全な内容が残り、新規宛先の出現はMove失敗になります。共有違反・権限拒否等は失敗とし、自動再送しません。成功応答は確定後だけ返します。確定API呼出し以降の失敗・通信断・期限切れは`changed=null`、それ以前の拒否は`changed=false`です。finallyで未確定の一時ファイルを可能な限り清掃し、清掃失敗は変更不明として扱います。既存対象を削除する補償処理は行いません。

## 起動と検証

タスク値はUTF-8 JSON stdinだけへ入ります。固定C#は配布時のDeflate/base64からPowerShell 5.1のAdd-Typeで読み込みます。実行時のスクリプトファイルや外部コンパイラは不要です。

起動長の式は`UTF16Len(WrapperLaunchLine(FileContentWrapper()))`で、OpenSSHのcmd接頭辞と引用符を含みます。file_contentは**7,997**、commandは**7,185**単位です。8,000超過はビルド時とCLI接続前に拒否します。固定コード変更時は必ず再測定します。

```powershell
go test ./internal/module -run '^TestFileContent|TestDistributionWrapper' -v -count=1
go test ./...
go vet ./...
.\build.ps1
.\tests\windows-ssh\run.ps1
```

ローカルWindows build 26300、Windows PowerShell 5.1.26100.9818で、BOM/改行/日本語/非BMPのバイト一致、1MiB、同値時の更新時刻、不正受信、親なし、ディレクトリ、junction対象/祖先、ロック時の保全、明示DACL保持、4並列更新後の完全バイト列と一時ファイル清掃を検証します。SSH fixtureはServer Core 2022上で実ラッパーを起動します。

Windows 11のSSH対象、フルServer 2022、任意の外部プロセスによる宛先の消失/リンクへの切替を同期して再現する競合試験、すべてのACL拒否・OSエラー経路は未確認です。ローカルcmd/Server Coreの合格を両OSのV-08/09/10/14/15/16/18すべての合格にはしません。

## 2026-11-08の実行記録

PR #21のF-001修正では、オープン直前に通常ファイルを実ファイルsymlinkへ交換する同期テスト`TestFileContentLinkSwapWindows`を追加しました。Windowsコンテナではリンク作成失敗をskipせず失敗として扱い、リンク先が期待内容と同値・異値の両方で`write_failed`、リンク先不変、一時ファイルなしを確認しました。試験用コードの注入はテスト内だけで行い、製品の属性検査・比較処理を使います。既定のコンテナ試験にも含めます。修正後の実SSH/cmd起動長は7,997 UTF-16単位で構築式と一致しました。今回の証跡は`.cache/pr21-fix-evidence/`です。以下は修正前の実行記録です。

`go test ./...`、`go vet ./...`、`build.ps1`（固定ラッパー検査とWindows amd64ビルド）、`git diff --check`を実行しました。全Goテスト・vet・ビルドは成功しました。コンテナ回帰を同時に実行した回ではcommandの受信境界で期限超過が出たため、最終のSSH回帰は単独で実行しています。

SSHの証跡は`.cache/windows-ssh/1e0ecd98f80c/`です。実cmd起動行は`wrapper-launch-file.json`へ保存し、7,985単位で構築式と一致しました。Server Core 2022（10.0.20348.5773）、PowerShell 5.1.20348.5773、OpenSSH 9.5.6.4、Docker 29.9.2、Hyper-V分離、固定ベースdigestはmanifest.jsonに記録します。ローカル証跡と試験用資格情報はGitへ含めません。埋込C#ソースは`.gitattributes`で改行変換を無効にし、圧縮結果の再現性を保ちます。

最終SSH回帰は終了0でした。command/file_contentの製品モジュール4試験が成功（Goパッケージ128.686秒）、続くtransport全7サブテストも成功（34.59秒）しました。manifestで専用コンテナ・専用イメージの削除を確認し、試験用秘密鍵も削除されています。

## Linux

Linuxは別のPython receiverを使用します。[Linuxの詳細設計と制約](linux-file-content.md)を参照してください。上記のWindowsハンドル・DACL方式はLinuxには適用しません。
