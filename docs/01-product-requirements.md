# 製品要求仕様

## 1. 製品の目的

`gdtvm` は、開発言語、SDK、ビルドツール、コンパイラを一つのCLIで取得、検証、展開、切替、削除するバージョンマネージャーである。Windows中心だった既存機能を失わず、Linuxにも同一の概念とコマンドで対応する。

主対象は、複数の開発環境を使う初心者から上級者までである。初心者には「完全なバージョンを指定して install/use する」という小さな概念だけを見せる。一方、企業ネットワーク、オフライン、独自ツール、プロジェクト固定、GUIからの利用に必要な拡張点を備える。

## 2. 達成目標

1. 一般ユーザーがrelease archiveを任意の書込み可能フォルダーへ展開して開始できる。
2. 標準ツール定義はrelease archiveへ同梱し、初回起動時にnetworkから別取得しない。
3. ツール追加や取得URL・展開処理の変更を、原則としてGoコード変更なしのTOML更新で行える。
4. すべての導入物を製品管理ルート内へ隔離し、巨大なツールディレクトリをコピーせず切り替える。
5. Windowsのcmd、Windows PowerShell 5.1、PowerShell 7以降、およびLinuxのbash、zsh、fishで利用できる。
6. `code.exe .` で端末から起動したVS Codeと、その統合端末・拡張プロセスが選択環境を利用できる。
7. CLIと将来のWails v3 GUIが同じアプリケーションサービスを利用できる。
8. ダウンロード、checksum、任意の上流artifact署名、ローカル定義、外部コマンドの信頼判断を利用者に明示する。

## 3. 非目標

- OSのパッケージマネージャーの置換。
- 管理者権限が必要なシステムドライバー、Windows SDK全体、Visual Studioなどの自動導入。
- 任意の部分バージョン、範囲、`latest` をプロジェクト設定へ保存すること。
- 旧 `anyvm_win` のJSON状態、配置、生成済みスクリプトとのデータ互換。
- 第三者がGoモジュールとしてimportする公開SDK。内部APIは同一リポジトリ内のCLIと将来GUI専用である。
- macOS対応。スキーマで将来追加できるようにするが、初期リリースの受入対象外とする。

## 4. 対象プラットフォーム

| OS | アーキテクチャ | クライアント要件 |
|---|---|---|
| Windows 10/11 | amd64 | 必須 |
| Windows 11 | arm64 | 必須 |
| Linux | amd64 | 必須、libc非依存のクライアント |
| Linux | arm64 | 必須、libc非依存のクライアント |

Linux CLIは `CGO_ENABLED=0` の静的Goバイナリとし、muslへリンクしたバイナリとは定義しない。管理対象ツール自体のlibc要件は各ツール定義に属する。

artifact選択のarchはclient buildの`GOARCH`を使い、native hardwareを検出して実行中に別archへ暗黙変更しない。Windows arm64上でamd64 clientがemulation実行されている場合はamd64 toolを管理し、`doctor`がnative arm64 clientの利用を推奨する。異architecture artifactを「動くかもしれない」という理由で候補にしない。

## 5. 用語

| 用語 | 定義 |
|---|---|
| 管理ルート | `tools`、`registry`、`state` などを格納するgdtvmのデータルート |
| 配布ルート | 実行file、`gdtvm.toml`、`registry/`、文書を含む配布物のルート。ポータブルモードでは管理ルートと一致する |
| データルート | `tools`、`state`、`cache` 等の可変dataを置くルート。ポータブルでは配布ルートと一致し、user/multi-userではユーザー別に分離する |
| ポータブルモード | 実行ファイルのあるフォルダーを管理ルートとする既定モード |
| ユーザーモード | OS APIで得たuser home配下またはWindowsユーザーデータ領域を使うモード |
| マルチユーザーモード | 管理者が配置するread-only共有distributionと、ユーザーごとに分離したdata rootを併用するモード |
| 標準定義 | clientと同じrelease archiveへ同梱した`registry/`のツール定義 |
| ローカル定義 | 利用者が追加し、内容hashによる承認対象となるツール定義 |
| ツールID | CLIとTOMLで共通の小文字kebab-case識別子 |
| 完全バージョン | レジストリの1件に曖昧さなく一致する文字列 |
| ユーザー選択 | ユーザー全体の既定バージョン |
| プロジェクト選択 | 最寄りの `.gdtvm.toml` が固定するバージョン |
| 有効選択 | CLI明示、プロジェクト、ユーザーの優先順位で解決した最終選択 |
| currentリンク | `selection_strategy="link"` のツールで、ユーザー選択の導入先を指すWindowsジャンクションまたはLinux symlink |
| shim | 呼び出されたファイル名から管理ツールを判定し、有効バージョンと環境を解決して実体を起動する小型ネイティブ実行物 |
| 定義レジストリ | ツール定義、スキーマ、message、license、上流検証情報をclientとともに管理・配布する`registry/` tree |
| バージョンカタログ | 各公式配布元を照会して得た、導入可能な完全バージョンとアーティファクトのキャッシュ |

## 6. ツールID

初期標準ツールIDは次の17件とする。

`android-sdk`, `bazel`, `cmake`, `dart`, `dotnet`, `flutter`, `go`, `gradle`, `jdk`, `kotlin`, `llvm`, `mingw`, `ninja`, `node`, `python`, `rust`, `winlibs`

別名は入力時だけ受け付け、保存時は正規IDへ正規化する。最低限 `nodejs`→`node`、`androidsdk`→`android-sdk`、`java`→`jdk`、`.net`→`dotnet` を提供する。IDはASCII小文字で始まり、ASCII小文字・数字・単一ハイフンだけを含み、2～40文字とする。project schemaの予約key `disabled` はtool IDに使用できない。全ID/aliasはregistry内で一意とし、aliasが別toolの正規IDやaliasと衝突してはならない。

## 7. 機能要件

### 7.1 初期化と自己診断

- client、`gdtvm.toml`、registry、README、USER_GUIDE、LICENSEを含むrelease archiveを配布する。
- 初回に管理ルートを作成し、release archiveへ同梱された標準定義を検証して読み込む。
- シェル統合を対話的に設定、確認、修復、除去できる。
- 現在の権限、ファイルシステム、リンク作成、PATH、registry tree hash、状態整合性、ツール実体を診断できる。
- 修復は導入済みツールを再ダウンロードせず、状態・リンク・shim・一時物を可能な限り再構築する。

### 7.2 カタログ

- 対応ツール一覧、導入可能版一覧、導入済み版一覧、現在の選択を表示する。
- 公式配布元からバージョン情報を更新し、オフライン時は最後に検証済みのキャッシュを使う。
- prerelease、nightly、EOLを定義のチャネル情報として保持できるが、既定の `--latest` はstableだけを対象にする。

### 7.3 導入

- 完全バージョンを指定して導入できる。
- `--latest` はstable最新版を完全バージョンへ解決し、取得元、バージョン、対象OS/arch、検証状態、容量が判明していれば容量を表示してから確認する。
- 取得進捗、検証進捗、展開進捗をイベントとして通知する。
- ZIP、tar系、7z、自己展開exe、公式インストーラーの管理ルート内展開、および外部ヘルパーを用いる処理を定義駆動で行う。
- 依存する管理対象ツールを計画に含め、自動導入する。OSパッケージは不足を表示するだけで自動導入しない。
- 成功後はダウンロード物を既定で削除する。設定により保持できる。
- 同一バージョンの再導入は整合性を検査して成功扱いにする。破損時は `repair` または明示的再導入を案内する。

### 7.4 切替

- ユーザー選択とプロジェクト選択を提供する。
- 未導入版を `use` した場合、対話時は導入計画を提示し確認後に導入する。非対話時は `--yes` がなければ失敗する。
- 部分指定、範囲、チャネル名、`latest` を `use` と設定ファイルでは拒否する。
- link方式のツールでは、Windowsのユーザー選択 `current` にディレクトリ・ジャンクション、Linuxには相対symlinkを優先する。rustup等のbackend方式はbackend selectorで切り替え、実体treeを指すcurrentリンクを作らない。
- プロジェクト選択はshimで動的解決し、ツール本体の複製やプロジェクトごとの巨大なリンクツリーを作らない。
- 選択解除は導入済み本体を削除しない。

### 7.5 実行環境

- PATH追加、環境変数設定、コードページ指定、既定引数、実体パスをツール定義で表現する。
- 子プロセスへ適用する環境は、親環境を基準に、gdtvm管理値を決定的な順序で上書きする。
- shimは終了コード、標準入力、標準出力、標準エラー、引数、作業ディレクトリ、シグナルを可能な範囲で透過する。
- Windowsのshimは `.cmd` や `.ps1` ではなくネイティブ `.exe` とし、cmdとPowerShellの双方から同一に動作させる。
- `exec` により選択済み一式、または明示した完全バージョンの環境で任意コマンドを実行できる。

### 7.6 削除

- 指定した完全バージョンだけを削除できる。
- 選択中、依存中、実行中の可能性がある版は警告し、既定では削除を拒否する。
- `--force` でも管理ルート外を削除してはならない。
- シェル統合を除去する際は、gdtvmのマーカー範囲だけを削除し、既存内容と既存HKCU AutoRun値を復元する。

### 7.7 設定駆動の拡張

- 新しいツール、OS/arch、取得方法、環境変数、公開コマンド、依存関係はTOMLだけで追加できる。
- 定型処理で不足する場合、OS別外部コマンドhookを使用できる。
- プロジェクトファイルへhookや取得URLを書けないようにする。
- ローカル定義は初回または内容ハッシュ変更時に、パス、ハッシュ、実行予定hook、取得元を表示して承認を求める。

### 7.8 国際化と自動化

- 日本語と英語のメッセージを提供し、OSロケールを自動選択する。
- `--lang ja|en` とグローバル設定で上書きできる。
- 人向け出力と機械向けJSON出力を分離する。
- 非対話実行では入力待ちを発生させず、安全でない判断を既定で拒否する。

## 8. 旧機能からの対応表

| 旧機能 | 新仕様 | 備考 |
|---|---|---|
| `init` | `gdtvm setup` | 管理ルート、定義、shim、シェル統合をまとめる |
| ツール別 `update` | `gdtvm refresh [tool]` | 公式配布元からカタログ更新 |
| 全ツール `update` | 引数なし `refresh` | 失敗ツールがあっても残りを継続し集約結果を返す |
| `install -l` | `available <tool>` | 一覧と導入を分離 |
| `install -v` | `install <tool>@<version>` | 完全指定のみ |
| `install --latest` | 同名オプション | 解決完全版と警告を必ず表示 |
| `versions` | `installed [tool]` | `*`相当の選択状態も表示 |
| `version` | `current [tool]` | 引数なしは全ツール |
| `set -v` | `use <tool>@<version>` | user既定。`--project`でプロジェクト |
| `unset` | `disable <tool>` | `--project`対応、`--all`で旧全解除 |
| `uninstall -v` | `uninstall <tool>@<version>` | 安全検査を追加 |
| `rehash` | 通常不要、`repair` | shim/index/linkの整合性を回復。Windows link置換には短い非原子的区間を許す |
| バージョンJSONキャッシュ | バージョンカタログ | release同梱definitionと上流取得結果を分離 |
| ツール別activate/deactivate | shim＋shell初期化 | 旧環境変数は標準定義へ移植 |
| Windows junction | link方式のWindows `current` junction | 一般ユーザー、ローカルNTFSを前提 |
| Ninjaのsymexe | 内蔵Go shim | `OPTS`、`CODEPAGE`、`PATH`、`EXE`相当を汎用化 |
| Rust rustup管理 | 定義駆動rustup backend | CARGO_HOME、RUSTUP_HOME、sccacheを継承 |
| Python/WiX展開 | 定義駆動helper/step | pipを含むポータブル環境を維持 |
| LLVM/MinGW/WinLibsの7-Zip | helper依存とextract step | 7-Zipを検証して管理ルートへ取得 |
| NuGet同梱と分離キャッシュ | dotnet定義 | NuGet環境変数とnuget実行物を維持 |
| setup_jp.bat | `setup` | PS 5.1/7とcmdをバックアップ付きで設定 |
| エラーログ | `logs`＋構造化イベント | 秘密情報をマスクしローテーション |
| client/定義更新 | `gdtvm self-update` | 公式GitHub Releaseのchecksumとarchive SHA-256を照合し、clientと標準定義を一括更新 |

旧Ninja shimの参照実装は `https://github.com/kznagamori/symexe` である。新実装はこの設定概念と実行透過性を機能要件として再実装するが、そのC++実行ファイルやリポジトリをruntime dependencyにはしない。

### 8.1 ツール固有機能の踏襲監査

旧Windows版の各tool実装で共通CRUD以外に行っていた処理は、次の仕様へ移す。ここにないtool固有のGo分岐を追加せず、標準definition/helper/backendで表現する。

| tool | 踏襲する固有機能 | 規範箇所 |
|---|---|---|
| Android SDK | download pageからcommand-line tools build発見、`latest` layout、SDK環境と追加packageを含むmutable root | 12.4 |
| Bazel | 単一実行物/zipの版別取得と公開 | 12.5 |
| CMake | top-level SDK directory正規化、bin command群 | 12.6 |
| Dart | SDK archive、版間共有`PUB_CACHE` | 12.7 |
| .NET | SDK archive、`nuget.exe`補助取得、CLI/NuGet cacheの管理root分離 | 12.8 |
| Flutter | stable SDK archive、共有`PUB_CACHE`、mutable internal cache | 12.9 |
| Go | SDK archive、GOROOT/GOPATH/GOBIN/cache/module環境の分離 | 12.10 |
| Gradle | distribution archive、GRADLE_HOME/cache、JDK要件 | 12.11 |
| JDK | Temurin feature/release asset選択、JAVA_HOME | 12.12 |
| Kotlin | compiler archive、JDK依存、script launcher | 12.13 |
| LLVM | self-extracting配布物の7-Zip展開、LIBCLANG_PATH | 12.14 |
| MinGW | long release ID/7z asset、compiler bin layout | 12.15 |
| Ninja | 単一exe、旧symexeのtarget/PATH/引数/codepage委譲 | 12.16 |
| Node.js | version index、archive、node/npm/npx/corepack | 12.17 |
| Python | Windows installerのWiX分解、MSI administrative extraction、ensurepip | 12.18 |
| Rust | rustup-init、managed CARGO_HOME/RUSTUP_HOME、完全toolchain、GNU設定、任意sccache | 12.19 |
| WinLibs | GCC/任意LLVM構成のlong ID、7z展開、LIBCLANG_PATH | 12.20 |

踏襲とは、取得・導入・一覧・選択・解除・削除・環境構築という利用者機能を維持することであり、旧版の誤った環境変数名、未検証download、PATH依存の外部command、非原子的状態更新まで再現することではない。公式仕様でdeprecatedまたは意味が違う値は、同じ目的を満たす現行のdocumented値へ置換し、その差を12章と回帰試験へ明記する。

## 9. 旧データからの移行

旧 `anyvm_win` のディレクトリや状態を自動検出・変更・削除しない。初回Go版は新しい管理ルートへ導入する。利用者は必要な完全版をGo版で再導入し、動作確認後に旧版を手動撤去する。

旧tool directoryを新receiptなしで流用する機能は初期版では提供しない。巨大な再downloadを避ける将来のimport機能を追加する場合も、artifact/重要fileのdigest検証、staging、receipt生成、元directory非破壊を必須とし、別仕様で定める。

## 10. 初心者向け標準操作

通常利用で覚える必要がある操作は次の4つに限定する。

1. `gdtvm setup`
2. `gdtvm install node@22.18.0`
3. `gdtvm use node@22.18.0`
4. `gdtvm current`

プロジェクト固定、定義更新、診断、ローカル定義、オフラインなどはヘルプの「詳細」区分に置く。通常の成功出力には内部用語（snapshot、receipt、hook、shim owner）を出さない。

## 11. 性能・信頼性要件

- `current` とshimの選択解決は、ネットワークへ接続せず、通常のローカルSSDで100 ms以内を目標とする。
- shim起動による追加遅延は、キャッシュ有効時の中央値20 ms以内を目標とする。
- 1ツール1版の導入はプロセス間排他し、別ツールのダウンロードは設定上限まで並列化できる。
- 電源断や強制終了後も、完成版ディレクトリを半端な状態として公開しない。
- すべての書換えは一時ファイル＋flush＋同一ボリューム上のrenameを基本とする。
- ネットワーク再試行はGET/HEADなど冪等操作だけに行い、指数バックオフと上限を持つ。

## 12. プライバシー

テレメトリは送信しない。GitHubおよび各ツール配布元への通常のHTTP要求以外の外部通信を行わない。ログへアクセストークン、Proxy認証情報、Cookie、クエリ内秘密値、ユーザーホーム全体の内容を記録しない。管理ルート外の`.gdtvm.toml`を全filesystem走査・履歴索引化せず、各操作のcwdから必要な1ファイルだけを探索する。
