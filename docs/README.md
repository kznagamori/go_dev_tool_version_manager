# go_dev_tool_version_manager 仕様書

本ディレクトリは、`anyvm_win` の全機能を継承し、Windows と Linux に対応する Go 製開発ツール・バージョンマネージャー `go_dev_tool_version_manager`（コマンド名 `gdtvm`）の実装仕様である。

この仕様書は既存ソースコードを参照しなくても実装できることを目的とする。実装時に仕様と旧実装の挙動が異なる場合は、本仕様を正とする。ただし、旧実装が提供した機能は [01-product-requirements.md](01-product-requirements.md) の機能対応表によりすべて引き継ぐ。

## 文書一覧

| 文書 | 内容 |
|---|---|
| [01-product-requirements.md](01-product-requirements.md) | 目的、対象、用語、機能要件、旧機能対応 |
| [02-architecture.md](02-architecture.md) | レイヤー、コンポーネント、依存方向、並行処理 |
| [03-storage-and-state.md](03-storage-and-state.md) | ポータブル／ユーザーモード、ディレクトリ、状態、ロック |
| [04-cli.md](04-cli.md) | CLI構文、コマンド、オプション、終了コード、対話 |
| [05-configuration.md](05-configuration.md) | グローバル設定、プロジェクト設定、環境変数、優先順位 |
| [06-tool-definition-schema.md](06-tool-definition-schema.md) | TOMLツール定義スキーマ、処理ステップ、テンプレート |
| [07-registry.md](07-registry.md) | orphanブランチ、署名、更新、キャッシュ、オフライン |
| [08-installation-and-runtime.md](08-installation-and-runtime.md) | 発見、導入、検証、切替、shim、環境構築 |
| [09-platform-integration.md](09-platform-integration.md) | Windows/Linux、シェル、リンク、VS Code |
| [10-internal-api.md](10-internal-api.md) | Wails v3を見据えた内部API、イベント、エラー |
| [11-security.md](11-security.md) | 信頼境界、ローカル定義、アーカイブ、外部コマンド |
| [12-standard-tools.md](12-standard-tools.md) | 17標準ツールの取得元、導入、環境変数、互換機能 |
| [13-quality-and-release.md](13-quality-and-release.md) | テスト、受入条件、Goビルド、配布、保守 |
| [14-data-contracts.md](14-data-contracts.md) | 状態、カタログ、JSON出力、helper、制限値の機械契約 |
| [15-reference-definition.md](15-reference-definition.md) | 完全なツール定義例、実装時の解釈基準、設定駆動受入条件 |

## 仕様上のキーワード

本文の「必須」「禁止」「推奨」「任意」は次の意味で用いる。

- **必須**: 適合実装が必ず満たす。
- **禁止**: 適合実装が行ってはならない。
- **推奨**: 原則として満たす。満たさない場合は理由と代替テストを記録する。
- **任意**: 実装してもしなくてもよいが、実装時は記載された制約に従う。

## 固定された製品判断

- 製品名は `go_dev_tool_version_manager`、CLI名は `gdtvm` とする。
- 設定およびツール定義は TOML とする。
- Windows amd64/arm64、Linux amd64/arm64をクライアント対象とする。
- ポータブル方式を既定とし、OSのユーザーデータ方式も提供する。
- 標準定義は同一GitHubリポジトリの orphan ブランチ `registry` で配布する。
- 標準定義リリースタグは `registry-v<SemVer>` とし、Ed25519署名を必須とする。
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
