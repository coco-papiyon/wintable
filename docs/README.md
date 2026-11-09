# ドキュメント索引

この索引は現行仕様・利用手順と履歴資料の入口です。実装状況のまとめは[モジュール一覧](design/modules.md)、OS/runtimeの保証範囲は[対応表](support.md)を参照してください。

## 利用・現行仕様

- [Windows導入・利用](usage.md)
- [モジュール一覧とWindows/Linux実装状況](design/modules.md)
- [Windows command](design/modules/command-module.md)
- [Windows file_content](design/modules/file-content-module.md)
- [Linux command](linux-command.md)
- [Linux file_content](linux-file-content.md)
- [SSH transport](ssh-transport.md)
- [対応範囲](support.md)

## 開発・検証・配布

- [試験一覧と変更に応じた試験方針](testing.md)
- [CIと実SSH受入](ci.md)
- [配布・公開条件](release.md)
- [Windowsコンテナ検証](validation/windows-container.md)
- [Windows版初期設計](design/windows-ansible.md)
- [初期検証計画](design/windows-ansible-validation.md)
- [Linux対象追加の設計判断（ADR-0001）](adr/0001-linux-target-support.md)

初期設計・ADR・検証計画はその作成時点の判断や試験を保存する履歴資料です。現在の対応状態や公開条件と記述が異なる場合は、現行仕様、[対応表](support.md)、[CI](ci.md)を優先してください。`docs/validation/`以下は各commitに結び付く記録で、現在のHEADに対する検証結果とは限りません。

## GitHubワークフロー

- [GitHub CLIの導入](github-cli-setup.md)
- [GitHub MCPの導入](github-mcp-setup.md)
- [GitHub workflow skills仕様](github-workflow-skills-spec.md)
