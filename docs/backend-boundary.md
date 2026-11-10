# OS/module backend境界

`DefaultBackends` は Windows powershell / Linux shell も登録する。共通 `PrepareScript` と OS別固定 receiver を使い、command の直接 argv 実行とは別の契約とする。変数展開後のスクリプトを全件検査し、逆OSは接続前拒否。[終了規則・上限・一時領域](script.md)を参照。

`module.Backends` は `module.Key{OS, Module}` ごとにPrepare、固定Command、
実際のLaunchLine、そのOS用ValidateLaunch、Decodeを登録する。
`engine.BuildWithBackends` は全ホストの変数展開と能力・引数・JSONサイズ・固定行の
検査を完了してからPlanを返す。エラーでは部分Planを返さない。
選択したbackendを値コピーしてTaskPlanへ保持し、Executeは全件のOS/module整合性を
接続前に再検査する。登録欠落時のWindows fallbackはない。
本番は `DefaultBackends` にWindows command/file_contentとLinux command/file_contentを登録する。
Linux file_contentは独立したPrepare・receiver・changed/error decoderを使う。

`Prepare` はWindows専用のCRT引用、32,766 UTF-16単位と従来の対象パス検査を維持する。
`PrepareLinuxCommand` はLinux backend向けの検査・共通JSON生成関数。
POSIX絶対パス、制御文字・末尾/・ドットセグメント拒否、UTF-8/NUL検査、
各値32KiB・NUL込み合計64KiB・argv4096要素を境界込みで適用する。
固定行はWindows 8,000 UTF-16単位とLinux 8KiB UTF-8を別々に検査する。
タスク値は固定行へ連結せず、所有権を持つPreparedのJSONをstdinへ送る。
SSH、順序、forks、timeout/cancel、結果描画は共用する。

既存のRuntime.Wrappers/Decode注入APIはWindows専用の互換口。
新規backendはRuntime.Backendsへ登録し、同じ登録をpreflightで使用する。
旧Prepared.RequestもWindows専用。backend実行は選択済みBackend.Requestを使う。
[互換表](../README.md#inventory-osとcli互換性)、[ADR-0001](adr/0001-linux-target-support.md)を参照。

## fetch backend

両OSのfetchはsrcだけをstdinへ直列化し、絶対化したローカル保存先をPrepared.FetchDestinationへ保持します。全件の保存先衝突と既存のリンク祖先をBuildで検査します。Runtimeは接続前にStreamingConnector能力を検査し、RunFetchが専用Stream・整合性検証・NTFS保存を所有します。通常Decodeへバイナリを渡しません。Windows固定bootstrapはASCIIの-Commandで配布コードを展開し、タスク値は一切連結しません。上限は従来の8,000 UTF-16単位を維持します。[仕様](fetch-module.md)。

## ローカルcommand委譲

`delegate_to: "localhost"` を指定したcommandはWindows制御端末上で実行します。`become` は省略または `false` のみで昇格しません。元ホストごとに変数展開・実行し、結果も元ホストに所属します。全taskが委譲ならSSH接続属性・鍵・known_hostsは不要です。絶対exeとargv、出力上限、期限・中断時の子孫清掃などは[ローカルcommandの仕様](local-command.md)を参照してください。
