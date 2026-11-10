# 対応範囲

Windows対象の `powershell`（PowerShell 5.1）と Linux対象の `shell`（/bin/sh）を実装。OS別の選択が必須で混在対象への自動切替はない。[制約と検証範囲](script.md)を参照。

制御端末は Windows 11 / Windows Server 2022以降の amd64 のみ。
Linux制御端末、ARM64、第二Linuxディストリビューションは保証対象外。

| SSH対象 | command | file_content | fetch | 実証範囲 |
| --- | --- | --- | --- | --- |
| Windows amd64 | 実装済み | 実装済み | 実装（NTFS制御側） | Server Core 2022コンテナ。Windows 11 / フルServer実機は未確認 |
| Debian 13 amd64 / OpenSSH / POSIX互換 `/bin/sh` / CPython 3.13 | 実装済み | 実装済み（祖先・属性制限あり） | 実装（NTFS制御側） | 固定Debian imageで個別実SSH。CI対応公開は下記条件付き |
| その他のOS/arch/runtime | 保証対象外 | 保証対象外 | 保証対象外 | 未確認 |

Linux対象には管理者が **事前に** `/usr/bin/python3` のCPython 3.13系、
OpenSSHと通常の実行ユーザーを用意する。CLIはPythonの自動導入・更新を行わない。
CIはCPython 3.13.5を固定する。Pythonの別実装・別系列は拒否する。
SSH公開鍵認証と事前配備したknown_hostsが必須。秘密鍵・known_hostsパスはWindows制御側、
executableは対象OS側の絶対パス。Linux commandはPATH探索せず、argvをshell展開しない。
sudo/become、package/systemd、再起動、任意のシェル選択は対象外。

通信切断・timeout・cancel後の子孫プロセス停止は保証しない。
changed=nullは対象状態不明を表す。再送は行わず、状態を確認してから再実行する。

混在inventoryは模擬backend/engineと個別Windows/Linux実SSHを分けて検証する。
**混在単一runの実機E2Eは未確認**。両コンテナモードの同時稼働を前提にしない。
モジュール別の現行実装と設計は[一覧](design/modules.md)を参照。L07の設計・制約・検証層は [Linux file_content](linux-file-content.md) を参照。実装済みとCI対応公開は区別する。

対応公開の条件は、同じSHA・同じ製品exeの両OS必須試験と集約ゲートの合格。
標準GitHub Windows runnerの実測を先に行い、Linux実SSHは既存Windows PCで必要時に実行する。
標準Windows containerが利用不能なら、その能力確認証跡を条件にWindowsも既存PCで実行する。
実行場所はpublication.json/execution.jsonへ明記し、ローカル試験をCI実証とは扱わない。
常設self-hosted runnerや追加VM契約は不要。candidate artifactだけでは公開しない。
過去の個別実証は [Issue #28](validation/issue28/README.md)、
実行・導入・復旧手順は [CI](ci.md) を参照。

L07の公開条件には、属性保持/拒否・0600・ACL/xattr拒否・同期競合/障害と
配布サンプルの新規→同値→差分を含める。詳細は
[Linux file_contentの保証境界と手順](linux-file-content.md)を参照。
属性/競合試験のSKIPや準備失敗は対応確認に数えない。

fetchは最大1GiB、Windows制御端末のNTFS保存限定です。第三者による検査後の保存先差し替えは上書きを許容します。確定前の失敗はchanged=false、確定後の失敗は既知のtrueを保持します。パス・リンク・ACL・依存条件は[fetch](fetch-module.md)を参照してください。

## ローカルcommand委譲

`delegate_to: "localhost"` を指定したcommandはWindows制御端末上で実行します。`become` は省略または `false` のみで昇格しません。元ホストごとに変数展開・実行し、結果も元ホストに所属します。全taskが委譲ならSSH接続属性・鍵・known_hostsは不要です。絶対exeとargv、出力上限、期限・中断時の子孫清掃などは[ローカルcommandの仕様](local-command.md)を参照してください。

## fileの対応範囲

| 経路 | 実装 | 通常確認 |
| --- | --- | --- |
| Windows SSH | PowerShell/C# receiver | 実SSHは配布時に確認 |
| Linux SSH | Python receiver | Debian fixtureのreceiverと製品CLI |
| Windows localhost | 共通receiverとJob Object | Windows実プロセスとCLI |

stateはdirectoryのみです。パス・リンク・競合時の保証範囲は[file仕様](design/modules/file-module.md)を参照してください。
