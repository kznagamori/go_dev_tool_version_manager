# プラットフォーム・シェル統合仕様

## 1. 共通原則

- 一般ユーザー権限で完結する。
- UAC、`runas`、`sudo`、`pkexec` を自動起動しない。
- system-wide PATH、HKLM、`/etc/profile`、`/usr/local` を変更しない。
- shell startup変更は事前表示、個別確認、backup、marker、冪等、undoを必須とする。
- profileをsourceしない `--no-profile` shellではgdtvmを自動有効化できないことを明示する。

## 2. Windows一般ユーザー

### 2.1 対象filesystem

ポータブルrootはlocal writable NTFSを推奨する。Windows directory junctionはローカルvolume上のdirectoryを対象にし、network shareやmapped network driveでは使わない。FAT/exFATやpolicy制限でjunction/hardlinkが使えない場合はshim-onlyへフォールバックする。

`doctor` は次を実際の一時領域で非破壊probeする。

1. directory作成
2. test target作成
3. directory junction作成とtarget確認
4. file hardlink作成とfile ID確認
5. link自身だけを削除
6. test領域削除

管理者group membershipではなく、現在tokenと実際の操作成功で能力判定する。通常のローカルNTFS directory junctionは標準ユーザーで作成可能な前提を採るが、失敗時のshim-onlyを常備する。

参考となるMicrosoft仕様:

- `https://learn.microsoft.com/ja-jp/windows/win32/fileio/hard-links-and-junctions`
- `https://learn.microsoft.com/en-us/windows-server/administration/windows-commands/mklink`

### 2.2 junction操作

Go側は可能ならWindows APIを直接使う。`cmd.exe /c mklink /J` を使う実装でも、argv quoting、localized output非依存、return code、reparse point検査を必須とする。

- 既存pathが通常directory/fileなら削除・上書きしない。
- 既存junctionが同targetなら成功。
- 異targetならtemporary junctionを作り検査後、旧junction自身を除去してtemporaryをrenameする。Windowsではこの置換に短い欠落区間を許し、stateを正としてshimがexact payloadへfallbackする。
- 除去時にtarget directoryを再帰削除しない。
- targetは同じtool rootのpayloadだけ。

### 2.3 shim

Windows shimはmulti-callのgdtvm PE native console executable `.exe`。`.cmd`や`.ps1` wrapperをcommand shimとして使わないため、cmd、PowerShell 5.1、PowerShell 7から同じquotingとexit codeを得る。

NTFSではsingle shim binaryからhardlinkを作る。hardlink不可かvolumeが異なる場合、`allow_small_shim_copy=true` のときだけ小型shimをcommandごとにcopyできる。大容量tool directory/fileをcopyするfallbackは禁止する。

### 2.4 cmd.exe setup

使用するregistry keyは現在ユーザーだけ:

`HKCU\Software\Microsoft\Command Processor\AutoRun`

既存AutoRunを無条件に上書き・連結しない。`cmd` は `/d` がない場合にAutoRunのコマンド文字列を実行するため、引用符や`&`, `|`, `^`, `%`, `!`を含む既存値へ機械的に文字列追加すると意味が変わり得る。次の方式を採る。

1. 既存値とtypeを `state/setup.toml` とbackup fileへ保存する。
2. gdtvm所有の `state/shell/cmd-init.cmd` を作る。
3. AutoRunが未設定なら、gdtvm initだけを`call`する値を候補にする。
4. 既存値がgdtvm管理済みならmarker/hashを検証して冪等更新する。
5. 既存値が単一のuser-owned `.cmd`/`.bat` fileを`call`する単純形式で、そのfileが書込み可能なら、registry値を変えず対象fileへgdtvm marker blockを追加する案を表示する。
6. 既存値がinline command、複合command、環境変数展開、または解析不能な場合は自動合成しない。AutoRunを保持したまま、利用者が既存scriptへ追加できる1行と別startup方法を提示する。
7. 変更後のfile/registry値を表示して確認してから書く。
8. undo時、現在値またはfile blockがgdtvm設定時hashと一致する場合だけ元値へ復元する。第三者変更があれば自動上書きせずmanual手順を示す。

cmd initはgdtvm実行file directoryとshimsをPATH先頭へ重複なく追加し、`state/shell` に原子的生成されたcmd用環境snapshotをcallしてuser currentに必要な`shell_export`対象を設定する。`shell_export_path=true` のruntime environmentだけをPATHへ反映する。link方式でjunctionが有効ならsnapshot内のtool pathは安定した`current` junction、shim-onlyなら選択payloadの絶対path、backend方式ならshared pathとselectorである。UTF-8が必要な個別commandはshim内でcodepageを処理し、shell全体へ無条件 `chcp 65001` を行わない。

cmd用generated fileはdelayed expansionを有効化せず、`SET "NAME=value"` 形式を基本とし、batch内のliteral `%` を`%%`へencodeする。改行、NUL、環境変数名として不正な値を拒否する。pathに`!`があってもdelayed expansionを有効化しない。

### 2.5 Windows PowerShell 5.1 / PowerShell 7+

各hostの `$PROFILE.CurrentUserAllHosts` または採用したprofile pathを実プロセスに問い合わせる。両者を別々に設定する。

profileへ次のmarker blockだけを追加する。

```text
# >>> gdtvm initialize >>>
<gdtvmが生成したinit fileのdot-source 1行>
# <<< gdtvm initialize <<<
```

block本体はASCII互換path quotingを正しく行い、生成fileはUTF-8。既存encodingを検出し、profile全体を書き換える場合は同encodingを保持する。markerが1件なら更新、複数ならrepair対象。

execution policyを変更しなくても実行できる手段（署名、直接profile内関数、process単位の許可）を優先する。`CurrentUser` policy変更は別警告・別確認で、元値を保存する。企業Group Policyが優先している場合は変更を試みない。

## 3. Linux

### 3.1 current symlink

- `tools/<id>/current` から `versions/<encoded-version>/<encoded-variant>/payload` への相対symlink。
- temporary symlinkを作り、`rename` で置換する。
- symlink不可filesystemではshim direct-targetを使う。
- ownership/permissionは現在userのみ。setuid bitsを許さない。

### 3.2 shell setup

bashは `.bashrc`、zshは `.zshrc`、fishは `$XDG_CONFIG_HOME/fish/conf.d/gdtvm.fish` を既定候補とする。login shell差異があるため、実際のshellとstartup fileを表示して選択させる。

bash/zshのmarker:

```text
# >>> gdtvm initialize >>>
source "<generated-init>"
# <<< gdtvm initialize <<<
```

fishはfish構文のsourceを生成する。POSIX shell構文を混在させない。

generated initの責務はgdtvm binary/shimsをPATHへ追加し、`state/shell` のbash/zsh用またはfish用環境snapshotをsourceしてuser currentの`shell_export`対象と、`shell_export_path=true` のPATHを設定することだけ。snapshotは選択変更時に原子的再生成される。link方式でsymlinkが有効なら安定した`current`、fallback/backendではabsolute shared/selectorを使う。network、registry update、catalog refreshをshell起動時に行わない。

bash/zshの値はPOSIX single-quote規則でquoteし、値中のsingle quoteを安全な連結表現へ変換する。fishはfish固有のescapeを用いる。文字列を未quoteでshell sourceへ出さない。

### 3.3 system prerequisite

glibc、libstdc++、JDK、unzip等がOS packageとして必要でもgdtvmはpackage managerを実行しない。definitionのprobeとinstall hintを表示する。Linux client自身はCGOなしのためgdtvm起動にlibcを要求しない。

### 3.4 libc判定

platform resolverはLinux tool artifact選択のため`glibc`, `musl`, `unknown`を判定する。`GOOS/GOARCH`だけから推測せず、次を安全なread-only順で使う。

1. `/bin/sh`, `/bin/ls`等の実在ELFで`PT_INTERP`を読み、basenameが`ld-musl-*.so.1`ならmusl、`ld-linux*.so*`/`ld64.so*`ならglibc。
2. ELFが静的または利用不能なら、標準loader directoryにあるmusl/glibc dynamic loaderの実在とELF形式を確認する。
3. 両方の根拠がある、または根拠がなければunknownとする。任意の`ldd` output文字列や現在PATHだけを決定根拠にしない。

unknown時はdefinitionの`libc="any"`候補だけを選択でき、glibc/musl専用artifactを試行実行して推測しない。`doctor`は根拠pathと判定を表示する。

## 4. PATH順位

推奨順位:

1. gdtvm command shims
2. gdtvm executable directory
3. 利用者の既存PATH

toolのcurrent bin pathを多数shell PATHへ直接追加すると選択解除・project選択が不安定になるため、公開commandはshimを第一経路とする。ただしtoolがshim未公開の補助実体を内部起動する場合、shimが子環境PATHへpayload binを追加する。

setupは同一canonical pathの重複を作らない。既存PATHに別version managerのshimが先行する場合、doctor warningと実際に解決されたcommand pathを表示し、勝手に削除しない。

## 5. VS Code

標準利用モデルは、初期化済みcmdまたはPowerShellでプロジェクトへ移動し、`code.exe .` を起動することである。

- VS Code processはgdtvm shimsを含むPATHとuser current環境を継承する。
- extensionsとintegrated terminalsもその環境を継承する。
- extensionがPATHからlanguage server/compilerを起動し、workspaceをcwdにする場合、shimが `.gdtvm.toml` を解決する。
- user currentのjunction pathは安定しているため、extension設定へ固定pathを指定する必要がある場合に使える。
- project固有の `JAVA_HOME` 等をVS Code親processにも適用したい場合は `gdtvm exec -- code.exe .` を用いる。
- 選択変更後、既に起動中のlanguage serverは古い実体を保持しうるため、terminal再起動またはDeveloper: Reload Windowを案内する。

`code.exe` をgdtvmのcommand shimとして奪わない。

## 6. setup backup/undo

変更対象ごとに次を保存する。

- canonical pathまたはregistry key
- 変更前の存在、type、encoding、content/hash
- gdtvmが書いたcontent/hash
- timestamp、client version、operation ID

backupは`state/setup-backups/<timestamp>/`へ置き、owner-only permission。undoはcurrent contentがgdtvm書込み後hashと一致する場合だけ自動復元する。不一致はthree-way情報を示し、非gdtvm内容を失わせない。

## 7. uninstall時の注意

利用者が管理rootを手動削除する前に `gdtvm setup --remove` を実行する。ポータブルfolder削除だけではprofile/AutoRun markerを除去できない。`doctor` は移動前rootを指すdead markerを検出し、現在rootから安全にremove/repairできる。

## 8. platformエラー

| 状況 | 挙動 |
|---|---|
| Windows junction不可 | warning、shim-only、正常継続 |
| hardlink不可 | small shim copy許可ならcopy、なければerror |
| Linux symlink不可 | shim direct-target、warning |
| profile書込み不可 | manual行を表示、tool管理は継続可 |
| HKCU policyでAutoRun禁止 | PowerShellまたはmanual setupを案内 |
| network root | 既定拒否、user mode/local rootを案内 |
| path長制限 | short homeを案内。途中でtruncateしない |
| antivirus lock | 有限retry後、path/processを示して失敗 |
