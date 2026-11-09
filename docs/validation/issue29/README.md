# Issue #29 検証記録

## 現在の実行方針と対象

2026-11-09のユーザー指定により、標準GitHub runnerを先に検証し、難しい部分は既存PCで必要時に実行する方針へ変更した。常設self-hosted runnerや追加VM契約は不要。

実装・実行対象SHA: `567b564f0b5bb03a717c7f8cd3bf5dc4da6c68ae`
比較先main: `ca736389edf245cdcff5f45194c96d8e3e5ea51c`
共通Windows製品exe SHA-256: `ECACA47DF6394F9E97327CA907A0CE72BED2034B558BE36DF968121B770A61EA`
検証済みZIP SHA-256: `3034A98652F626CA39395CA964AD56DB249C7404ECF0EE82498CF623DF7889D4`

この記録の文書更新commit自体を実SSHで検証済みとは扱わない。上記SHAの実装・配布物に対する結果である。

## 確認結果

| 実行場所 | 検証 | 結果・参照 |
| --- | --- | --- |
| GitHub標準 `windows-2022` | Go 1.27.3、全Goテスト、vet、固定ラッパー、脆弱性監査、配布回帰 | [通常Windows CI](https://github.com/coco-papiyon/wintable/actions/runs/42557016861) success |
| GitHub標準 `windows-2022` | 共通exe/ZIPの生成、監査、製品・Go・依存ライセンス、ZIP内容検査 | [標準runner workflow](https://github.com/coco-papiyon/wintable/actions/runs/42557016954) のbuild success |
| GitHub標準 `windows-2022` | 固定Windows imageのprocess isolation起動、OpenSSH能力確認、V-01〜V-18 | 同workflowのwindows success。両module実SSH、CLI、非管理者ZIP導入、日本語空白パス、Go/WSL/Pythonなし、起動行、EOF・中断、清掃まで合格 |
| 既存Windows PC | 同じCI配布exeによる固定Debian 13 amd64 / CPython 3.13.5の実SSH | L01〜L06の層別必須ケース、配布exeの日本語文書例、版照合、4種の清掃検査に合格 |
| 既存Windows PC | CI Windows証跡とローカルLinux証跡の集約 | 同一SHA・exe/ZIPハッシュ、全必須ケース、期限、清掃を照合し `publication.json` が `passed` |

Windowsの能力確認・実SSHにはprocess isolationを使用し、nested Hyper-Vを要求していない。Linux試験はローカル実行であり、GitHub Actions上のLinux実SSH成功とは扱わない。

ゲート合成21ケース、CI取得条件の正負例9ケースが合格。旧SHAの実際のCI runを、補完スクリプトがartifact実行前に拒否することも確認した。失敗・skip・欠落・重複・null終了値・別SHA・exe/ZIP不一致・未清掃を合格にしない。

## 実行手順と証跡

対象SHAをクリーンにcheckoutし、同じSHAの標準runner workflowが成功した後、既存Windows PCで実行する。exeのローカル再buildは行わない。

```powershell
./tests/ci/complete-local.ps1 -RunId 42557016954 -DockerContext desktop-linux `
  -OutputDirectory .cache/issue29-standard-complete
```

この実行で生成したローカル証跡・配布物は次のとおり。本ディレクトリに同梱したファイルではない。

- `.cache/issue29-standard-complete/evidence/windows/`: 同じCI runのWindows証跡。
- `.cache/issue29-standard-complete/evidence/linux/`: 既存PCのLinux実SSH・文書例・版・清掃証跡。
- `.cache/issue29-standard-complete/evidence/publication.json`: `status=passed`、上記SHAとハッシュ、`windows_execution=github-actions`、`linux_execution=local`。
- `.cache/issue29-standard-complete/evidence/execution.json`: workflow run `42557016954`、attempt `1`、実行場所。
- `.cache/issue29-standard-complete/validated-distribution.zip`: Windowsで検証したCI候補ZIPを再梱包せずコピーしたもの。

CIの `candidate`、`windows-evidence`、`hosted-windows-capabilities`、`candidate-audit` は3日保持。失効後は同じSHAのworkflowを再実行して、その結果を使う。再実行時の出力先は新規ディレクトリにする。詳細と能力不足時の手順は [CI手順](../../ci.md) を参照。

秘密鍵・fixture directory・Docker build context・SSH設定・任意の実タスク入力を証跡へ一括収集しない。CIとローカルの証跡は許可リストで抽出し、専用資源の清掃結果を確認した。

## 保証範囲と未確認事項

- 模擬混在＋個別Windows/Linux実SSHを検証。混在単一runの実機E2Eと専用remote Docker/tunnelは未確認。
- 今回は標準Windows環境で全試験を実行できた。能力不足時の `-WindowsLocal` 経路は条件判定の正負例で検証しているが、今回の実機通し実行には使用していない。
- Linux file_contentは未対応で、全接続前拒否を維持する。L07は後続範囲。
- 制御端末はWindows amd64、Linux対象は固定Debian 13 amd64 / CPython 3.13。Linux制御端末、他ディストリビューション/arch、sudo等は保証対象外。
- 通信切断・期限切れ・キャンセル後の子孫停止は保証しない。

現行のユーザー指定方針に対する必須試験と集約は合格。対応範囲は [対応表](../../support.md)、要件別の結果は [実装2回目の報告](https://github.com/coco-papiyon/wintable/issues/29#issuecomment-6731807453) を参照。

## 同梱している過去のWindows証跡

既存の `windows/` は、次の旧SHAに対するWindows検証記録として保持している。今回の標準runner実行や最新Linux実SSHの証跡と混同しない。

- 実装・実行対象SHA: `0623209ff3004aa704ddf05d1abfa9e2811a1386`
- 比較先main: `56db6e56c65e787435238deceb0a460524680dfc`
- exe SHA-256: `0498845FA630949B21D259EA04C6B22A6470031694330591EACD5C7EE058034C`

実行Agent: Codex（製品バージョン未確認）。PowerShell、Go、Docker、git、認証済みgh/gh apiを使用。
