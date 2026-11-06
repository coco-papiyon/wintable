# GitHub MCPサーバー構成

このリポジトリのCodexは `.codex/config.toml` からGitHub公式MCPサーバーをSTDIOで起動する。起動スクリプトはGitHub CLIの既存認証をプロセス内だけで利用し、トークンを設定ファイルやリポジトリへ保存しない。

## 構成

- 公式サーバー：`github/github-mcp-server`、バージョン `v2.1.0`
- Windows x86_64版を `.codex/tools/github-mcp-server/` に配置し、公式のSHA256チェックサムを照合する。バイナリはGit管理対象外。
- 必須コマンド：PowerShell 7の `pwsh`、認証済みの `gh`
- 有効なtoolset：`context,repos,issues,pull_requests,labels,users`
- 読み取り専用にはせず、スキルが必要とする本文更新、コメント、ラベル、PRレビュー・マージ等を提供する。操作権限は認証アカウントとリポジトリの保護ルールに従う。
- 設定の起動スクリプトパスは現在の作業ディレクトリに依存しない絶対パス。別の場所へ移動した場合は `.codex/config.toml` のパスを変更する。

## 別環境へのセットアップ

1. `gh auth login --hostname github.com` でログインする。
2. 公式リリース `v2.1.0` の `github-mcp-server_Windows_x86_64.zip` と `github-mcp-server_2.1.0_checksums.txt` を取得し、ZIPのSHA256を照合してから上記配置先へ展開する。
3. 設定の絶対パスを調整し、このリポジトリを信頼済みプロジェクトとして開く。
4. CodexのMCP設定で再起動するか、新しいセッションを開始し、`/mcp` で `github` の接続を確認する。

既存セッションには設定が即時反映されない場合がある。`codex mcp list` は登録確認であり、接続確認にはMCPツールの読み取り呼び出しも行う。

## 参照

- [Codex MCP設定（OpenAI公式）](https://developers.openai.com/codex/mcp/)
- [GitHub公式Codex導入手順](https://github.com/github/github-mcp-server/blob/main/docs/installation-guides/install-codex.md)
- [GitHub MCPサーバー v2.1.0](https://github.com/github/github-mcp-server/releases/tag/v2.1.0)
