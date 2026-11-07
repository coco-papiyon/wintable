# Windows OpenSSHコンテナ試験

Issue #8のtransportを、Go模擬サーバーではなくWindows Server Core LTSC 2022内の
Win32-OpenSSH・cmd.exe・PowerShell 5.1に接続して検証するためのfixtureです。
製品のcommand/file_contentラッパーは使用しません。

## 前提と実行

- WindowsコンテナモードのDocker、PowerShell **7.3以上**、プロジェクト指定Go。
- `docker run --rm --isolation=hyperv mcr.microsoft.com/windows/servercore:ltsc2022 cmd /c ver`
  が成功すること。Windowsホストとイメージの互換性に応じてprocess分離も利用できます。
- 既定ではホストの`C:\Windows\System32\OpenSSH`の実行ファイルをイメージへコピーします。
  ZIP導入版の場合は公式配布を展開したディレクトリを`-OpenSSHDirectory`で指定してください。
  ホストのサービス・レジストリ・鍵・ファイアウォールは変更しません。
- 初回はServer Coreイメージの取得が必要です。今回の環境には既にありました。

リポジトリルートのPowerShell 7で実行します（Dockerへアクセスできる権限が必要）。

```powershell
.\tests\windows-ssh\run.ps1
# ホストとイメージがprocess分離に対応している場合
.\tests\windows-ssh\run.ps1 -Isolation process
# 取得済みの同じイメージ・OpenSSHバイナリで再現する場合
.\tests\windows-ssh\run.ps1 -Isolation process `
  -BaseImage 'mcr.microsoft.com/windows/servercore@sha256:<記録済みdigest>' `
  -OpenSSHDirectory C:\tools\OpenSSH-Win64
```

出力は毎回新しい`.cache/windows-ssh/<実行ID>/`へ保存します。`-OutputDirectory`は
存在しないパスだけを受け付けます。`manifest.json`にソースSHA／変更状態、fixtureの
SHA-256、全OpenSSHファイルの版とSHA-256、Docker・Go版、ベースと完成イメージID、
分離方式、実行コマンド、終了値、削除結果を記録します。`test.log`に対象OS・PowerShell・
既定シェル、各ケースの結果を残します。版の違うOpenSSHで実行した結果を同一視しないでください。

秘密鍵は実行ごとに生成し、ビルドコンテキストには入れず、finallyで削除します。
公開鍵だけを`docker cp`でコンテナへ入れます。ホスト鍵も認証されたDocker管理経路の
`docker cp`で取得してknown_hostsへ固定し、ssh-keyscanの自動信頼は使いません。
ポート公開・ホストディレクトリのマウントは行わず、ホストからコンテナNATアドレスへ接続します。
コンテナ専用のContainerAdministratorを使うため、既存の対象機には流用しないでください。

## 検証内容

| ケース | 観測・期待値 |
| --- | --- |
| V-05 未知鍵・変更鍵・誤認証 | 固定公開エラー、セッションなし、known_hosts不変、receiver実行0回 |
| V-11/18 stdin・EOF | 有効な約2MiBの日本語UTF-8 JSON、7バイト読取、受信長とSHA-256一致、EOF記録 |
| V-11 両出力 | stdout/stderrを並行出力、それぞれ1MiB超、保持バイト・truncated・終了値7一致 |
| 接続再利用 | Connectのcontext終了後も同一接続でversion＋transferを2回実行 |
| V-06/18 送信停止 | 受信しない相手へSSHウィンドウ超の16MiBを送信、deadline_exceeded、12秒内return |
| V-06 EOF後待機 | 入力EOF後に終了しない相手をdeadlineで中断 |
| V-06 キャンセル・Close | 送信中のcancel、並行・重複Closeで解放、取得不能exitはnil |
| V-06/18 通信断 | 送信中にコンテナ内sshdを停止、connection_lost、12秒内return |
| 再送なし | receiver実行履歴が期待する8件と完全一致、中断した接続で再実行不可 |

16MiBはtransport単体のフロー制御試験であり、製品の2MiB上限を緩めるものではありません。
UTF-8の分割は受信側7バイト読取で確認します。TCPパケットの分割位置は固定しません。
出力上限ちょうど／前後、handshake停止、exec応答停止、秘密非表示・結果分類は既存単体試験も併用します。
子プロセス停止はSSH切断の保証に含まれません。中断後の残存プロセスは最後のコンテナ削除で破棄します。

## 終了・後片付け

成功はスクリプト終了0、失敗は例外による非0です。通常の失敗でもfinallyでログ取得、
専用コンテナと専用タグ、試験用秘密鍵の削除を実施します。ベースイメージは保持します。
強制終了・ホスト再起動でfinallyが走らなかった場合は、該当実行IDだけを確認して削除します。

```powershell
docker ps -a --filter 'name=wintable-ssh-'
docker rm -f wintable-ssh-<実行ID>
docker image rm wintable-ssh-fixture:<実行ID>
# 該当実行ディレクトリに残ったclient秘密鍵だけを削除
Remove-Item -LiteralPath .cache/windows-ssh/<実行ID>/client
```

## 確認範囲と2026-11-07の実行結果

Docker 29.9.2、ホストWindows build 26300.9818、既存Server Core LTSC 2022イメージ
`sha256:652b2805d507...`で、Hyper-V／process分離ともビルドStep 3で次のエラーになりました。

```text
hcsshim::ExpandScratchSize failed in Win32: The system cannot find the file specified. (0x2)
```

SSHを使わない最小の`docker run --rm --isolation=process ... cmd.exe /c ver`でも、
Server Coreと既存winrmイメージの両方で同じエラーを確認しました。
SSH試験以前のDockerストレージ／コンテナ作成段階の失敗です。ホスト側Docker設定や
既存イメージを削除して修復する操作はしていません。**コンテナSSH試験は未実行・未合格**です。
ビルド失敗のログとmanifestは`.cache/windows-ssh/046b345aeb94`（hyperv）、
`83cc53bdee56`（process）へ保存されています。これらはローカル証跡でGitには含めません。

このfixtureの成功後も、Server Coreコンテナの結果をWindows 11対象の合格にはしません。
Windows 11ホスト上で動かしても対象ユーザーランドはServer Coreです。
フルWindows 11／Server 2022の受け入れと、#9/#10の製品ラッパーの受信不正拒否・副作用なしは別途必要です。

参考: [Windowsコンテナの互換性](https://learn.microsoft.com/en-us/virtualization/windowscontainers/deploy-containers/version-compatibility)、
[公式Win32-OpenSSH導入](https://github.com/PowerShell/Win32-OpenSSH/wiki/Install-Win32-OpenSSH)。
