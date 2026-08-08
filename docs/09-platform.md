# プラットフォーム・シェル統合仕様

## 1. 共通原則

対応は`windows-amd64`と`linux-amd64-glibc`。Windows標準user、Linux非rootで完結し、自動昇格、UAC、`sudo`、HKLM、system環境変数、system package managerを使わない。arm64、Linux musl等は[14-maintenance.md](14-maintenance.md)の追加手順を経るまで非対応とする。

platform adapterはOS/arch/user home、filesystem identity、link、permission、process、environment、shellを実装し、tool IDによる分岐を持たない。tool固有情報はdefinition/receiptを正とする。

PATH integration方式はOSごとに1つだけとする。

| OS | 提供する方式 | 推奨既定 |
|---|---|---|
| Windows | `user-path` / `none` | `user-path` |
| Linux | `shell-profile` / `none` | `shell-profile` |

Windowsで`shell-profile`、Linuxで`user-path`を指定した場合は`E_USAGE`とし、その環境で提供する方式を案内する。

## 2. root決定

### 2.1 Windows

user modeのdata rootはKnown Folder APIで取得した`LocalAppData`直下の`gdtvm`。環境変数`LOCALAPPDATA`の文字列だけを信頼しない。portableは実行中`gdtvm.exe`のcanonical directoryをdistribution/data rootとする。

### 2.2 Linux

user modeのdata rootは実UIDをOS user lookupして得たhome直下`.local/share/gdtvm`。`HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`で暗黙置換しない。明示`paths.user_data_root`だけは[05-configuration.md](05-configuration.md)の検証後に許す。

bootstrap scriptは`id -u`と`getent passwd <uid>`でhomeを求め、値が一意でない、空、absoluteでない、owner不一致、`getent`が利用不能なら変更せず停止し、手動archiveまたは明示install先を案内する。runtime clientはGoのOS user lookup portを使う。

### 2.3 共通安全検査

filesystem root、network share、他user所有、world-writable parent、symlink/reparse loop、現在userが作成・renameできないrootを拒否する。data rootとdistribution rootのidentityをstateへ保存し、別rootのstate/linkを混在させない。

portable rootのdirectoryが移動された場合、保存済みidentityと現在pathが一致しないため`doctor`が検出する。v0.1は自動再生成を行わず、`gdtvm setup`の再実行を案内する。

## 3. Windows filesystem

### 3.1 対象

NTFSを正式対象とする。ReFS/FAT/network/removable等でjunction、hardlink、atomic replace、file identity、ACLの必須能力が欠ける場合はsetup probeで理由付き拒否する。portable rootも同じ能力を検査する。

path比較はvolume規則に加えWindows予約名、末尾dot/space、ADS、Unicode case-foldを検査する。long path利用可否をAPIで扱い、`MAX_PATH`へ暗黙truncateしない。ACLは現在user所有かつ他user書込み不可を基本とし、継承結果を検査する。

### 3.2 current junction

managed user selectionの`tools/<tool>/current`はdirectory junctionとする。

1. targetが同じdata rootのhealthy payloadであることをcanonical pathで検査する。
2. temporary junctionを作成してtargetを再読する。
3. selection stateをcommitする。
4. 旧junctionだけをunlinkしてtemporary名をcurrentへrenameする。

junction targetを再帰削除しない。currentがregular directory、symlink、別root reparse pointなら自動置換せず`doctor`診断とする。置換中の短い欠落は許容し、shimはreceiptのdirect targetを使えるようにする。

### 3.3 shim

公開commandは`shims/<command>.exe`。同一volumeで安全にrootを導出できるときはclientへのhardlinkを優先し、それ以外はclient releaseへ内蔵した小型native resolverを展開する。resolverのSHA-256/version/ownerを検査する。tool executableをcopyしない。unknown basenameをCLIとして実行しない。

## 4. Windows PATH integration

### 4.1 選択肢

対話`setup`は次を順に表示する。

| 値 | 位置づけ | 動作 |
|---|---|---|
| `user-path` | 推奨・既定 | HKCUのuser PATHへshim directoryを1 entry追加 |
| `none` | 代替 | 永続変更なし。利用者が明示pathまたはabsolute commandを使う |

`user-path`ならcmd、Windows PowerShell、PowerShell 7、GUI appから同じshimを利用できる。既に起動中のprocess環境は変わらないため、新しいterminal/GUI processが必要である。

`--non-interactive`では`--path-integration`必須。`user-path`失敗時に自動fallbackしない。

Windows向けのPowerShell profile方式はv0.1で提供しない（[15-deferred.md](15-deferred.md) D-07）。`user-path`が使えない環境では`none`を選び、shim directoryを利用者自身のprofileやlauncherから参照する。

### 4.2 user PATH書込み

Win32 Registry APIで`HKCU\Environment`の`Path`値だけを扱い、`setx`、`reg.exe`、PowerShell childを使わない。

1. 値の存在、raw UTF-16 text、`REG_SZ|REG_EXPAND_SZ`型、registry key identityを読む。他型は拒否する。
2. NUL、不正展開、serialized length、environment block上限を検査する。
3. semicolon単位で比較用だけ正規化し、shim pathの同値entryを検出する。空entryと他entryのraw表現・順序・環境変数参照を保持する。
4. 追加するshim absolute path 1件を先頭へ置くPlanとbefore digestを表示する。既存なら変更しない。
5. raw値と型を`state/setup.toml`/owner-only backupへ保存してから同じ型で書く。値が存在しなければ`REG_EXPAND_SZ`を作る。
6. key/valueを再読し、期待値・型・長さを照合する。不一致なら保存値へrollbackする。
7. `WM_SETTINGCHANGE`の`Environment`通知をtimeout付きで送る。通知失敗は書込み成功＋`W_ENVIRONMENT_NOTIFICATION_FAILED`とする。

追加後のPATHは32,767 UTF-16 code unit未満かつ[04-storage-and-data.md](04-storage-and-data.md)§21のより小さい安全上限内でなければ変更しない。registryへgdtvm専用valueやversionごとのentryを増やさないため、registry肥大化はshim path 1 entryに限定される。

`setup --remove`は現在のgdtvm rootとsetup stateが所有を証明する同値entryだけを除去する。利用者が他entryを変更済みでも全Pathを古いbackupへ巻き戻さない。競合・duplicate・型変化は変更せず診断する。

## 5. Linux integration

### 5.1 currentとshim

`tools/<tool>/current`は同じdata root内payloadへのrelative symlinkとする。absolute target、root外、既存regular directoryを拒否する。temporary relative symlinkを作ってtargetを再検査し、renameで置換する。

公開command shimも`shims/`からclientへのrelative symlinkを使う。rootを安全に決定できないfilesystemでは同release内蔵native resolverを利用できるが、tool本体をcopyしない。

### 5.2 shell profile

Linux setupは`shell-profile|none`を表示し、対話既定を`shell-profile`とする。現在shellだけを対象とし、判定不能ならbash/zsh/fishの1件を選ばせる。検出した全profileを一括変更しない。対象shellがbash/zsh/fishのいずれでもない場合は`E_UNSUPPORTED_SHELL`とし、`none`を案内する。

bashは`~/.bashrc`、zshは`~/.zshrc`、fishは`~/.config/fish/conf.d/gdtvm.fish`を対象とする。homeは§2.2のOS lookup値を使う。

POSIX shell:

```sh
# >>> gdtvm initialize >>>
export PATH='<escaped-shim-directory>':"$PATH"
# <<< gdtvm initialize <<<
```

fish:

```fish
# >>> gdtvm initialize >>>
fish_add_path --prepend '<escaped-shim-directory>'
# <<< gdtvm initialize <<<
```

literalを各shell規則でescapeし、command substitutionを生成しない。marker 0件なら末尾へ追加、完全一致1件ならno-op、不完全/複数なら自動修正せず診断する。owner、permission、symlink、before digestを検査し、backup→temp→flush→atomic replaceを行う。利用者fileをsource/evaluateして編集判断しない。

### 5.3 glibc

対応Linuxはamd64/glibcだけ。OS/archはkernel API、libcは実体loader/libc identityとprobeで判定し、環境変数や`ldd`表示だけで決めない。musl、WSL上の異種filesystem、containerで必須能力が欠ける場合は`E_PLATFORM_UNSUPPORTED`。gdtvmはglibcやbuild packageを導入しない。

## 6. PATH順位

gdtvm shim directoryはuser PATH/profileの先頭へ1回だけ追加する。個別tool bin、version payload、current directoryをglobal PATHへ追加しない。shim resolverがproject/user selectionを解決する。

PATHへ追加するのはshim directory 1件だけ。個別tool/version path、credential、download URLを入れない。Windowsはcase-insensitive、Linuxはbyte-sensitiveに重複を扱う。

## 7. setup transactionとremove

setup Planは[04-storage-and-data.md](04-storage-and-data.md)§16の`SetupPlan`を正本とし、mode、旧/新data/distribution root、検証済みfilesystem能力、current/shim方式、shim path、integration方式と対象file/registry value、backup、再起動要否をまとめて1回確認する。これらをwarning parameterや`writes[]`から逆算しない。

書込み順はroot/state初期化、shim生成、integration backup、integration変更、再読検証、setup state commit。途中失敗は今回作成物だけ逆順rollbackし、既存tool/stateを削除しない。mode変更はdataを移動せず新rootを初期化し、旧rootを結果へ表示する。

`setup`は冪等とする。setup済みrootへの再実行は差分だけを適用し、既に一致する項目をno-opとして報告する。portable rootの移動後、link/shim/integrationを作り直す手段はこの再実行である。

`setup --remove`はsetup stateで所有を証明できるPATH entryまたはmarkerだけを除去する。shim、tool、state、cacheは残す。backupは最新1世代を保持しsecretを記録しない。

## 8. editor・service・長寿命process

VS Code等は起動時環境を保持する。setup/use後は新terminalまたはReload Windowが必要な場合を表示する。language serverが実体pathを保持する場合は再起動を案内するが、processを強制終了しない。

service、scheduled task、CIはuser selectionに依存せずproject完全versionを使う。

## 9. platform error

stable platform reason codeは次のexactly 7件とする。

| code | 条件 |
|---|---|
| `E_PLATFORM_UNSUPPORTED` | OS/arch/libc/FSが対象外 |
| `E_PERMISSION` | user権限、owner、ACL/mode不適合 |
| `E_PATH_UNSAFE` | root逸脱、link/reparse、PATH値不正 |
| `E_LINK_FAILED` | junction/symlink/hardlink生成・検証失敗 |
| `E_SHELL_PROFILE_CONFLICT` | marker重複、不完全、同時変更 |
| `E_PATH_INTEGRATION_FAILED` | registry PATHの型、長さ、競合、再読不一致 |
| `E_UNSUPPORTED_SHELL` | 対象shellに安全なintegrationがない |

errorには対象logical path/value名、実行しなかった変更、利用者が選べる安全な代替を含める。secretやregistry raw全体を通常logへ出さない。
