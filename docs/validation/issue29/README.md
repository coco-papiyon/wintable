# Issue #29 検証記録

実装・実行対象SHA: `0623209ff3004aa704ddf05d1abfa9e2811a1386`
比較先main: `56db6e56c65e787435238deceb0a460524680dfc`
Windows製品exe SHA-256: `0498845FA630949B21D259EA04C6B22A6470031694330591EACD5C7EE058034C`

## 確認結果

- Go 1.27.3 Windows amd64で `go test ./...`、`go vet ./...`、`build.ps1` 合格。
- 配布候補のZIP作成・展開後全ファイルhash・製品exe help・必須同梱文書/例の検査は合格。
- `tests/ci/test-gate.ps1` の合成証跡15ケース合格。正例、および子skip/欠落/重複、null終了値、異なるSHA/バイナリ、版不一致、未清掃、期限超過、片側job失敗/skip/取消、artifact欠落、Windows必須ケース欠落を検査。これはゲートの試験でありLinux実SSHの合格証跡ではない。
- 既存 `tests/validation/test-summary.ps1` の全正負例合格。
- `tests/validation/run.ps1 -DistributionDirectory "$pwd/dist/package" -OutputDirectory "$pwd/.cache/ci/windows"` 合格。V-01〜V-18、通常/clean controllerでの共通配布exe、Windows command/file_content実SSH、固定起動長、SSH拒否/中断/再送0を確認。
- Windows専用container/imageとclean controller/native containerを清掃済み。許可リストで抽出した証跡を `windows/` に保存。秘密鍵、fixture.json、known_hosts、build context、任意対象出力は含めない。

## 未確認・完了を保留する範囲

- GitHub Actions用self-hosted runnerはAPI確認時0件。workflowは実装済みだが、CI上のWindows→Linux Docker実到達、同一SHAでの両OS jobとpublication-gate合格は未確認。
- 本実行のLinux実SSH検証は実行承認が拒否されたため未実行。L01〜L06と配布物のLinux文書例の実SSH合格を本記録では主張しない。
- 混在単一runの実機E2E、専用remote Docker/tunnelは未確認。模擬混在と個別実SSHを区別する。
- Linux file_contentは未対応。登録前拒否を維持し、L07は#30へ残す。
- 固定Debian 13 amd64以外、Linux制御端末、sudo等と切断後の子孫停止は保証対象外。
- 対応公開・Issue完了の条件には未確認が残る。runnerを登録し、このSHA以降の同一SHA/共通exeで両jobと最終ゲートを実行する必要がある。

後続の証跡保存commitは本記録と抽出証跡のみを追加する。実装・試験コードは上記SHAから不変。
実行Agent: Codex（製品バージョン未確認）。PowerShell、Go、Docker CLI、git、認証済みgh/gh apiを使用。
