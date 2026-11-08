# OS/module backend境界

`module.Backends` は `module.Key{OS, Module}` ごとにPrepare、固定Command、
実際のLaunchLine、そのOS用ValidateLaunch、Decodeを登録する。
`engine.BuildWithBackends` は全ホストの変数展開と能力・引数・JSONサイズ・固定行の
検査を完了してからPlanを返す。エラーでは部分Planを返さない。
選択したbackendを値コピーしてTaskPlanへ保持し、Executeは全件のOS/module整合性を
接続前に再検査する。登録欠落時のWindows fallbackはない。
本番は `WindowsBackends` のcommand/file_contentだけを登録する。

`Prepare` はWindows専用のCRT引用、32,766 UTF-16単位と従来の対象パス検査を維持する。
`PrepareLinuxCommand` は実行機能ではなく後続backend向けの検査・共通JSON生成関数。
POSIX絶対パス、制御文字・末尾/・ドットセグメント拒否、UTF-8/NUL検査、
各値32KiB・NUL込み合計64KiB・argv4096要素を境界込みで適用する。
固定行はWindows 8,000 UTF-16単位とLinux 8KiB UTF-8を別々に検査する。
タスク値は固定行へ連結せず、所有権を持つPreparedのJSONをstdinへ送る。
SSH、順序、forks、timeout/cancel、結果描画は共用する。

既存のRuntime.Wrappers/Decode注入APIはWindows専用の互換口。
新規backendはRuntime.Backendsへ登録し、同じ登録をpreflightで使用する。
旧Prepared.RequestもWindows専用。backend実行は選択済みBackend.Requestを使う。
[互換表](../README.md#inventory-osとcli互換性)、[ADR-0001](adr/0001-linux-target-support.md)を参照。
