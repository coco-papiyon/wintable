# wintable

Windowsネイティブで動作するGo製の構成管理CLIを設計しています。現在は設計段階で、実行可能なCLIやAnsible互換機能は未実装です。

- [Windows版Ansible相当ツールの設計](docs/design/windows-ansible.md)
- [検証計画と後続実装](docs/design/windows-ansible-validation.md)

Issue #1 の成果物は上記の設計資料です。初期実装案はWindows制御ノードからWindows管理対象へのSSH接続と、独自JSON形式のタスク定義です。既存Ansible Playbookの直接実行は対象外です。
