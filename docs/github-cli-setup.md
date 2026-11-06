# GitHub CLI利用手順

全6スキルのGitHub操作は `gh` と `gh api` に統一する。MCPは前提としない。

## 導入と認証

1. GitHub CLIをインストールし、`gh --version` を確認する。
2. `gh auth login --hostname github.com` でログインする。
3. `gh auth status` と `gh api user --jq .login` で認証を確認する。トークンの値を表示・保存する必要はない。
4. `git remote -v` と `gh repo view coco-papiyon/wintable` で操作先を照合する。別リポジトリでは実際の対象名を使う。

## 操作方法

- Issue：`gh issue view/create/edit/comment/close`
- PR：`gh pr view/create/edit/comment/merge`
- ラベル：`gh label` と対象Issue／PRの編集コマンド
- 高水準コマンドで不足する操作：`gh api`。一覧はページネーションを処理し、更新はHTTPメソッドと入力JSONを明示する。
- 複数行の投稿本文：UTF-8ファイルと `--body-file` を使用する。
- 正式な `APPROVE`・`REQUEST_CHANGES` レビューは提出せず、通常のPRコメントに結果を投稿する。

## サンドボックスでのアクセス拒否

`GitHub CLI/config.yml` の読み取り拒否、`.git/FETCH_HEAD` の書き込み拒否、コマンド起動失敗の場合は、Agentが利用できる承認付き実行経路で必要なgh・git操作を実行する。操作自体が禁止された場合は迂回しない。

承認付き実行はWindowsの管理者実行とは異なる。ACLを広範囲に緩めたり、トークンをコピーしたりして解決しない。認証・権限不足が続く場合は実際のエラーと未完了操作を報告する。

## 参照

- [GitHub CLI公式マニュアル](https://cli.github.com/manual/)
- [gh api公式マニュアル](https://cli.github.com/manual/gh_api)
