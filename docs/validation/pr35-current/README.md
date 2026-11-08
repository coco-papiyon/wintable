# PR #35 最新製品の同一ZIP再検証

配布commit: `7af2f1b03961c616fa7b7b09c16539969409c76b`。
ZIP SHA-256: `2AD4A10D4685913CD41E9B81E423127409D328BD9544A9D271BD2E85A7BDF4E5`。
exe SHA-256: `F2125E6CA963DF29882445B76F172417ECF41054A1C892FBF58E2531027D45E4`。

main取り込み後のbackend選択を含む最新製品から候補を生成し、旧31b3ae7の証跡を流用せず全V-01〜18を再実行した。Linux command文書もZIPへ同梱し、Windows利用手順の対応範囲を修正した。

## 実行と結果

Go 1.27.3で `scripts/distribution.ps1 -OutputDirectory dist/pr35-candidate` を実行し、mod verify、全test、vet、固定ラッパー長、govulncheck v1.9.0とfinding拒否ゲート、CGOなしWindows amd64 build、ライセンス収集、ZIP内容・ハッシュ検査が成功した。audit.jsonにfindingなし。

生成したZIPを `WINTABLE_DISTRIBUTION_ZIP` に指定し、`tests/validation/run.ps1 -OutputDirectory dist/pr35-acceptance` を実行、終了0。final/manifest.jsonのacceptance=passed、prerequisites_passed=true、全18シナリオpassed。実SSHの認証、転送・EOF、境界、ACL・リンク差し替え、通信障害、Ctrl+C、出力障害も再実行した。

固定Server Core 2022 digestとHyper-V隔離はfinal/manifest.jsonに記録。V-13はContainerUser、Go/WSL/Pythonなし、日本語・空白入り展開先、同一ZIPのhelp・SSH command/file_content・再実行changed=false・JSON結果・終了0を確認。final/ssh/controller.jsonとzip-controller.logにZIP/commit/exeの対応を記録した。試験用コンテナ・イメージの削除も成功。

`scripts/verify-acceptance.ps1` に同一ZIPとacceptance.jsonを指定して成功。保存対象は生ログ・集約表・スクリプト・監査・ビルド情報のみで、秘密鍵・exe・ZIP・Docker contextは含めない。sha256.csvで保存ファイルの生バイトを照合できる。

ZIPはローカル `dist/pr35-candidate/` に保持し、実機合格後は再ビルド・再梱包していない。この配布commit以降は文書と証跡だけを変更する。Windows 11等の別環境やLinuxの実機合格はこの証跡の対象外。公開前の人手レビューとリリース公開は未実施。
