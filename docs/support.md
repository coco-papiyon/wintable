# 対応範囲

制御端末は Windows 11 / Windows Server 2022以降の amd64 のみ。
Linux制御端末、ARM64、第二Linuxディストリビューションは保証対象外。

| SSH対象 | command | file_content | 実証範囲 |
| --- | --- | --- | --- |
| Windows amd64 | 実装済み | 実装済み | Server Core 2022コンテナ。Windows 11 / フルServer実機は未確認 |
| Debian 13 amd64 / OpenSSH / POSIX互換 `/bin/sh` / CPython 3.13 | 実装済み | 未対応、全接続前に終了2 | 固定Debian imageで個別実SSH。CI対応公開は下記条件付き |
| その他のOS/arch/runtime | 保証対象外 | 保証対象外 | 未確認 |

Linux対象には管理者が **事前に** `/usr/bin/python3` のCPython 3.13系、
OpenSSHと通常の実行ユーザーを用意する。CLIはPythonの自動導入・更新を行わない。
CIはCPython 3.13.5を固定する。Pythonの別実装・別系列は拒否する。
SSH公開鍵認証と事前配備したknown_hostsが必須。秘密鍵・known_hostsパスはWindows制御側、
executableは対象OS側の絶対パス。Linux commandはPATH探索せず、argvをshell展開しない。
sudo/become、package/systemd、再起動、汎用shellは対象外。

通信切断・timeout・cancel後の子孫プロセス停止は保証しない。
changed=nullは対象状態不明を表す。再送は行わず、状態を確認してから再実行する。

混在inventoryは模擬backend/engineと個別Windows/Linux実SSHを分けて検証する。
**混在単一runの実機E2Eは未確認**。両コンテナモードの同時稼働を前提にしない。
L07（Linux file_content）は後続範囲であり、接続前拒否だけを確認する。

対応公開の条件は、同じSHA・同じ製品exeに対する両OSジョブと
`publication-gate` の合格。`candidate` artifactは試験前の候補であり公開版ではない。
Issue #29実装時点ではself-hosted runner未登録のためCIでの実到達・両OS合格は未確認。
過去の個別実証は [Issue #28](validation/issue28/README.md)、
CI実行・導入・復旧手順は [CI](ci.md) を参照。
