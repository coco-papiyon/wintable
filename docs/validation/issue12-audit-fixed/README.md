# 脆弱性監査修正後の最終ZIP検証

配布commit: `31b3ae7918603960975c4bafc4e05ce75e87e3a0`。
ZIP SHA-256: `ca459e3fa8171d444e9b2790ccd501a63950783808593613f31ab9147e135217`。
exe SHA-256: `C7BAD84A7C9697552422C52F10DD2B4C268B77D3886C28B5B835AF6CD6CB0F14`。

Issue #12の要修正2回目に対応。golang.org/x/cryptoを `v0.58.1-0.20261009181718-2f1f834330b2` へ更新した。govulncheck v1.9.0のJSONモードはfindingがあっても終了0になるため、CIと候補生成で `scripts/auditcheck` による明示的な全finding拒否を追加した。参考情報のosvメッセージだけでは拒否しない。空・不正なJSON、設定・SBOM欠落は拒否する。旧候補の実audit.jsonに対してGO-2026-5932を理由に終了1を確認し、9ケースの回帰テストも成功した。

## 実行結果

`distribution.ps1 -OutputDirectory dist/audit-fixed-candidate` はmod verify、全Goテスト、vet、両ラッパー長検査、監査、CGOなしWindows amd64ビルド、Go・依存のライセンス収集、ZIP生成・全ファイルのハッシュ照合まで成功。新audit.jsonにfindingなし。DB更新日時は2026-11-05T23:49:57Z。製品MIT LICENSE・依存ライセンス・通知を同梱した。[配布commitのWindows CI](https://github.com/coco-papiyon/wintable/actions/runs/42490337732)も成功した。

同一ZIPを `WINTABLE_DISTRIBUTION_ZIP` に指定し、`tests/validation/run.ps1 -OutputDirectory dist/audit-fixed-acceptance` を実行、終了0。final/manifest.jsonはacceptance=passed、prerequisites_passed=true。V-01〜18の全必須項目が合格した。実SSH、認証、通信障害、ACL・reparse、2MiB境界、EOF・分割転送、実Ctrl+C、出力障害を含む。ローカルでSKIPされた項目はcoverage.csvに指定された実SSH・コンテナ試験で確認した。

固定Server Core 2022イメージ `mcr.microsoft.com/windows/servercore@sha256:b52799a67fa596d4bddcd82ae1dab0077a84767fbc33c7342b72b53ecc71ad09`、Hyper-V隔離、PowerShell 5.1.20348.5773で検証。V-13はGo/WSL/Pythonなし、ContainerUser、日本語・空白入り展開先、help・SSH command/file_content・再実行changed=false・結果JSON・終了0を同一ZIPで確認した。controller.jsonとzip-controller.logにZIP/commit/exe対応と通常ユーザーの実測がある。全一時コンテナ・fixtureイメージの削除を確認した。Windows 11/フルServer等の別環境の全試験合格は主張しない。

## 証跡と再確認

生ログはfinal/、ZIP・commit・各シナリオの対応はacceptance.json、全保存ファイルの生バイトハッシュはsha256.csv。旧候補の記録はissue12-mitに監査不合格の履歴として保持した。秘密鍵・exe・ZIP・Docker contextは保存対象から除外した。

```powershell
./scripts/verify-acceptance.ps1 -Zip (Get-Item dist/audit-fixed-candidate/*.zip) -Evidence docs/validation/issue12-audit-fixed/acceptance.json
```

最終ZIPはローカルdist/audit-fixed-candidateに保持し、試験後の再梱包はしていない。証跡保存後の差分は文書と記録だけであり、製品・監査ゲート・検証手順は配布commitと同一。公開前には人による実ログ・最新監査・ライセンスのレビューが必要。リリース公開は未実施。
