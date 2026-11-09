# 標準runnerと必要時のローカル実SSH検証

追加のVM契約・larger runner・常設self-hosted runnerを使わない。
公開リポジトリのGitHub標準runnerによる実行を使用する。
[GitHubの料金条件](https://docs.github.com/en/actions/concepts/billing-and-usage)に従い、
このworkflowはprivateリポジトリでは起動しない。artifact保存容量は別途プランの上限を確認する。
保存期間は3日とし、不要になったartifactはGitHub上で削除できる。

## GitHub Actionsで実行する範囲

`.github/workflows/ssh-acceptance.yml` のbuild/windowsは標準 `windows-2022` を使う。
固定Go 1.27.3で監査・license・ZIP内容検査を含む既存配布処理を実行し、
`candidate` artifactへ `package/` と生成済み `package.zip` を保存する。
Windows jobはそのexeとZIPを使い、V-01〜V-18・両module・非管理者ZIP導入を実行する。
Windows Server 2022の固定base digestを維持し、Dockerの **process isolation** を指定する。
Hyper-Vのnested virtualizationは要求しない。

実行前の `Windows container capability` stepは、Dockerサービスと固定imageの
process-isolated container起動を実測し、`hosted-windows-capabilities` に結果と清掃状態を保存する。
標準runnerのimage変更等で利用できなければjobを失敗にし、未実行を合格にはしない。
[公式runner image](https://github.com/actions/runner-images/blob/main/images/windows/Windows2022-Readme.md)
のDocker搭載だけを実SSH成功の証拠とは扱わない。

成功時の `windows-evidence` は固定ファイル名の許可リストで抽出する。
秘密鍵、fixture directory、Docker build context、SSH設定、環境dumpを一括uploadしない。
通常のPR/push CIと監査も維持する。fork PRをself-hosted PCで自動実行する構成はない。

## 既存Windows PCで必要時に実行する範囲

Windows標準runnerから固定Linux Dockerへ到達する経路を確保するために、
追加VMや公開SSH・外部中継サービスを契約しない。Linux実SSHは既存PCで必要時に実行する。
この実行を「GitHub Actions上のLinux実SSH」とは呼ばない。
PCをGitHub runnerとして登録したり、常駐サービスを入れたりする必要はない。

1. GitHub Actionsの共通buildとWindows jobの成功を待つ。
2. 実行対象SHAをクリーンにcheckoutする。PowerShell 7.3以上、Go 1.27.3、認証済みgh、
   Docker 28以上の `desktop-linux` contextとOpenSSH clientを用意する。
3. 次を実行する。run IDは成功した同じSHAの `Standard runner SSH candidate` workflowのID。

```powershell
./tests/ci/complete-local.ps1 -RunId <run-id> -DockerContext desktop-linux
```

helperは実際のActions APIでrepository/workflow/head SHAと両job結果を検査し、
同じrunのcandidateとWindows証跡を取得する。ローカルでexeを再buildしない。
固定Debian image/CPython 3.13.5の実SSHと配布exeの日本語文書例を実行し、
`tests/ci/gate.ps1` で両OS証跡・同一SHA・exe/ZIPハッシュ・全必須ケース・清掃を照合する。
旧SHA、汚れた作業ツリー、別workflow、欠落artifact、実テスト失敗、skip/欠落/重複は拒否する。
artifactが失効した場合は、同じSHAのworkflow全体を再実行してから新しい結果を使う。

成功時は新規 `.cache/local-acceptance/<id>/` 以下へ次を保存する。

- `validated-distribution.zip`: Windowsで検証したZIPのバイト列をそのままコピーしたもの。
- `evidence/publication.json`: 必須検証の集約、SHAとハッシュ、Windows/Linuxの実行場所。
- `evidence/execution.json`: CI run URL/attemptとローカル実行の区別。
- `evidence/windows/`、`evidence/linux/`: 許可リストの証跡。任意の実タスク入力を混ぜない。

候補artifactだけでは公開条件を満たさない。このhelperが成功した同じZIP・ハッシュ・証跡を
人手で確認して公開する。GitHub Releaseへの自動投稿は行わない。

## 標準Windows containerが利用できなかった場合

能力確認step自体が失敗した場合だけ、既存PCでWindowsも実行できる。
通常の実SSHテスト失敗をローカル実行で上書きして合格にはしない。

```powershell
./tests/ci/complete-local.ps1 -RunId <run-id> -WindowsLocal `
  -WindowsDockerContext desktop-windows -DockerContext desktop-linux
```

Windows用には既存のHyper-V/Windows containers環境とOpenSSH serverバイナリが必要。
その後にLinux contextを使用する。同時稼働を前提にせず、必要なモード切替は既存環境で準備する。
helperはDockerのグローバルcontextやモードを切り替えず、環境変数を復元する。
CIの能力確認artifactが同じSHAで `unavailable` を記録し、`container_removed` が
真偽値の `true` であることを必須とする。清掃失敗・清掃結果の欠落は代替実行でも拒否する。

## 必須検査と範囲

| 基準 | 検証 |
| --- | --- |
| L01 | TestHostOS / TestOSSchemaBoundaries / TestOSPreflightNoConnections。Linux file_contentは全ホスト接続前拒否 |
| L02 | 固定独立probeの2MiB/EOF/両stream/鍵拒否/期限・中断/再送なし |
| L03〜L04 | Windows製品exeのargv/出力/終了値、raw receiverの不正入力・runtime・E2BIG、文書例 |
| L05〜L06 | 模擬混在、個別実SSH、合成境界、固定起動長。層別に記録 |
| Windows V-01〜V-18 | 既存coverage/summarizeとZIP導入。期待値を緩めない |

混在単一runの実機E2E、専用remote Docker/tunnelは未確認。
Windows制御・Debian 13 amd64/CPython 3.13のみ。Linux file_contentは未対応。
切断後の子孫停止は保証しない。対応範囲は [support.md](support.md) を参照。
Linuxはready60秒、試験600秒、清掃90秒。Windows jobは35分、能力確認は5分。
強制中断時はfixture READMEの実行ID別回収手順を使い、共有Docker資源のpruneは行わない。
