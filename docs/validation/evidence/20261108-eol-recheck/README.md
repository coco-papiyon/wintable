# Git取得後の証跡再集約と改行差の再検証

Issue #11の[要修正3回目](https://github.com/coco-papiyon/wintable/issues/11#issuecomment-6716798060)に対する実装4回目。保存された実測証跡は変更せず、Gitから取得したソースと証跡のLF/CRLF差で集約が失敗する問題を修正した。

## 比較と結果

| 項目 | 修正前 | 修正後 |
| --- | --- | --- |
| Git archive取得元SHA | `64e7bd38cfc739f85cae4c622cd4214764b0e6f5` | `ee42c48335a6584d3dea7e92727ca49df09d9a6f` |
| 入力証跡ハッシュ検査 | 46件一致 | 46件一致 |
| 集約器回帰試験 | 23件PASS・終了0 | 28件PASS・終了0 |
| 保存証跡の再集約 | 終了1・unconfirmed・前提条件false | 終了0・passed・前提条件true |
| シナリオ | 18件passedだが全体受け入れ不可 | 18件passed・全体受け入れ成功 |

`baseline/` と `fixed/` に実際のresult.jsonとログを保存した。`fixed/manifest.json`、`scenarios.csv`は展開先で再集約した出力。実行日時はresult.jsonにUTCで記録。修正後SHA以降の変更はこの再検証証跡と文書のみ。

## 修正と検査範囲

保存証跡のcoverage.jsonの生バイトSHA-256は `DDE7163F80F5650FEA7A36F24642FCD6086BA8CF0DD78BAB508E15D3F1C4300E`、Gitから取得したソース側は `C4EC856C2E535DA734C1BFFD8D1B4619F26878D0D7B5F2F4B82569399E5F980E`。両方の値は修正前後で変わらない。

集約器は、まず保存証跡の生バイトハッシュとmanifestの一致を必須とする。ソースと証跡の内容比較では厳密UTF-8で読み、CRLFだけをLFへ置換してOrdinal比較する。BOM、空白、大文字小文字や内容は無視しない。証跡や期待ハッシュの書換えで通過させていない。

追加の5パターンは、LF証跡成功、CRLF証跡成功、内容変更拒否、空白変更拒否、生バイトハッシュ未更新の改行変更拒否。内容・空白変更のケースではハッシュを更新してもソースとの不一致を拒否する。既存の欠落・失敗・SKIP・SHA不一致・清掃失敗など23パターンも保持する。

今回Docker実機試験、go test/vet/buildは再実行していない。製品・Goコード・実機試験入力を変更せず、[既存実測証跡](../20261108-complete/README.md)に対する集約処理と回帰試験を変更した。旧実測SHAとバイナリハッシュはそのまま保持する。

## 再現コマンド

リポジトリで、修正をcommitした後に実行する。

```powershell
pwsh -NoProfile -File tests/validation/test-git-evidence.ps1 -Revision 64e7bd38cfc739f85cae4c622cd4214764b0e6f5 -OutputDirectory .cache/eol-baseline
# 旧版は意図どおり終了1。result.jsonで集約終了1を確認する。
pwsh -NoProfile -File tests/validation/test-git-evidence.ps1 -Revision ee42c48335a6584d3dea7e92727ca49df09d9a6f -OutputDirectory .cache/eol-fixed
# 修正版は終了0。
```

同じ出力ディレクトリは再利用できない。再実行時は新しい名前を指定する。検証スクリプトは指定SHAのgit archiveを展開し、展開先のテストと集約器を別pwshプロセスで実行する。未コミットファイルを検証ソースに使わず、既存証跡も上書きしない。

`sha256.csv` はこの再検証証跡のハッシュ索引。元の20261108-complete以下のファイルは変更していない。
