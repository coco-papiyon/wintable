# Linux file_content: API選定と保証境界

Issue #30 / ADR-0001 決定5の詳細。標準CPython 3.13の [os API](https://docs.python.org/3.13/library/os.html) のみを使用する。ctypes、外部コマンド、sudo、追加バイナリは使わない。

## 入力と公開契約

`path`/`content`のみ。pathはPOSIX絶対パス、空セグメント・`.`・`..`・末尾/・制御文字・不正Unicodeを拒否する。contentはBOMを足さないUTF-8、改行/NUL保持、1MiBまで。JSONは2MiBまで、重複・未知キー・型不正・末尾データを拒否。Goとreceiverの両方で検査する。

## 信頼境界と拒否条件

rootとSSHユーザーを信頼する。全祖先をルートから `open(O_DIRECTORY|O_NOFOLLOW|O_CLOEXEC, dir_fd=...)` で辿り、確定・清掃まで保持する。各祖先はrootまたはSSHユーザー所有、group/other書込み不可、xattrなしを必須とする。stickyな/tmpも拒否する。ACLはsystem.posix_acl_* xattrとして拒否する。列挙失敗は拒否。対象のsymlink・FIFO/socket/device/directory・複数hardlinkも拒否する。

この制約は非信頼ユーザーの名前空間変更権限を排除するための追加条件である。root・同じUIDの敵対的操作、mount namespaceの変更、検査後の権限変更は禁止前提。これらに対する完全防御はPython標準APIでは保証できない。dirfdはrenameを禁止しない。確定直前に祖先の名前と保持fdのdev/inoを照合し、検出した移動を拒否するが、最終照合と確定の間の移動は防げない。その場合、移動された元のディレクトリへ更新する可能性がある。これはWindowsの祖先共有拒否と異なる。信頼前提に合わない環境では使用しない。

## 更新順序とAPI

1. 対象を `O_RDONLY|O_NOFOLLOW|O_NONBLOCK` で開き、同じfdのfstat/listxattrで通常ファイル・属性を確認。FIFO待機しない。ENOENTだけ新規。最大content長+1バイトを比較し、同値ならtemp作成・mtime変更なし。
2. 親fsyncの事前確認。暗号乱数128bitの同一ディレクトリtempを `O_CREAT|O_EXCL|O_NOFOLLOW` /0600で作成。衝突は拒否し再送しない。全バイト書込み、fchown→fchmod、属性読戻し・xattr再検査、ファイルfsync。
3. 既存UID/GID/mode（特殊bitも含む）を保持できなければ拒否。新規は実効UID/GID・0600を設定し読戻しする。setgid/umaskに依存しない。default ACL付き親はtemp以前に拒否する。
4. 祖先・対象・tempを再検査。既存はfd相対 `os.replace` で原子的に名前を置換。新規は `os.link(...,follow_symlinks=False)` の排他的宛先作成でno-replace。どちらも対象symlinkのリンク先に書き込まない。新規link後はtemp名unlinkが別段階で必要。
5. 親fsync、残る自身のtemp名をdev/ino照合後unlink、親fsync、fd close。temp名を差し替えられた場合は他者の名前を削除せずnull。確定後に宛先を補償削除しない。

同じUIDによるtemp差替えとunlinkの間の競合まで排他的に防止するAPIはない。dev/ino検査は検出補助であり、上の信頼境界を代替しない。比較時点CASなし。既存の通常ファイル並行置換では最後の完全な内容が残り得る。新規宛先出現はlinkが失敗し上書きしない。

## 結果と障害

changed=falseはok、trueはchanged。確定APIを呼ぶ直前に状態を不明へ移し、以後の例外はexecution_failed / failed/null。事前拒否はwrite_failed / failed/false。清掃・close失敗もnull。通信断・timeout・cancelは共通transport/engine契約でnull、自動再送しない。ディレクトリfsync不能・属性列挙不能では成功扱いしない。電源断耐性はfilesystem/デバイス実装に依存し、全filesystemの保証ではない。

## 検証の区別

`tests/linux-ssh/file-content.ps1` は固定Debian fixture、実SSH、通常runnerを使用する。`product-bytes` はWindows製品exe経路と独立hash/mode/UID probe。`API-cases` は製品receiver関数を実SSH先で読み込み、テストだけのAPI差替えで競合/障害の同期点を作る。製品には注入入口を含めない。モックの成功を全実機競合の証明とはしない。

属性の追加受入試験は、rootで用意したroot所有・0644の既存ファイルを通常runnerから更新し、fchown不能による確定前拒否、元内容・UID/GID/mode/mtime/inode不変、自身のtemp清掃を照合する。setgid有無×default ACL有無の4組合せを実SSHで試し、ACL付き親の拒否、ACLなし親の新規実効UID/GID・0600を確認する（setgid親のGIDはrunnerの実効GIDと異なるroot、umaskは077）。

`commit-disconnect` は同じreceiverソースにテスト限定の同期点を置く。確定前はtempのファイルfsync直後（started=false）、確定後は実際のreplace直後で待機し、独立SSH probeで元内容/完全な新内容を照合してから実SSH接続を閉じる。結果null、receiver完走後の完全な内容・確定呼出し1回・残存tempなしを検証する。テストではSIGHUPを無視してreceiverを完走させるため、強制終了時の即時清掃を証明する試験ではない。製品の固定起動行に同期点やsignal変更は入れない。

未確認: 全filesystem、実機電源断、悪意のroot/同一UIDへの完全防御、強制kill時の即時temp回収、全device種別、混在単一run実機E2E。強制切断後の残存はfixtureコンテナ破棄で回収する。対応公開は既存CI集約条件と別途保存する実証記録に従う。

実行例: examples/linux-file-content.json。実SSH検証は tests/linux-ssh/run.ps1 に -ProductTestScript tests/linux-ssh/file-content.ps1 を指定する。既存Linux command試験も同時に実行し、skipを失敗として扱う。
