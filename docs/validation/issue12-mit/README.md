# MIT採用・配布候補の検証記録

本体ライセンスはユーザーの指定によりMIT（Copyright 2026 coco-papiyon）。Go・依存ライブラリのライセンスと通知は別途同梱する。

最終候補source_commitは `c698817112724af47d7147d74fa21386dc5507f8`。ZIPハッシュはzip.sha256、ビルド情報・依存一覧・監査応答は同梱の各ファイルを参照。test/vet、固定起動長（command 7185、file_content 7997）、CGOなしWindows amd64ビルド、govulncheck v1.9.0、ZIP再展開と全ファイルハッシュ検査は成功した。監査DBは2026-11-05T23:49:57Z。

前回実行 `79068f225778379ba923f5c3004997b06f35cf29` は全18シナリオpassed。previous-*はその実測結果であり、最終候補の結果ではない。通常ユーザーContainerUserでZIP展開、Go/WSL/PythonなしのSSH実行、command/file_contentと再実行時のchanged=falseを確認した。ただしWindows PowerShell 5.1がBOMなしスクリプトの日本語リテラルを誤読していたため、日本語パスの条件は未確認とする。

最終候補では日本語フォルダーをUnicodeコードポイントから生成し、日本語と空白の存在を実行時に検査するよう修正した。修正版の最終ZIPでのコンテナ再検証は実行承認が拒否され、未実施。旧結果を流用して最終候補の合格としない。V-13と配布commitに対応する全必須合格の確認が残るため公開不可。ZIPを再生成せず、同一ZIPで再検証する。

再実行（管理用環境のPowerShell 7、Docker Windows containers、固定Go 1.27.3。ZIP導入・利用自体に管理者権限は不要）：

```powershell
$env:WINTABLE_DISTRIBUTION_ZIP=(Get-Item dist/mit-release/*.zip).FullName
pwsh -NoProfile -File tests/validation/run.ps1 -OutputDirectory dist/mit-release-acceptance
```

保存先は未作成ディレクトリとする。ログには対象環境の識別情報が含まれるため公開前に内容を確認する。全18passed、ZIP_CONTROLLER_PASS、ContainerUserのnon_admin=true、日本語パス、ZIP/実行exe/配布commitの対応、コンテナ削除を確認してから受け入れmanifestを作成し、verify-acceptance.ps1と実ログレビューを行う。
