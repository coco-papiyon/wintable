# Windows制御の実SSH CIと配布

`.github/workflows/ssh-acceptance.yml` はpushされたSHAを全ジョブで明示checkoutする。
固定Go 1.27.3でWindows amd64 exeを一度buildし、配布manifestとSHA-256を付ける。
Windows対象ジョブとLinux対象ジョブは同じartifactを取得し再buildしない。
配布候補には埋込Windows/Python receiverを含むexe、README、docs、examplesを同梱し、
ZIP展開後の全ファイルhashとexe helpを検査する。ランタイムの外部Pythonソース配布は不要。
候補作成は既存の `scripts/distribution.ps1` を再利用し、依存脆弱性監査、MITライセンス、
Goと全依存のライセンス・通知、配布ZIP検査を維持する。監査の失敗も公開を拒否する。

## runnerの用意

GitHub Actionsが有効であることに加え、専用のWindows amd64 self-hosted runnerが必要。
PowerShell 7.3以上、Go 1.27.3取得経路、Docker 28以上、OpenSSH clientを用意する。
Windows用はHyper-V/Windows containersとOpenSSHサーバーバイナリを用意し
`wintable-windows-ssh`、Linux containers用は`desktop-linux` contextを用意し
`wintable-linux-ssh`ラベルを付ける。Windows/X64既定ラベルも必要。
用途別runnerを分離し、同じDocker daemonをモード切替する並行ジョブは登録しない。
使い捨てrunnerを推奨する。workflowはfork PRのコードを自動実行しない。
Issue #29実装時点のAPI確認ではrunner登録0件。登録と実行成功を文書だけで代替しない。

Windows runnerからLinux Dockerへの接続はローカルLinux containersを既定経路とする。
Linux試験の成功はWindowsからloopback SSHへ実到達した証跡を必要とする。
専用remote Linux Dockerホストに変更する場合は
[fixture手順](../tests/linux-ssh/README.md#dedicated-remote-linux-docker-host)に従い、
事前検証した管理host鍵・公開鍵認証とloopbackトンネルを使う。
workflowのLinux呼出しに `-DockerContext <alias> -TunnelHost <same-alias>` を設定する。
未検証ssh-keyscan・自動信頼・公開SSHポートは使わない。remote方式は未実証。

## 必須検査と公開条件

| 試験基準 | 検証層・証跡 |
| --- | --- |
| L01 OS/全件preflight | Windows全体Go検査のTestHostOS/TestOSSchemaBoundaries/TestOSPreflightNoConnections、backend境界検査。未対応Linux file_contentは全ホスト接続0 |
| L02 SSH基盤 | test.jsonl、固定独立probe。UTF-8分割read・2MiB/EOF・両stream・拒否・timeout/cancel/切断・再送0 |
| L03 command | product-argv/product-exits、Windows製品exe→sshd→receiver。文書同梱whoami例も実行 |
| L04 receiver | raw-invalid/raw-EOF/raw-runtime/raw-E2BIG。生receiverの不正入力・runtime shimであり製品CLI試験とは区別 |
| L05 共通実行契約 | TestMixedBackends等の模擬混在、個別実SSHの接続再利用/失敗後skip/中断。混在実機E2Eは未確認 |
| L06 境界 | raw-boundaries、product-output、合成TestLinuxDecoder、固定起動長検査。CLI到達境界と生receiver合成を区別 |
| Windows V-01〜V-18 | 既存coverage.json/summarize.ps1。配布exeのclean controller・command/file_content回帰を維持 |

最終ジョブはalways実行し、両試験jobのsuccess、同一SHA/バイナリ、必須サブケースの
一意なPASS、版照合、有限期限、清掃成功を検査する。親PASSで子SKIPを補わない。
失敗・取消・未実行・欠落artifact・null終了値・重複は公開不可。
合格時だけ`validated-distribution`を生成する。GitHub Releaseへの自動投稿は行わない。
Windowsでは同じ候補をZIP化し、既存の非管理者・日本語空白パス・Go/WSL/Pythonなしの
導入検査も実行する。`zip-controller.log` の完了マーカー、終了0、コンテナ清掃が必須。
公開者はこのworkflowの同一SHAの`publication-gate`合格とartifactを使う。
branch protectionの必須チェックにも同名jobを設定する。実装時点で保護設定は未変更。

## 証跡と期限

Linuxはready60秒、全体600秒、finally清掃90秒。jobは15分。
Windows jobは35分、Go/コンテナ操作は既存ランナーの個別有限期限を持つ。
バージョン、hash、期待/実測、終了値、開始終了、資源と清掃を保存する。
`export-evidence.ps1` は固定ファイル名の許可リストだけを14日保存し、
fixtureディレクトリ・鍵・SSH設定・環境dump・任意コマンド出力をuploadしない。
試験入力は固定の合成値だけ。資格情報をサンプルへ書き込まない。
候補artifactは7日、合格配布artifactと結果は14日保存する。

強制中断ではfinallyが完了する保証はない。Linuxは
[実行ID別回収手順](../tests/linux-ssh/README.md#evidence-ownership-and-recovery)を使う。
Windowsはmanifestのimage/container名をDocker inspectで照合してその実行分のみ削除し、
証跡フォルダのclient秘密鍵が残っていればそのファイルのみ削除する。
共有pruneは使わない。清掃証跡のないrunは再利用せず不合格とする。

## ローカル再現

```powershell
./tests/ci/package.ps1 # dist/packageは新規ディレクトリ
Copy-Item dist/package/wintable.exe dist/wintable.exe
./tests/ci/test-gate.ps1
./tests/validation/test-summary.ps1
# Windows containersモード
Compress-Archive -Path dist/package/* -DestinationPath dist/windows-candidate.zip
$env:WINTABLE_DISTRIBUTION_ZIP="$pwd/dist/windows-candidate.zip"
./tests/validation/run.ps1 -DistributionDirectory "$pwd/dist/package" -OutputDirectory .cache/ci/windows
# 専用Linux containers環境で、同じcheckoutと配布物を使う
./tests/linux-ssh/run.ps1 -DockerContext desktop-linux -ProductTestScript ./tests/linux-ssh/product.ps1 -OutputDirectory .cache/ci/linux
./tests/ci/gate.ps1 -EvidenceDirectory .cache/ci -DistributionDirectory dist/package -SourceSHA (git rev-parse HEAD) -WindowsJob success -LinuxJob success
```

文書例は`examples/linux-inventory.json`と`examples/linux-command.json`。
接続先、user、Windows制御側の鍵パスを実環境へ変更して実行する。
混在例は`mixed-inventory.json`/`mixed-command.json`。`${exe}`をhost変数に持ち、
共通taskをOS別絶対パスへ展開する。os省略はWindows、os付きv1は旧CLIでは未知キー終了2。
