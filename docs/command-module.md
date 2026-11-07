# commandモジュール（I-07）

commandはCRT規則でargvを受け取るWindows絶対パスの.exeに限定します。cmd.exe、バッチ、.com/.ps1、独自引数解析exe、シェル構文は対応しません。コマンドは非冪等です。終了0はchanged=true、非0はfailed/changed=nullで子の終了値を返します。通信断・期限切れは変更不明で、自動再送しません。

## 固定コードとプロトコル

`internal/module/command.cs`を埋め込み、gzip/base64で固定PowerShellブートストラップに格納し、全体をUTF-16LE/base64のEncodedCommandへ変換します。圧縮は配布コードの符号化で、タスクごとのコード生成・分割送信は行いません。ソースはサイズ制限のためコンパクトに保っています。Add-TypeはPowerShell 5.1に付属する.NET FrameworkのC#コンパイラを使用します。実行時コンパイルが禁止される対象環境は未対応です。コンパイラの一時ファイルにもタスク値は入りません。

タスク値はPreparedが一度だけ生成したBOMなしUTF-8 JSONをstdinへ送信し、EOF後に実行します。受信側は最大2,097,153バイトまで読み、2MiB超過、不正UTF-8/BOM、末尾データ、重複キー（エスケープ後も照合）、未知キー、型違い、不正サロゲートを実行前に拒否します。受信パーサーはこのスキーマに必要な文字列・文字列配列・オブジェクトだけを扱い、深さも制限します。診断は定型で本文や解析例外を返しません。

ProcessStartInfoはUseShellExecute=false、FileNameとCRT符号化Argumentsで起動します。子stdinは即座に閉じます。最終行は.NET Frameworkと同じくFileNameを常に引用符で囲み、非空の引数列の前に区切りを加えます。Go側・受信側で終端NULを除き32,766 UTF-16単位以下を検査します。非BMP文字は2単位です。

stdout/stderrは別々のバイトストリームで並行回収し、各1MiBを保持した後も破棄しながら読み続けます。応答はBOMなしUTF-8の単一JSONで、成功した起動ではexit_code、stdout/stderr（base64）、stdout_truncated/stderr_truncatedを必須とします。拒否・起動失敗はerrorだけです。外側PowerShellの終了値と子の終了値は区別し、不正応答や途中中断では子終了値をnullとします。既定の結果に出力本文を含めません。

両出力のbase64は合計2,796,208バイトになり得るため、Prepared.Requestはtransportにstdout最大3MiBの応答枠を指定します。通常transportの各1MiB制限は維持し、この枠の最大値も3MiBに固定します。応答超過・外側stderr・外側非0・不正スキーマ・重複キー・不正base64は定型failed/changed=nullへ変換します。

## 起動行とビルド検査

対象構成はWindowsディレクトリがC:\Windows、OpenSSHのDefaultShellがC:\Windows\System32\cmd.exe、既定の/c、PTYなしです。他のシェル、カスタムDefaultShellCommandOption・shell引数は対象外です。SSH execへ送るのは`powershell.exe -NoLogo -NoProfile -NonInteractive -EncodedCommand <固定base64>`です。

計測対象は`"C:\Windows\System32\cmd.exe" /c "<SSH execの固定行>"`全体です。[OpenSSHの実装](https://github.com/PowerShell/openssh-portable/blob/latestw_all/contrib/win32/win32compat/w32-doexec.c)のshellの引用符と非PTY/cmd分岐の構築を基準にしています（w32-doexec.cのdo_exec_windows）。GoのWrapperLaunchLineが同じ式を組み立て、UTF16Lenで測ります。今回のコードは7,401単位です。

`build.ps1`はTestDistributionWrapperを必須実行してからビルドし、実行時もcli.RunEngineが最初の接続前に同じ最終行を検査します。上限8,000超過は配布物不備として終了3です。合成7,999/8,000/8,001単位の境界もテストします。Server Core 2022の実OpenSSH経由で起動中のcmd.exeをCIMから取得し、実行ファイルパスの大文字小文字以外は構築式と完全一致し、7,401単位であることを確認しました。採取値はwrapper-launch.jsonに保存します。

## 検証と未確認事項

`go test ./...`、`go vet ./...`、`build.ps1`を使用します。WindowsではTestCommandWindowsが実cmd.exe・PowerShell 5.1を起動し、CRTReceiver.exeのmsvcrt.dll `__wgetmainargs`が実コマンドラインを解析します。CLR/Goのargv解析で代用しません。試験専用exeには、引数をUTF-16LE/base64行で返すモード、生バイト同時大量出力、終了値、実行検出マーカーがあります。

Windows 11ローカルで確認した項目：日本語・空白パス、空引数、引用符、バックスラッシュ、非BMP、メタ文字・改行、9,000文字argvの往復、生出力各1MiBとtruncated、終了値、子起動行32,765/32,766/32,767、stdin 2MiB−1/同値/+1、7バイト分割送信、不正UTF-8/JSON/重複/型/未知キー/BOM/サロゲートの拒否、本文非露出、EOF前の実行禁止。モジュール・engineの回帰試験では外側/子終了値の区別と既定出力非公開も検証します。

## 2026-11-08の修正とSSH検証

実OpenSSH 9.5.6.4経由では、従来の8,192バイト読取で9,000文字argvの受信が停止しました。4,096バイト読取でも再現しました。受信を既存transport fixtureと同じ7バイト単位に変更すると、長いargvと2MiB−1/同値/+1を含む製品ラッパーの全試験が成功しました。原因の内部機序は未確定であり、待機時間や追加の標準出力・標準エラーで回避していません。小さい断片をEOFまたは上限+1までバイト列として結合し、その後に厳密UTF-8/JSON検査する契約は同じです。

`tests/windows-ssh/run.ps1`は製品ラッパー試験に加え、`TestCommandSSHLaunchAndDisconnect`を実行します。CIMで実cmd起動行を採取し、7バイト単位のSSH送信（日本語パスのUTF-8途中分割）、明示EOF、別の未完JSON要求の送信中切断、切断前後の副作用なしを確認します。専用コンテナ以外を観察する機能はありません。`TestCommandWindows`には末尾空白で実起動行を7,999/8,000単位にした起動試験と、8,001の接続前拒否を追加しました。

今回の製品試験証跡: `.cache/windows-ssh/4726fe503440/`。`command-test.log`は実cmdとSSH両モードで全サブテスト成功、`wrapper-launch.json`は実起動行、`manifest.json`はソースhash・OpenSSH版・コマンド・終了値・後片付けを保存します。ベースはServer Core LTSC 2022のdigest `sha256:b52799a67fa596d4bddcd82ae1dab0077a84767fbc33c7342b72b53ecc71ad09`、Hyper-V分離です。ホストのSSHサービス・設定は変更しません。

Windows 11のローカルcmd/PowerShell/CRT試験は成功しましたが、Windows 11をSSH対象とする試験、およびフルServer 2022での試験は未確認です。Server Coreコンテナを両OSの実機合格へ読み替えません。Issue #9の受け入れ条件全体は、これらが未確認のため合格扱いにしません。独自引数解析exe等の互換範囲は本書冒頭のとおりです。