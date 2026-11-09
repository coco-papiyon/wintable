# fetch: リモートファイルをWindows制御端末へ取得

Windows / Debian 13対象ホストから通常ファイルを無変換で取得する。既存のSSH公開鍵認証・known_hosts検証を使い、scp/sftpや追加のサーバー機能は不要。Windows対象はPowerShell 5.1/.NET、Linux対象は `/usr/bin/python3` のCPython 3.13とOpenSSHが必要。制御端末はWindows amd64、保存volumeはNTFSのみ。

```json
{
  "version": 1,
  "hosts": "all",
  "tasks": [
    {
      "name": "ログ取得",
      "module": "fetch",
      "args": {"src": "${fetch_src}", "dest": "collected"}
    }
  ]
}
```

inventoryの各hostのvarsへ、Windowsなら `"fetch_src": "C:\\logs\\日本語 空白.bin"`、Linuxなら `"fetch_src": "/var/log/example.log"` を設定する。[実行例](../examples/fetch.json)と既存の[混在inventory例](../examples/mixed-inventory.json)を参照。

```powershell
.\wintable.exe run --inventory inventory.json --tasks examples/fetch.json --format json
```

## 引数と保存先

必須文字列 `src` と `dest` の2個だけを受け取る。変数展開後、選択した全host・全taskを接続前に検証する。事前検証ではディレクトリを作成しない。

- srcは対象OSの通常ファイルの絶対パス。Windowsはドライブ絶対パス、Linuxは `/` 起点。日本語・空白を許可。相対パス、ドットセグメント、空要素、制御文字、ワイルドカードを拒否する。WindowsはUNC/device、ADS、予約名（COM/LPTの上付き1/2/3を含む）、末尾空白/ドットも拒否する。
- destは制御端末の保存ルート。相対destはCLI起動時の作業ディレクトリ基準で絶対化し、計画へ固定する。Windows形式の `\` を使う。UNC/device、ドライブ相対、ドットセグメントなどを拒否する。欠けたディレクトリは実行時に作成する。
- 初版ではWindowsのsrcと最終保存パスを各240 UTF-8バイト以下、Linuxのsrcを4096 UTF-8バイト以下に制限する。保存名の分も必要なのでdestは短いルートを選ぶ。

最終ファイル名は次の規則。生のホスト名やリモートbasenameをローカルのパス要素にしない。basenameに拡張子や機密情報が含まれても名前に露出しない。

```text
dest\h-<SHA256(UTF8(host識別名))>\s-<SHA256(UTF8(OS名 + NUL + 正規化src))>.bin
```

SHA-256は小文字16進全64桁。hostはinventoryの識別名をそのまま使う。OS名は `windows` / `linux`、区切りは1バイト `00`。Windows srcはGoのUnicode小文字化、Linux srcはそのまま。Unicode正規化はしない。再実行で同じ保存先になる。異なるhost識別名は別ディレクトリになる。全件の最終パスをWindowsの大小文字を無視して比較し、同一パスと祖先/子孫の衝突を接続前に拒否する。同じrun内で同じ取得先を二度使うこともできない。

例としてPythonで保存先を計算する場合、hostとsrcがASCIIなら以下と一致する（非ASCIIのWindows srcはGoのUnicode小文字化規則を使用する）。

```python
import hashlib, pathlib
host, target, src = 'server1', 'linux', '/var/log/example.log'
h = hashlib.sha256(host.encode('utf-8')).hexdigest()
s = hashlib.sha256((target + '\0' + src).encode('utf-8')).hexdigest()
print(pathlib.Path('collected') / ('h-' + h) / ('s-' + s + '.bin'))
```

## 転送・整合性・結果

最大ファイルサイズは **1GiB（1,073,741,824バイト、境界を含む）**。空ファイル、テキスト、任意のバイナリを扱い、文字コード・BOM・改行を変換しない。約64KiBの固定バッファを使い、ファイル全体をメモリやbase64/JSONへ展開しない。メモリ使用量は主にforksに比例する。

専用SSH execのstdoutをバイナリstreamとして受信する。通常の結果captureの1MiB制限を通さない。stderrは記録せずdrainする。`--include-output` を指定してもfetchの内容・リモート例外を結果JSONや診断へ出さない。destはリモートへ送らない。

フレームは `WTF1`（4バイト）、宣言サイズ（little-endian uint64）、正確にそのサイズの本体、SHA-256（32バイト）、EOF。受信側はサイズ上限・不足/過剰・ハッシュ・EOF・リモート正常終了のすべてを確認する。senderは同じ開いたファイルの転送前後の全体ハッシュと転送時ハッシュ、サイズ/更新情報/識別情報を照合する。Windowsでは書込み共有を拒否する。Linuxでは変更を検知したら失敗させるが、非協調writerに対する厳密な時点スナップショットを保証しない。必要なら利用者が書込みを停止する。

| 結果 | status | changed | exit_code |
| --- | --- | --- | --- |
| 新規保存・内容更新 | changed | true | null |
| 同値確認 | ok | false | null |
| 確定前の転送・比較・flush・置換失敗 | failed | false | null |
| 確定後の清掃・期限等の失敗 | failed / cancelled | true | null |
| 接続失敗・未実行 | 既存のunreachable / skipped / cancelled | false | null |

親ディレクトリの作成はchangedに数えない。task期限は転送・比較・flush・確定までを含み、確定直前にもctxを検査する。OSの同期ファイルAPI自体を強制中断する保証はない。中断が確定後に観測された場合も、既知のchanged=trueを保持する。ホスト内の順序・失敗後の後続停止、他ホストの継続、forks、終了コードは既存仕様と同じ。自動再送はしない。

## ローカル確定と競合の扱い

同一ディレクトリへ乱数名で一時ファイルを排他作成し、読戻しSHA-256とflushを確認してからハンドル相対の原子的renameで確定する。既存先を直接切り詰めない。同値なら最終ファイルの内容・時刻を変更せず、一時ファイルを削除する。新規・更新後のファイルACLは制御端末の実行ユーザーとSYSTEMだけへfull accessを与える。更新前のACL、リモート所有者・ACL・時刻は複製しない。作成ディレクトリは親ACLを継承する。

**第三者が検査後に保存先を差し替えた場合、その保存先の上書きを許容する。** ユーザー承認に基づき、検査した旧ファイルの識別子との一致を確定条件にはしない。他プロセスとの同時編集では最後の確定が優先される。changed=falseも同値確認時点の結果であり、その後の第三者の変更を保証しない。通常の同一内容再実行ではchanged=falseになる。

祖先はドライブrootからハンドル相対に開き、symlink/reparse pointを辿らず検査する。保持した祖先の削除/renameを拒否する。既存の最終ファイルもリンク・複数hard linkを拒否する。最終検査後にリンクへ差し替えられた場合のrenameは、リンク先の内容ではなくその名前のentryを置換する。ディレクトリは上書きしない。ハンドル操作を保証できないfilesystemでは失敗する。

同じwintable間では保存先ごとの `.s-<hash>.bin.lock` の共有なしOSハンドルで同時取得を拒否し、再試行しない。空lockファイルは終了後も残す。存在はロック所有を意味せず、次回は同じファイルを開いて取得する。実行中にlockファイルを手動で削除しない。

通常終了・失敗・cancelでは不要な一時ファイルをハンドルで削除し、lock/祖先ハンドルを解放する。清掃失敗は成功扱いしない。強制終了や電源断後の `.wintable-fetch-*.tmp` は残る場合がある。自動で他runのものと判断して削除せず、実行プロセスがすべて停止したことを確認して、保存先ディレクトリ内の不要な一時ファイルだけを手動削除する。電源断後の永続性やディレクトリ作成のrollbackは保証しない。

## 失敗と対象外

不存在、権限不足、ディレクトリ、symlink/reparse、複数hard link、サイズ超過、転送中の変更、破損、不完全なEOF、ローカル書込/比較/flush/確定/清掃失敗はサニタイズしたtask_failed。通信エラーはconnection_lost等、期限はdeadline_exceeded、親キャンセルはcancelledとして扱う。詳細なリモート例外やsrc/destは下位エラーから公開しない。

URL download、ディレクトリ再帰、ワイルドカード、転送再開、所有者/ACL/時刻の完全複製は対象外。両OSの個別実SSHと混在単一runの実機E2Eは別の検証範囲であり、公開可否は同一SHAの受入証跡とCIゲートで判断する。
