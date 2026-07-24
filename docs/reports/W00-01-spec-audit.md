# W00-01 仕様監査レポート（規範用語・未決事項・矛盾のissue一覧）

- タスクID: `W00-01`
- 目的: `docs/README.md` と番号付き 01〜17 の全18文書を読み、規範用語を確定し、未決事項・矛盾を洗い出し、実装者の暗黙判断として残るものがない状態を確認する。
- 実施日時: 2026-07-24（JST）
- 対象commit: `fef3055`
- 実施者: Claude Code
- 実施環境: Linux 6.18.5 x86_64 / go1.24.7（bootstrap）/ bash

## 1. 監査範囲と方法

全18文書を全文精読した（合計5433行）。

| 文書 | 行数 | 文書 | 行数 |
|---|---:|---|---:|
| README.md | 121 | 09-platform-integration | 178 |
| 01-product-requirements | 220 | 10-internal-api | 359 |
| 02-architecture | 164 | 11-security | 170 |
| 03-storage-and-state | 188 | 12-standard-tools | 511 |
| 04-cli | 311 | 13-quality-and-release | 372 |
| 05-configuration | 253 | 14-data-contracts | 493 |
| 06-tool-definition-schema | 626 | 15-reference-definition | 204 |
| 07-registry | 211 | 16-implementation-progress | 383 |
| 08-installation-and-runtime | 337 | 17-repository-documentation | 332 |

方法: 各文書の規範領域（README「文書の規範領域」表）を正本として、複数文書に現れる横断的契約（enum・既定値・上限・終了コード・E_*・版形式・tool/helper数・shell集合・承認policy・conflicts対称性・templateスコープ）を相互照合し、値の不一致・欠落・暗黙判断の有無を確認した。

直近の停止記録（2026-07-24）で先行レビューが矛盾・欠落12項目＋判断4項目を利用者確認の上すべて反映済みである。本監査はその結果を独立に再確認し、残存項目を確定するものである。

## 2. 規範用語カタログ（実装リファレンス）

以降の実装で参照する規範値。値の正本は各文書であり、本表は要約（README規範領域表のとおり要約は正本を置換しない）。

### 2.1 中核enum

| 用語 | 値 | 正本 |
|---|---|---|
| Mode | `portable` / `user` / `multi-user` | 03, 05 |
| Scope | `user` / `project` | 02, 04 |
| Channel | `stable` / `prerelease` / `nightly` / `eol` | 02, 06 |
| os | `windows` / `linux`（将来値はschema更新、macOSは非目標） | 06 §4, 01 §3 |
| arch | `amd64` / `arm64` | 06 §4 |
| libc | `any` / `glibc` / `musl` / `none`（既定 Win=`none`, Linux=`any`） | 06 §4 |
| version_scheme | `semver` / `numeric` / `jep223` / `date` / `lexical` / `regex` | 06 §3 |
| selection_strategy | `link` / `backend`（archive=link, rustup=backend） | 06 §4 |
| manager | `archive`（既定） / `rustup` | 06 §3 |
| artifact_kind | `official` / `third-party` | 06 §4 |
| relocation | `portable` / `repair-required` / `fixed` | 06 §4 |
| Digest algorithm | schema 1 は検証済み判定に `sha256` のみ | 06 §6.1, 11, 14 |
| verification(receipt) | `verified` / `verified-signed` / `unverified-approved` | 14 §9 |
| upstream_signature | `verified` / `not-provided` / `not-applicable` | 14 §9 |
| local_definition_policy | `prompt`（既定） / `deny` / `allow` | 05, 11 |
| unverified_artifact_policy | `prompt`（既定） / `deny`（`allow`は存在しない） | 05, 11 |

### 2.2 クライアント版

- 形式 `YYYY.mm.DD.XX`（JST release日、`00`開始、同日修正で`XX`+1）。正本は repository root の `/VERSION` 1行（UTF-8 BOMなし、末尾LF 1つ）。
- 正規表現 `^[0-9]{4}[.](0[1-9]|1[0-2])[.](0[1-9]|[12][0-9]|3[01])[.][0-9]{2}$` ＋実在Gregorian日付検査。
- 比較は `(YYYY,mm,DD,XX)` を10進整数化した4要素tupleの辞書順。SemVer変換・部分一致・zero padding省略を禁止。
- tag は `v<version>`。正本: 13 §1.2。

### 2.3 終了コードと E_*（相互照合済み・完全対応）

終了コード 0〜12（04 §7）。E_* 全30コード（10 §9）が04 §7の対応表へ漏れなくmapされることを確認した。

| code | E_* |
|---:|---|
| 0 | （成功・変更不要、E_*なし） |
| 1 | E_STATE_CORRUPT, E_PLAN_STALE, E_COMMAND_AMBIGUOUS, E_UPDATE_FAILED, E_DEPENDENCY_CONFLICT, 一般失敗 |
| 2 | E_USAGE |
| 3 | E_CONFIG_PARSE, E_CONFIG_SCHEMA, E_REGISTRY_INVALID, E_DEPENDENCY_CYCLE |
| 4 | E_TOOL_UNKNOWN, E_VERSION_INVALID, E_VERSION_NOT_FOUND, E_VERSION_NOT_INSTALLED, E_PLATFORM_UNSUPPORTED |
| 5 | E_CATALOG_MISSING, E_OFFLINE, E_NETWORK |
| 6 | E_DIGEST_MISMATCH, E_SIGNATURE_INVALID, E_POLICY_DENIED, E_ARCHIVE_UNSAFE, E_DEFINITION_UNAPPROVED |
| 7 | E_HOME_NOT_WRITABLE, E_LINK_FAILED |
| 8 | E_PROCESS_FAILED |
| 9 | E_LOCKED（lock競合・lock待機timeout） |
| 10 | E_CANCELLED |
| 11 | E_PARTIAL |
| 12 | （doctorがerror検出、E_*なし） |

`E_TIMEOUT` は文脈依存（network起因=5, 外部process/hook起因=8）。この規則で30コード全てが単一codeへ確定する。

### 2.4 設定既定値と上限（05 §3 と 14 §16 が一致）

主要な変更可能fieldの既定値と、14 §16 の built-in hard maximum を相互照合し一致を確認した（definition_bytes 2MiB/16MiB, catalog_response 32MiB/256MiB, release_metadata 2MiB/16MiB, release_archive 1GiB/4GiB, artifact 64GiB/256GiB, download_cache 10GiB/1TiB, archive_entries 1e6/2e6, extracted 128GiB/512GiB, single_extracted 64GiB/256GiB, compression_ratio 1000/2000, process_output 8MiB/64MiB, max_redirects 10/20 等）。

`registry/registry.toml` bytes・definition step count・dependency nodes・template rendered string・path components は05に対応fieldがなく built-in既定に固定（14 §16）。この分担も矛盾なし。

### 2.5 収録物・コマンド・shell

- 標準tool 17件（`android-sdk, bazel, cmake, dart, dotnet, flutter, go, gradle, jdk, kotlin, llvm, mingw, ninja, node, python, rust, winlibs`）＋ helper 2件（`seven-zip, wix`）。01 §6 / 07 §3 / 12 章で一致。
- 公開command 16件（`setup, tools, available, refresh, install, uninstall, installed, use, disable, current, self-update, doctor, repair, exec, completion, version`）。04 §3 / 10 §12.1 / 17 §4.3 で一致。
- shell 6種（`cmd, powershell(5.1), pwsh(7+), bash, zsh, fish`）。01 §2 / 04 §3.1,3.15 / 05 §3 で一致。

### 2.6 registry tree hash（07 §8）

`gdtvm-registry-tree-v1` domain separatorを1回、各regular fileについて `path長(8B BE)｜path bytes｜size(8B BE)｜raw bytes` をpath ASCII昇順で連結したSHA-256（64桁小文字hex）。11・13・14が同一algorithmを参照。矛盾なし。

## 3. 横断整合性の検証結果（すべてPASS）

| 検証項目 | 対象文書 | 結果 |
|---|---|---|
| E_* 30コードが終了コード表へ完全対応 | 04 §7 ↔ 10 §9 | PASS |
| 設定既定値 ↔ hard maximum の整合 | 05 §3 ↔ 14 §16 | PASS |
| tool 17 / helper 2 の件数・ID一致 | 01 §6 ↔ 07 §3 ↔ 12 | PASS |
| command 16件の名称・構文一致 | 04 §3 ↔ 10 §12.1 ↔ 17 §4.3 | PASS |
| shell 6種の集合一致 | 01 ↔ 04 ↔ 05 | PASS |
| 版形式・regex・比較規則 | README ↔ 13 §1.2 ↔ 14 §2 | PASS |
| 非対話承認policyの整合 | 04 §5 ↔ 05 §3.4 ↔ 10 §8 ↔ 11 §4 | PASS |
| conflicts対称性（llvm/mingw/winlibs） | 12 §14,15,20 ↔ 06 §3 | PASS（llvm↔winlibs, mingw↔winlibs 対称。llvm↔mingwは命令名非重複のため非競合） |
| `{{tool}}` テンプレートのruntime限定 | 06 §12 ↔ 15 §2 | PASS |
| checksum `pending` の遅延解決 | 14 §10 ↔ 08 §4 | PASS |
| helper許可step（write-file等禁止） | 14 §14 ↔ 06 | PASS |
| self-update transaction（混在release不可・rollback） | 04 §3.11 ↔ 08 §16 ↔ 11 §2 ↔ 07 §9 | PASS |
| link/backend方式の状態・receipt契約 | 03 §9 ↔ 06 §5.3 ↔ 08 §8.1 ↔ 14 §9 | PASS |
| `disable --all` snapshot（wildcard非保存） | 04 §3.9 ↔ 08 §10.3 | PASS |

## 4. 残存 open item（軽微・非ブロッカー）

先行レビューで実質的な矛盾は解消済み。本監査で新たに確認した残存項目は次の2件で、いずれもW01着手を妨げない。実装者の**暗黙**判断として残さないよう、恒久解決タスクを明示する。

### 4.1 [Low] version/variant のdirectory名encodingの厳密規則（03 §4）

- 内容: 「UTF-8 bytesをpercent-encodingし `[A-Za-z0-9._-]` を保持。Windows予約名・末尾dot/space・`/ \ : NUL` を直接使わない」とあるが、`dot` は保持対象のため**末尾dot**が素の規則では残存し、また**Windows予約名（CON/PRN/…）は英数字のみで保持され残存**する。この2ケースを満たす追加encodingの具体手順が明文化されていない。
- 影響: 17標準toolの版（semver/numeric/jep223/date/long-ID）はいずれもこのケースに該当せず、実害は現状なし。理論上はlocal定義の病的な版/variantでのみ発生。
- 推奨解決（安全側・README §121準拠）: 末尾dot/spaceも percent-encode し、予約名は基底名一致時に決定的markerで曖昧性を除く。恒久対応は **W02-04（PathResolver/encoding実装）** で仕様03 §4へ厳密規則＋fixture＋negative testを追加してから実装する。
- 状態: open（暗黙判断ではなく本レポートで追跡）。

### 4.2 [Cosmetic] 例示 client_version の年月日ズレ

- 内容: 07 §2 の `minimum_client_version = "2026.07.24.00"` に対し、14章の state 例が `client_version = "2026.07.23.00"`。前者を満たすと後者は最小版未満になる。
- 影響: いずれも説明用の例示値であり規範値ではない。動作・schemaに影響なし。
- 推奨: 恒久的に統一する必要はないが、将来のfixture作成（W02-05, W03-11）時に例示版を最小版以上へ揃えると誤読を避けられる。
- 状態: open（cosmetic）。

## 5. 環境事項（仕様の矛盾ではない・実装進行に関わる事実）

W00-01の規範対象外だが、後続タスクの前提として記録する。

1. **Go toolchain**: bootstrap は go1.24.7 だが `GOTOOLCHAIN=auto` かつ proxy 到達可能で `go1.26.5.linux-amd64`（仕様13 §1が指す最新）を取得可能。→ go.mod に `go 1.26.x` を宣言すれば最小要件を充足でき、**buildブロッカーではない**（W00-03/W01で確定運用する）。
2. **CGO**: 環境の既定は `CGO_ENABLED=1`。仕様13 §2はclient buildを `CGO_ENABLED=0` とするため、build時に明示設定する。
3. **Windows固有の実行検証**: 本環境はLinuxコンテナのため、W00-06（Windows CI runner）・W10（Windows標準registry評価）・W11/W12（Windows integration/E2E/品質）・G-WIN-E2E の**実行検証は本環境で不可**。定義・計画・Windows固有コードの作成とplatform-neutral testはWindows段階でも実施可能（16章フェーズゲート但し書き）。Windows実行が必須の合格判定は、Windows環境が使える時点まで保留し、未実施理由と再現commandを証跡へ残す（CLAUDE.md §10）。この制約は利用者へ別途通知する。

## 6. 結論

- 全18文書を精読し、横断的な規範契約（enum・既定値・上限・終了コード・E_*・版形式・収録物・承認policy・tree hash・link/backend契約・self-update transaction）に**矛盾なし**を確認した。
- 実装着手（W01）を妨げる未決事項は**なし**。
- 残存open itemは §4 の2件のみで、いずれも軽微・非ブロッカーであり、恒久解決タスク（W02-04 / 将来fixture）を明示したため、実装者の**暗黙**判断として残らない。
- 環境事項（§5）は仕様矛盾ではなく、Go 1.26要件は本環境で充足可能。Windows実行検証のみ保留対象。

以上によりW00-01の完了条件（未決事項が実装者の暗黙判断として残らない）を満たす。次タスクは **W00-02**（Windows対象matrix固定）。
