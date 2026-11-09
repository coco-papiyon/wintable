# Windows制御端末でのcommand実行

taskに `"delegate_to": "localhost"` を指定すると、Windows制御端末のwintableと同じユーザー・権限でcommandを実行します。`become` は省略または `false` のみ有効で、昇格しません。`true`、文字列、nullは実行前に拒否します。

```powershell
.\dist\wintable.exe run --inventory examples/local-inventory.json --tasks examples/local-tasks.json --include-output
```

実行入力は既存Windows commandと同じ絶対パスのCRT互換 `.exe` と `argv` です。PATH検索、`cmd: "hostname"`、暗黙のシェル実行、Ansible YAMLは扱いません。例はWindowsを `C:\Windows` に導入した環境向けです。

委譲先は `localhost` のみで、対象モジュールはcommandのみです。属性を省略したtaskは従来どおりinventoryの対象へSSHで実行します。通常taskと委譲taskは同じ配列に記述でき、元ホスト内の順序、失敗後skip、期限・中断、`--forks` に従います。委譲は選択された元ホストごとに実行するため、複数ホストなら制御端末上でも並行起動します。run_onceではありません。

変数は元ホストのvarsで展開し、結果のhostとtask_indexも元ホストのままです。元ホストのOSがLinuxでも委譲taskにはWindows commandの検査を適用します。全選択ホスト・全taskの入力検査が完了するまで実行しません。

全taskがlocalhost委譲ならinventoryにはホスト名だけを記述できます。SSHのaddress/user/private_key_file/known_hosts_fileは不要で、接続も行いません。通常taskが含まれる場合は従来の接続属性が必要です。SSHは最初の通常taskの直前に接続し、後続通常taskで再利用します。

ローカルrunnerはCreateProcessで子を一時停止状態で作成し、kill-on-close Job Objectに登録してから開始します。登録に失敗した子は実行せず終了させます。stdinはEOF、stdout/stderrは並行して最後まで読み、表示指定時は各1MiBまで保持します。生バイトは既存のbase64出力形式で返し、`--include-output` 省略時は本文を保持・表示しません。ユーザーのexe/argvやOS例外は診断へ出しません。

通常終了時も残った子孫を終了させます。期限切れ・中断時はJob Object内の子孫を終了させ、プロセス終了、pipeの回収、handleの解放が終わるまでforks枠を保持します。ローカルtaskはバックグラウンドプロセスの常駐用途には使えません。

終了0はchanged=true、非0はfailed/changed=nullと終了値、起動失敗はfailed/changed=falseです。起動・実行エラーは既存の `task_failed`、期限は `deadline_exceeded`、親contextの中断は `cancelled` として返し、後続taskを実行しません。権限昇格、任意ホストへの委譲、file_content/fetch委譲は未対応です。
