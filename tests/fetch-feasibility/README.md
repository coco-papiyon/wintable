# Issue #39: Windows保存確定方式の成立確認（未通過）

このディレクトリは **却下した保存方式の再現用コード**。fetch製品実装でも受入試験でもない。schema・backend・engine・transportには登録していない。公開機能はcommand / file_contentのままである。

## 基準

[Issue #39の実装計画](https://github.com/coco-papiyon/wintable/issues/39) の「安全性と起動サイズの成立確認」を実施した。基準mainは `4e26bf4`。計画は、既存先の識別子確認だけで済ませず、競合窓を閉じること、成立しない場合は根拠と代替案をIssueへ戻し、安全性を下げて実装済みとしないことを要求している。

## 確認結果

Windows制御端末上の実ファイルAPIで次を再現した。

| プローブ | 実測 | 判定 |
| --- | --- | --- |
| 削除共有を拒否した旧ファイルを保持 | 新規保存と同値比較は成功。内容更新の `NtSetInformationFile(FileRenameInformationEx=65, REPLACE_IF_EXISTS\|POSIX_SEMANTICS)` は `STATUS_SHARING_VIOLATION (0xC0000043)`。既存内容は保全 | この方式では内容更新を実装できない |
| 旧ファイルの削除共有を許可 | 旧ハンドル保持後に別プロセス相当のrename→別ファイル作成を挿入すると、旧ハンドルの内容比較は成功したまま、別ファイルを置換してしまう | この方式では競合時の対象同一性を保証できない |
| 固定Windows sender | 起動行7,965 UTF-16単位（上限8,000）。日本語・空白のあるパスから1,400,000バイトを実cmd.exe / PowerShell 5.1で転送し、サイズ・全byte・SHA-256が一致 | 小さな実プロセス成立確認のみ成功 |

競合試験は確率的な連打ではなく、旧ハンドル保持後から確定前までの間に差し替えを必ず挿入する。上記の2種類の失敗を再現できるとプローブのGoテストはPASSになる。**PASSはfetch要件を満たす意味ではない**。

senderは専用バイナリstdoutの成立確認のみ。`WTF1`・little-endian int64サイズ・本体・SHA-256を出し、全体を3回読み比較する。通常のcapture、製品SSH、フレーム受信機、ローカル保存とは未統合。異常時の終了値伝播なども未実装であり、そのまま製品へ移してはならない。祖先のwrite共有を拒否するため、既存writeハンドルがあるユーザーディレクトリでは開けないことも確認した。

保存プロトタイプは試験専用 `_windows_test.go` 内に隔離した。祖先をパスで開く箇所、ロック・一時ファイルACL、ハードリンク競合、異常時のchanged、清掃なども製品要件を満たしたとは判断していない。src/destに任意のユーザーデータを使わないこと。

## 再実行

リポジトリルートからWindows上で実行する。`.cache` 配下に独立した一時ディレクトリを作成し、終了時に削除する。

```powershell
$env:WINTABLE_FETCH_FEASIBILITY = '1'
go test ./tests/fetch-feasibility ./internal/module -run 'Test(ProtectedReplacementIsRejected|DeleteSharedReplacementLosesIdentityProtection|FetchWindowsFeasibility)$' -count=1 -v
Remove-Item Env:WINTABLE_FETCH_FEASIBILITY
```

既定の製品回帰ではこれらのホスト依存プローブをskipする。CI/配布ゲートや既存受入の期待値は変更していない。

## 次に必要な設計確認

1. ハンドル相対操作で祖先・最終先を辿り、対象同一性の確認と置換を保護する方式を再設計する。例えばoplock等で第三者の操作を検出・制御する案は、自己の確定操作との干渉も含めた実機成立確認が必要であり、現時点で解決策として採用していない。
2. 原子置換の直前に旧ハンドルを閉じる方式は、今回再現した競合窓を残すため採用しない。同じwintable間の名前付きロックだけでも任意の他プロセスは保護できない。
3. TxFを新たな前提にする案も未採用。Microsoftは将来の提供可能性を理由に他の方式の検討を推奨しており、新しい依存条件と対応範囲の設計が必要。
4. 保存ゲートが成立した後にのみ、入力/schema、全件衝突検査、専用SSHストリーム、Linux sender、engine/CLI結果、両OS実SSH、1GiB境界・障害注入、文書・公開ゲートへ進む。

参考: [CreateFileの共有条件](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-createfilew)、[FILE_RENAME_INFORMATION](https://learn.microsoft.com/en-us/windows-hardware/drivers/ddi/ntifs/ns-ntifs-_file_rename_information)、[TxFの代替方式](https://learn.microsoft.com/en-us/windows/win32/fileio/deprecation-of-txf)。共有の解釈は文書に加え、上記の実測に基づく。この結果だけから別のWindows API方式まで不可能とは結論しない。

## 未実施

REQ-001〜REQ-009は製品機能として未実装・未確認。実SSH、Linux sender、timeout/cancel/切断、認証・known_hosts回帰、1GiB境界、ACL・リンク競合、配布・両OS証跡は未実施。実装回数は進めず、実装完了ラベル・PRも作成しない。
