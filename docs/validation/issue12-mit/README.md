# MIT配布候補・最終ZIPの検証記録

この旧候補は監査に GO-2026-5932 のfindingが残っているため配布不可です。以下の実機試験成功は当時の実測記録として保持しますが、監査合格・公開可能を示しません。修正版依存とfinding拒否ゲートを用いた新しいZIP・証跡で置き換える必要があります。

配布commit: `2a19a2becbd738b06929eb19825e395d936a8f17`。
ZIP SHA-256: `66dd56eb63c85fde014a962a7d1bd1af5e2a7a70cb2decfde28e38cc044ae3c0`。
実行exe SHA-256: `C1FEF694445F363E575D8374DCDB13E89F3DF66980D84EB5127AEE5EEBD9D68B`。

本体は権利者の指定によりMIT（Copyright 2026 coco-papiyon）。Go・依存ライブラリのライセンスと通知もZIPへ同梱した。build.json、build-info.txt、licenses.json、audit.json、zip.sha256を参照。

## 最終検証

`final/manifest.json` はacceptance=passed、prerequisites_passed=true。配布commitのクリーンなソースでtest/vet、両固定ラッパー（command 7185、file_content 7997）、CGOなしWindows amd64ビルド、固定Windowsコンテナの実SSH・CLI試験、V-01〜18が成功した。全必須サブケースとログの対応はfinal/coverage.json、coverage.csv、scenarios.csvに保存した。ローカルでSKIPされた実機項目は、対応表に指定した実SSH・コンテナ内試験で確認している。

最終ZIPそのものをGo/WSL/Pythonなしの別の固定Server Core 2022コンテナで、管理者でないContainerUserが「日本語 ZIP 配布」フォルダーへ展開した。全ファイルのハッシュと実行exeを照合し、help、SSH経由command/file_content、JSON結果・終了0、再実行changed=falseを確認した。final/ssh/zip-controller.log、controller.jsonにZIP/commit/exeの対応と通常ユーザー・各コンテナ削除の結果がある。

使用イメージは `mcr.microsoft.com/windows/servercore@sha256:b52799a67fa596d4bddcd82ae1dab0077a84767fbc33c7342b72b53ecc71ad09`、Hyper-V隔離、PowerShell 5.1.20348.5773。制御側のWSLランチャー除去など管理者による隔離環境の準備と、通常ユーザーによるZIP導入・利用を分離した。Windows 11/フルServerなど別環境の全試験合格を主張しない。

## 失敗履歴と変更範囲

previous-*は旧候補79068f2の実測結果であり、最終候補の結果ではない。そのZIP実行は成功したが、日本語パスはPowerShell 5.1の文字コード差で未確認だった。日本語フォルダーをUnicodeコードポイントから生成し、文字種と空白を実行時にも検査するよう修正した。

failure-deadline*は旧候補c698817でのSSHの2MiB境界試験の30秒期限超過を保存したもの。失敗を合格へ書き換えていない。7バイト受信を大きくする案は既存OpenSSHの停止挙動を解消できず撤回した。最終候補のreceiver.cs/wrapper.goはc698817と同一。試験側で1MiB超のSSH入力のみ2分まで待機し、モジュール試験全体は8分にした。これは境界の正しさを確認する試験期限であり、製品CLIの既定60秒を変更したものではない。最終試験では2MiB−1/同値/+1と不正入力拒否、EOF、分割転送、切断がすべて成功した。

## 再現と公開条件

配布commitをcheckoutし、同一ZIPを再梱包せず使用する。PowerShell 7、固定Go 1.27.3、Docker Windows containersを備えた検証環境で実行する。

```powershell
$env:WINTABLE_DISTRIBUTION_ZIP=(Get-Item dist/mit-verified-candidate/*.zip).FullName
pwsh -NoProfile -File tests/validation/run.ps1 -OutputDirectory dist/mit-verified-acceptance
pwsh -NoProfile -File scripts/verify-acceptance.ps1 -Zip $env:WINTABLE_DISTRIBUTION_ZIP -Evidence docs/validation/issue12-mit/acceptance.json
```

出力先は未作成ディレクトリを指定する。生ログと各ハッシュを確認し、保存証跡の再集約はsummarize.ps1へfinalディレクトリを渡す。証跡は.gitattributesにより改行変換を無効化し、sha256.csvで生バイトを検査する。秘密鍵・exe・ZIP・Docker contextはコミットしていない。最終ZIPはローカルdist/mit-verified-candidateに保持している。

acceptance.jsonは整合性検査を通過した。公開前には人による実ログ・最新監査・ライセンスの確認が必要であり、自動検証を公開承認の代用としない。PR作成・マージ・リリース公開は行っていない。証跡保存後のcommit差分は文書・記録のみで、検証した製品やZIPを変更しない。
