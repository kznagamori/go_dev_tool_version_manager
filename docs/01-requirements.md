# 製品要求仕様

## 1. 目的と利用者

`gdtvm`は、WindowsとLinuxでGo、Node.js、Python、.NET SDKの完全versionを一般user権限で導入・切替するCLIである。初心者に対する「簡単」とはcommand数の少なさではなく、書籍、公式サイト、projectに記載された完全版をそのまま指定し、意図しない別版を使わないことである。

主対象は次の利用者とする。

- 初心者: 完全版を明示して導入・選択し、version違いを避けたい。
- 中級者以降: 複数版とproject固定を使い、各toolの公式設定command/fileで調整したい。
- script/CI利用者: 非対話、offline cache、読取りJSON、再現可能な完全版指定を使いたい。

企業固有の集中監査、共有配布、独自CA設定、policy serverは対象外とする。企業は開発toolを公式のinstall文書に従って導入すると想定し、gdtvmはその代替を目指さない。OS標準proxy/trust store、一般user、公式配布手順と明示的third-party警告までは対応する。

## 2. v0.1の達成目標

1. Windows amd64とLinux amd64/glibcで同じ基本CLIを提供する。
2. Go・Node.js・Python・.NET SDKの完全版を管理root外へ書かずに導入する。
3. `portable`と`user`の2 modeを提供する。
4. user選択とproject完全版固定をnative shimから解決する。
5. tool固有動作をregistry TOMLで表し、Goコードへtool ID分岐を置かない。
6. upstream設定interfaceを維持しながら、設定・package・cacheの保存先を管理rootへ隔離する。
7. download、検証、展開、commit、selection、uninstallを失敗時に回復可能にする。
8. 通常CLIへ継続的なprogressを表示し、読取り操作には単一JSON結果を提供する。
9. 利用者が不具合に遭遇したとき、`doctor --report`だけで報告材料を作れるようにする。

## 3. 非目標

v0.1では次を扱わない。延期した機能の一覧と再導入手順は[15-deferred.md](15-deferred.md)を正とする。

- Windows/Linux arm64、Linux musl、macOS。
- Go・Node.js・Python・.NET SDK以外の標準tool。
- `multi-user`、system-wide install、自動昇格、UAC、`sudo`、HKLM、system PATH、system package自動導入。
- local definition、local script、plugin、registry単体更新。
- GUI。coreのprogress/cancel境界だけは維持する。
- 署名検証、artifact lock、専用security audit log、SBOM/provenance/attestation。
- tool package managerの再実装。`npm config`、`pip config`等の上流interfaceを置き換えない。
- 部分version、range、wildcard、近似補正、`latest`のproject保存。
- 第三者向け公開Go SDK。

## 4. platform

| OS | architecture/libc | v0.1状態 |
|---|---|---|
| Windows 10/11 | amd64 | 正式対象 |
| Linux | amd64/glibc | 正式対象 |
| Windows | arm64 | 非対応 |
| Linux | arm64またはmusl | 非対応 |

clientが起動することと、全標準toolが導入できることを分けて「対応」と表示してはならない。正式対象は4 toolすべてがCIで合格した上表2行だけである。

## 5. 用語

| 用語 | 意味 |
|---|---|
| distribution root | `gdtvm` binary、同梱registry、文書を置くrelease単位のroot |
| data root | `tools`、`state`、`cache`、`logs`、`tmp`、`locks`を置く可変root |
| portable | distribution rootとdata rootが同じ配置 |
| user | distributionとdataをOS user領域へ配置するmode |
| registry | client releaseへ同梱する検証済み標準tool definition集合 |
| catalog | definitionとupstream metadataから得たplatform別完全version一覧cache |
| payload | 1 tool・完全版・platformの導入実体 |
| receipt | 導入時に確定したartifact、検証、command、環境、storageの記録 |
| user selection | data root内でtoolごとに選んだ管理版 |
| project selection | `.gdtvm.toml`へ固定した完全版。user selectionより優先する |
| shim | command名からproject/user選択を解決して実体を起動するGo製resolver |
| Plan | 実行前に確定した完全版、取得物、検証、process、読書き、警告、rollbackの表示model |
| shared storage | 同じtoolの複数versionで共有する設定、cache、global binary領域 |
| version storage | 完全versionごとに分離するpackage、runtime mutable data領域 |

## 6. tool ID

| 正規ID | 入力alias | 表示名 |
|---|---|---|
| `go` | なし | Go |
| `node` | `nodejs` | Node.js |
| `python` | なし | Python |
| `dotnet` | `dotnet-sdk` | .NET SDK |

正規IDとaliasはASCII lowercase。aliasは入力だけに使い、state、receipt、project設定、JSONは正規IDを返す。対象外toolのIDや挙動を予約仕様として列挙しない。

## 7. 機能要件

### 7.1 初期化

- 公式bootstrapはuser mode、手動archive展開はportable modeを既定にする。
- `setup`はdata root、state、shim、registry compatibilityを初期化する。
- Windowsは`user-path|none`、Linuxは`shell-profile|none`を選べる。対話時の推奨既定はそれぞれ`user-path`、`shell-profile`。
- 変更内容をまとめて表示し1回確認する。既存内容をbackupし、gdtvm marker/entry以外を置換しない。
- `setup`は冪等とする。setup済みrootに対する再実行は差分だけを適用する。

### 7.2 catalog

- 同梱definitionとupstream metadataから完全version、channel、artifact availability、provider、checksumを取得する。
- 公開channelの`stable|prerelease`とsupport lifecycleの`supported|eol|unknown`を分ける。`--latest`はstableかつEOLでない版だけ。
- 存在しない/部分版は拒否し、`available <tool>`を案内する。近似候補を出さない。
- online取得失敗時は期限内cacheを利用できる。`--offline`はnetworkを一切使わない。

### 7.3 install

- `install <tool>@<version>`を基本とし、`install <tool> --latest`だけを例外とする。
- `install`単独ではselectionを変更しない。`--use`で導入成功後にuser/projectを選ぶ。
- Planの冒頭と確認直前にtool、完全版、OS/arch/libc、provider区分、checksum、channel、lifecycle、警告数を要約する。
- Plan本文には全URL、license、artifact、size、checksum、validation probeの実行file/完全版/取得元/digest/reason/argv/cwd/書込み先、全読書きpath、storage、rollbackを表示する。任意helper/backend processは起動しない。
- 公式artifactを優先する。要件を満たせない場合だけthird-partyを定義でき、provider、repository、license、採用理由を表示して承認を得る。
- upstream checksumとの一致を必須とする。
- stagingで安全に展開・probeしてから同一volume renameでcommitする。半端なpayloadをversions配下へ公開しない。
- download、検証、展開、probe、commitのprogressを継続表示する。

### 7.4 selection

- `use <tool>@<version>`は導入済み完全版だけをuserまたはprojectへ選ぶ。未導入なら同じ完全版のinstallを提案し、承認後に連続実行できる。
- project選択、user選択、未選択の由来を`current --explain`で表示する。
- project完全版はuser選択より優先する。

### 7.5 runtime

- shimはreceiptを正としてcommand、固定args、環境、storage rootを解決し、active definitionの変更で既存installを読み替えない。
- managed runtimeは親環境を基底にreceipt環境をcase規則どおりmergeする。install/probeはsanitized環境を使う。
- Windowsはdirectory junction、Linuxはrelative symlinkを通常のcurrent表現に使う。shimはlink破損時もreceiptからexact payloadを解決できる。

### 7.6 configuration/storage

- global `gdtvm.toml`は任意。未配置なら組込み既定値で動く。
- tool設定は上流command/fileを使う。gdtvmは管理rootを環境として渡すが、上流optionを独自schemaへ複製しない。
- definitionはstorageを`kind`と`scope=tool|version`で宣言する。engineはtool名を見ず、宣言からpath、receipt、uninstall保持を決める。
- 標準は設定/content cacheをtool scope、Go global binaryとNuGet cacheをtool scope、Node global package・Python site-packages・.NET CLI homeをversion scopeとする。
- 上流toolが管理rootへ隔離する公開手段を持たない領域（.NETのNuGet.Configと`dotnet tool install -g`配置先など）は、typed storageとして宣言せず公開文書で管理外と明示する。
- tool-scope shared storageはversion uninstallで削除しない。version-scope package/dataは対象versionと同じPlanで削除し、project venv/sourceには触れない。最後のversionでもtool scopeは別のshared purge確認なしに削除しない。

### 7.7 診断とuninstall

- `uninstall`はreceiptで所有を証明できる対象だけをtrash renameし、selection参照を確認する。
- `doctor`はread-only。config/state/registry、root、lock、receipt、link、shim、PATH順位、storage、disk、catalogを項目別に診断する。
- `doctor --report <path>`はsecretと個人pathを除去したMarkdown 1 fileを出力する。利用者はこれをそのまま不具合報告へ添付できる。

### 7.8 output/automation

- 通常textは日本語。tool ID、version、path、URL、error codeは翻訳しない。
- `--json`は読取り専用5 commandだけが持ち、操作完了後に単一documentをstdoutへ出す。progress/log/promptはstderr。状態変更操作の機械判定は終了codeで行う。
- coreは型付きprogress sinkとcancel tokenを使う。

## 8. 初心者向け標準操作

```text
gdtvm setup
gdtvm install node@22.18.0
gdtvm use node@22.18.0
gdtvm current node
```

この4操作を標準とし、`install node`をlatestの暗黙指定にしたり、install成功だけでselectionを変更したりしない。version例は公開文書作成時に実在fixtureへ置き換え、変化する最新版を固定しない。

## 9. 信頼性・privacy

- 完成payload/stateはtemp file flush、atomic rename、backup、lockで保護する。
- 中断やcancel後も既存selectionと完成payloadを保持する。中断した操作の一時物は`tmp/operations/<operation-id>/`単位で安全に削除できる。
- archive traversal、symlink/reparse point、case collision、非NFC、bomb、root逸脱を展開前/中に拒否する。
- command injectionを避けるためshell文字列を作らずargvを渡す。
- credential、proxy password、token、全環境map、user home absolute pathを通常log/receipt/reportへ保存しない。
- telemetryを追加しない。network先はclient bootstrapとdefinitionが宣言したupstreamだけ。
