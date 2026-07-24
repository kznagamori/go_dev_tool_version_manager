# go_dev_tool_version_manager 仕様書

本ディレクトリは、`anyvm_win` の全機能を継承し、Windows と Linux に対応する Go 製開発ツール・バージョンマネージャー `go_dev_tool_version_manager`（コマンド名 `gdtvm`）の実装仕様である。

この仕様書は既存ソースコードを参照しなくても実装できることを目的とする。実装時に仕様と旧実装の挙動が異なる場合は、本仕様を正とする。ただし、旧実装が提供した機能は [01-product-requirements.md](01-product-requirements.md) の機能対応表によりすべて引き継ぐ。

旧製品名、旧command名、旧実装方式への言及は移行範囲を示す情報であり、実装入力ではない。適合するGo実装および標準registryを作成するために、`anyvm_win` のsource、binary、version cache、生成scriptを読むことを前提にしてはならない。旧機能の規範動作は本仕様の対応先にすべて書かれていなければならず、不足を旧sourceの挙動で補完せず仕様を先に改訂する。

## 文書一覧

| 文書 | 内容 |
|---|---|
| [01-product-requirements.md](01-product-requirements.md) | 目的、対象、用語、機能要件、旧機能対応 |
| [02-architecture.md](02-architecture.md) | レイヤー、コンポーネント、依存方向、並行処理 |
| [03-storage-and-state.md](03-storage-and-state.md) | ポータブル／ユーザー／マルチユーザーモード、ディレクトリ、状態、ロック |
| [04-cli.md](04-cli.md) | CLI構文、コマンド、オプション、終了コード、対話 |
| [05-configuration.md](05-configuration.md) | グローバル設定、プロジェクト設定、環境変数、優先順位 |
| [06-tool-definition-schema.md](06-tool-definition-schema.md) | TOMLツール定義スキーマ、処理ステップ、テンプレート |
| [07-registry.md](07-registry.md) | 開発branch上の標準定義、release同梱、検証、ローカル定義 |
| [08-installation-and-runtime.md](08-installation-and-runtime.md) | 発見、導入、検証、切替、shim、環境構築 |
| [09-platform-integration.md](09-platform-integration.md) | Windows/Linux、シェル、リンク、VS Code |
| [10-internal-api.md](10-internal-api.md) | Wails v3を見据えた内部API、イベント、エラー |
| [11-security.md](11-security.md) | 信頼境界、ローカル定義、アーカイブ、外部コマンド |
| [12-standard-tools.md](12-standard-tools.md) | 17標準ツールの取得元、導入、環境変数、互換機能 |
| [13-quality-and-release.md](13-quality-and-release.md) | テスト、受入条件、Goビルド、配布、保守 |
| [14-data-contracts.md](14-data-contracts.md) | 状態、カタログ、JSON出力、helper、制限値の機械契約 |
| [15-reference-definition.md](15-reference-definition.md) | 完全なツール定義例、実装時の解釈基準、設定駆動受入条件 |
| [16-implementation-progress.md](16-implementation-progress.md) | Windows先行・Linux後続の実装、評価、停止・再開用進捗チェックリスト |
| [17-repository-documentation.md](17-repository-documentation.md) | GitHub公開用README、詳細操作ガイド、build・release検証説明の作成仕様 |

## 文書の規範領域と矛盾の扱い

各文書は次の領域を最終的に規定する。別文書からの要約は参照先の規範を置き換えない。

| 規範領域 | 正本 |
|---|---|
| 製品範囲、旧機能の踏襲、非機能要求 | 01 |
| レイヤー、依存方向、抽象port | 02 |
| path、directory、永続状態、lock | 03および機械形式について14 |
| 公開CLI構文、option、出力先、終了code | 04 |
| 実行file隣接設定/project設定 | 05 |
| tool definitionの意味と実行primitive | 06 |
| registry source、release同梱、schema検証 | 07 |
| install、selection、shim、runtime状態遷移 | 08 |
| Windows/Linux、shell、VS Code | 09 |
| Application Service、event、Wails bridge | 10 |
| trust、承認、外部実行security | 11 |
| 17標準toolとhelperの機能 | 12 |
| test、build、release合格条件 | 13 |
| TOML/JSON/receipt/Plan/helperの機械契約と上限 | 14 |
| tool definitionの完全fixture | 15 |
| 実装・評価の順序、進捗、停止・再開 | 16 |
| GitHub公開README・詳細操作ガイド | 17 |

同じ意味の規則に差がある場合、番号の大小や記載順で一方を選んではならない。実装とregistry作成を停止し、矛盾する両箇所と影響するtest・進捗項目を同時に改訂してから再開する。正本の一般則に対して別章が対象を限定して明示した例外は、その対象だけに適用する。例外か矛盾か判断できない場合も仕様改訂を必要とする。

実装に必要なenum、default、上限、失敗時動作が書かれていない場合、GoライブラリやOSの暗黙defaultを製品仕様として採用してはならない。未定義事項をissueとして記録し、仕様、fixture、受入testを追加してから実装する。

## 要求追跡表

本仕様への適合判定は次の対応で行う。単に章や型が存在するだけでなく、右欄の受入条件を満たすことを要求する。

| 要求 | 規範箇所 | 受入条件 |
|---|---|---|
| 本仕様だけからGo実装可能 | 全文書、特に02、03、06、08、10、14、15 | 入力、状態、処理順、失敗、永続形式、上限、platform差、試験期待値を旧sourceなしで決定できる |
| 既存機能をすべて踏襲 | 01章8節・8.1節、12章、13章8節・9節 | 共通CRUD、17ツール固有処理、shell、junction、shim、外部helperをcontract testと回帰試験で確認する |
| commandとoptionを再検討 | 04章2.1節および3節 | 採用理由、完全な構文、排他、対話、出力、終了codeが定義され、旧構文へ暗黙fallbackしない |
| tool追加・変更を設定駆動化 | 06章、12章、15章4節 | 既存primitiveで表現できるtoolはGo source変更なしで定義追加から全標準操作まで成功する |
| 外部programの取得利用を踏襲 | 08章9節、11章、12章21節、14章14節 | helper/backend/補助artifactを計画表示し、取得、digest検証、隔離実行、receipt、再利用、削除まで管理する |
| Wails v3に備えてlibrary化 | 02章、10章、13章15節 | CLIとWails bridgeが同じApplication Serviceを使用し、CLIにdomain、filesystem、network、process判断を置かない |

## 実装完了の判定

次のすべてを満たすまで、仕様に基づく移植完了とはしない。

1. 04章の全commandが10章のApplication Serviceを経由し、CLI薄層の禁止事項に違反しない。
2. 07章3節の17 tool definitionと必須helperをregistryへ登録し、12章の全platform受入条件を満たす。
3. 06章と14章のschema、制限値、状態contractをfixtureとstrict parserで検証する。
4. 01章8節・8.1節の各行に対応する13章8節・9節のcontract testと回帰試験が成功する。
5. Windows標準ユーザーとLinux非rootで、release archive展開、setup、同梱registry読込み、install、use、shim、VS Code起動、uninstall、repair、self-updateをend-to-end確認する。
6. local definitionだけを追加したfixtureでGo sourceを変更せずtool追加受入試験が成功する。
7. 外部programを使うPython、LLVM、MinGW、WinLibs、.NET、Rustの計画と監査に、実行物、取得元、完全版、digest、license、argv、書込み先が現れる。
8. CLI contract testと将来Wails bridge contract testが同一のlocale-neutral request/result/error/eventを共有する。

## 仕様上のキーワード

本文の「必須」「禁止」「推奨」「任意」は次の意味で用いる。

- **必須**: 適合実装が必ず満たす。
- **禁止**: 適合実装が行ってはならない。
- **推奨**: 原則として満たす。満たさない場合は理由と代替テストを記録する。
- **任意**: 実装してもしなくてもよいが、実装時は記載された制約に従う。

## 固定された製品判断

- 製品名は `go_dev_tool_version_manager`、CLI名は `gdtvm` とする。
- クライアント版は日本時間のリリース日を使う `YYYY.mm.DD.XX` とし、同日の通常初版は`00`、同日中のbug fixまたは特別releaseごとに`XX`を1増やす。
- 設定およびツール定義は TOML とする。
- 製品設定の正本は実行file隣の`gdtvm.toml`一つとし、project選択だけを`.gdtvm.toml`へ分離する。自己更新は既存`gdtvm.toml`を上書きしない。
- gdtvm固有環境変数はmulti-userで明示許可した`GDTVM_USER_HOME`だけを読み、標準proxy環境変数とmanaged toolの子process環境を例外とする。
- Windows amd64/arm64、Linux amd64/arm64をクライアント対象とする。
- ポータブル方式を既定とし、OSのユーザーデータ方式とread-only共有distribution＋ユーザー別dataによるマルチユーザー方式も提供する。
- 標準定義は同一GitHubリポジトリの開発branchの`/registry/`で管理し、各OS・architecture向けclient release archiveへ同梱する。
- 標準registryのcanonical tree SHA-256をrelease binaryへ埋め込み、同一releaseのclientとregistryの混在・直接改変を検出する。
- GitHub Actionsは`vYYYY.mm.DD.XX` tagをtriggerに4種類のarchiveを作成する。archive、SHA-256 `checksums.txt`、SBOM、provenanceをGitHub Release assetとして公開し、対象成果物のGitHub artifact attestationを別途発行する。
- バージョン指定は完全指定だけを認める。唯一の例外は、導入時に解決結果を確認する `--latest` である。
- link方式の通常切替にはWindowsでディレクトリ・ジャンクション、Linuxでシンボリックリンクを使う。rustup等のbackend方式は完全selectorを使う。ツール本体の切替用コピーは禁止する。
- プロジェクト選択および動的な環境設定には、Goで実装するネイティブshimを使う。外部の `symexe.exe` には依存しない。
- 一般ユーザー権限で完結させ、自動昇格、HKLM変更、システム環境変数変更、システムパッケージの自動導入を行わない。
- コア機能は同一Goモジュール内の内部ライブラリとし、CLIは入出力変換に限定する。

## 仕様の変更手順

互換性に影響する変更では、次を同時に更新する。

1. 本索引の固定判断または該当仕様。
2. ツール定義スキーマ版または状態スキーマ版。
3. CLI互換性および移行規則。
4. [13-quality-and-release.md](13-quality-and-release.md) の受入試験。

未決事項を実装者の判断だけで挙動へ追加してはならない。曖昧な場合は、安全側、非破壊側、非昇格側、完全バージョン側を選び、仕様改訂を先に行う。
