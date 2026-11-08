# Windows配布候補と公開条件

現状は公開不可です。[Issue #11の最新検証](https://github.com/coco-papiyon/wintable/issues/11#issuecomment-6717454311)は全体OKで、検証headは `40ba3c7675418c0df2e020ec68f2e5a6098918f8` です。ただし、その合格範囲は固定Server Core 2022コンテナの既存実測であり、本Issueの最終ZIPや配布commitの合格を示しません。製品本体は権利者の指定によりMITライセンスを採用し、リポジトリのLICENSEを同梱します。Go・依存ライブラリのライセンス本文と通知は別途保持します。残る公開条件は、配布commitへの全必須証跡の対応付けと、同一の最終ZIPによるV-13の合格です。

## 候補の生成

PowerShell 7、git、固定Go 1.27.3、ネットワーク接続を備えたWindows amd64で、クリーンな対象commitをcheckoutします。Goの選択はGOTOOLCHAIN=go1.27.3で固定し、test/vet・固定ラッパー検査・CGO無効ビルド・govulncheck v1.9.0を実行します。

```powershell
./scripts/distribution.ps1 -OutputDirectory candidate -ProductLicense LICENSE
```

出力先は未作成のフォルダーを指定します。監査は標準ライブラリを含む到達可能な脆弱性を検査します。検出・通信障害・ツール障害はすべて停止条件です。audit.jsonにはスキャナー、Go、DB更新情報、findingが保存されます。モジュールに存在するが到達しない脆弱性はgovulncheckのsymbolモードの対象外であり、別途リスクレビューします。ライセンスはgo list -m allの推移依存を含む固定版から収集し、Go本体のLICENSEと一覧を同梱します。ライセンス欠落も停止条件です。

ZIPはexe、examples、利用手順、製品LICENSE、依存・Goライセンス、audit.json、commit/Go/ツール版を記したbuild.json、全配布ファイルのSHA256.jsonを含みます。ZIP自体のハッシュは外部 `.zip.sha256` へ出力します。buildvcsとtrimpathを使用します。同じ固定ソースから同じ手順で再作成できますが、監査DB・生成日時・ZIP時刻が変わるためZIPのバイト同一性は約束しません。

```powershell
$zip = Get-Item ./candidate/*.zip
$hash = (Get-FileHash $zip -Algorithm SHA256).Hash
./scripts/verify-package.ps1 -Zip $zip -ExpectedSHA256 $hash
```

CIはPR/pushでテスト・vet・両ラッパー検査・ビルド・監査を行います。手動candidate workflowは同じ候補生成処理を行い、成功時だけartifactを保存します。公開権限も公開ステップもありません。通常CI成功を実機合格と扱いません。

## 同一ZIPの実機検証と公開

1. #11の合格証跡（上記コメントと `docs/validation/evidence/20261108-complete/`、`20261108-eol-recheck/`）を取得し、V-01〜18の全必須サブケースを配布commitに対応付けます。配布ブランチに証跡・集約器がない場合は、それらを取り込んでから保存証跡の再集約を行います。SHAが異なる過去証跡だけでは合格にせず、差分を確認して影響する検証を更新します。
2. 最終候補ZIPをGo/WSL/Pythonなしのクリーンな固定Windowsコンテナへコピーします。OSイメージdigest、OpenSSH/PowerShell版、対象側cmd既定シェル、制御・対象環境を記録します。制御側の展開・実行は通常ユーザーで行い、日本語・空白入り展開先からhelp、SSH接続、command、file_content、再実行時のchanged=false、JSON結果と終了値を確認します（V-13）。対象側SSH準備の権限と区別します。
3. ZIP SHA-256、build.jsonのsource_commit、全シナリオの実コマンド・終了値・環境情報・生ログを保存し、以下のmanifestへ記録します。各Vのlogは全必須サブケースの結果と根拠への索引を含めます。元ログを編集・正規化せずハッシュを採取します。
4. `verify-acceptance.ps1`でZIP・ソース・全18件とログハッシュを照合します。別SHA、未確認、NG、欠落、ZIP改変は拒否します。これは証跡の整合性検査であり、内容の真正性や試験実施を証明するものではありません。
5. レビュアーが実ログ、#11、最新の依存監査、ライセンス本文・通知、全必須サブケース合格を確認します。commitが最新リリース対象と一致することも確認し、同じZIPと外部ハッシュ、証跡を手動で公開します。合格後の再ビルド・再梱包は禁止し、変更時は再度V-13を行います。未確認が一つでもあれば公開しません。

manifestのスキーマ例（以下は未確認の雛形であり合格記録ではありません）：

```json
{
  "source_commit": "40桁の配布commit",
  "zip_sha256": "64桁のZIPハッシュ",
  "issue11_url": "https://github.com/coco-papiyon/wintable/issues/11#issuecomment-実証跡",
  "reviewer": "確認者",
  "clean_container": false,
  "no_go_wsl_python": false,
  "non_admin_install": false,
  "japanese_space_path": false,
  "scenarios": [{"id":"V-01", "status":"unconfirmed", "log":"logs/V-01.txt", "sha256":"64桁"}]
}
```

```powershell
./scripts/verify-acceptance.ps1 -Zip ./candidate/wintable-windows-amd64-<sha>.zip -Evidence ./acceptance/manifest.json
```

Go・依存・govulncheck・Actions SHAの更新は専用変更で行い、互換性と監査結果を確認して固定値を更新します。go.mod、build.ps1、両workflow、distribution.ps1、文書を整合させ、test/vet・起動長・ビルドと影響する実機試験を再実施します。DBは常に最新を使用し、その応答をaudit.jsonへ記録します。
