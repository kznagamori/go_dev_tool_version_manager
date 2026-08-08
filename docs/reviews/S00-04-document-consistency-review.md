# S00-04 文書整合性レビュー

## 1. 目的と対象

W00-01/S00-03後の現行仕様を再レビューし、標準toolへ追加された.NET SDKがGo、Node.js、Pythonと同じ契約軸で記述されていること、ならびに文書再構成後の文書内・文書間不整合がないことを確認・是正した記録である。

対象は[docs/README.md](../README.md)と[01](../01-requirements.md)～[15](../15-deferred.md)の現行16文書、rootの`AGENTS.md`/`CLAUDE.md`、歴史記録[W00-01](W00-01-specification-audit.md)。baselineは`main` commit `f48df04`、開始時worktreeはclean。環境はWindows 10.0.26200 x64、PowerShell 7.6.4、Go 1.26.5 windows/amd64。

本書は監査証跡であり規範仕様ではない。実装時は[docs/README.md](../README.md)が指定する番号付き仕様を正とする。

## 2. 結論

- .NET SDKはprovider/version source、artifact、checksum、archive layout、license、command、environment、storage、probe、両platform差、contract test、公開文書要件、進捗taskの全軸で他3 toolと同じ構造へ揃えた。
- `.NET SDK=SHA-512`を一般的なSHA-256処理へ誤って流さないよう、upstream digestを`sha256|sha512`、gdtvm内部identityをSHA-256固定として分離した。
- 全体監査で、Plan、CLI JSON、warning、doctor、path、bootstrap、registry読込み、strict definition schemaに残っていた不整合を同期修正した。
- 現時点で文書整合性を妨げるblockerはない。.NET definition実体と両OS実測は後続のP10-04/P10-06で行うため、実装済みとは扱わない。

## 3. 標準4 toolの同等性

| 契約軸 | Go | Node.js | Python | .NET SDK |
|---|---|---|---|---|
| provider | official | official | third-party（Astral） | official（Microsoft） |
| version source | JSON | JSON＋checksum text | static | JSON index＋子文書 |
| version scheme | go | semver | python | semver |
| upstream digest | SHA-256 | SHA-256 | SHA-256 | SHA-512 |
| archive/layout | zip/tar.gz、strip 1 | zip/tar.gz、strip 1 | tar.gz、strip 1 | zip/tar.gz、strip 0 |
| required command | go/gofmt | node/npm/npx | python/python3/pip/pip3 | dotnet |
| storage | tool共有＋一部version別 | tool共有＋global packageはversion別 | tool共有＋user packageはversion別 | NuGet cache共有＋CLI homeはversion別 |
| 特別なPlan警告 | なし | なし | third-party | Windows配布物の制限的license |
| 両OS | windows-amd64 / linux-amd64-glibc | 同左 | 同左 | 同左 |

.NET固有の差はtool IDによるGo分岐ではなく、[definition schema](../06-tool-definition.md)と[標準tool契約](../07-registry-and-tools.md)で表現する。親releaseの`release-date`継承、SHA-512、Windows `license_notice`、strip 0、`DOTNET_ROOT`/NuGet関連storage、`dotnet --version`/`--list-sdks` probeをcontract test対象にした。

## 4. 発見事項と修正

### 4.1 .NET SDK

- upstream digestをSHA-256固定としていた箇所を、provider指定`sha256|sha512`の1-pass検証へ修正した。内部receipt/index/release identityのSHA-256固定とは分離した。
- `releases[].release-date`が`sdks[]`の親にあるため、`item_parent_published_at_pointer`を追加して各SDKへUTC公開日時を継承するようにした。
- artifact size非提供は既存の`size=0`＝unknown契約で扱い、0 byteと表示しないことを明記した。
- Windows配布物の`LicenseRef-dotnet-library`と明示承認、LinuxのMITを維持した。
- `dotnet tool install --global`は`DOTNET_CLI_HOME`だけで隔離できると推測せず、P10-06の両OS測定まで管理外とした。

### 4.2 definition・registry

- 未使用だった`numeric` version scheme、`default_channel`、platform `supported`、storage `mutable`、install `include|exclude`、`notes`等をschema 1から除いた。archive entry filterはD-22へ移した。
- `strip_components`を標準4 toolが使うinteger `0|1`へ閉じ、2階層以上をv0.1標準registryへ採用しないことを明記した。
- `channel_pointer`をstring exact `stable|prerelease`またはboolean `true→stable`/`false→prerelease`へ固定した。
- shim indexの1 commandをexactly 1 toolへ対応させ、`tool_ids[]`を`tool_id`へ変更した。
- command別のregistry読込み範囲を明示し、shim runtimeはregistry/networkを読まず、client version不一致indexをfail closedにした。
- project fileへはcanonical tool IDだけを書き、aliasはCLI境界で正規化するよう統一した。

### 4.3 Plan・CLI JSON・path

- Planへ`client_version`、`invocation_id`、`receipt_index_revision`、typed `reads[]`、download/extract/probe/write/storage/rollbackのexact契約を追加した。
- setup固有情報をtool summaryへ混在させず、operation排他の`SetupPlan`へ旧/新mode/root、filesystem/link方式、shim、integration、backup、再起動要否として保持した。
- 任意helper/backend processを除き、外部実行はdefinition-declared probeだけとした。probeの実行file、version、source、digest、license、reason、argv、cwd、書込み先をPlanへ固定した。
- argv内のpathも裸のstringにせず、`PlanArg(literal|path)`と`PathValue`で表すようにした。
- `PathValue {role,path}`と22個の`path_role`をPlan、CLI JSON、typed error、doctor reportで共通化し、roleごとの対象を固定した。
- `InstallSummary`からreceiptに存在しないchannel/lifecycleを削除し、`SelectionSummary.command_path`を複数commandと矛盾しない`payload_path`へ変更した。
- Plan例はkey形状を示す構造例であり、空のoperation配列を持つため実行可能positive Planではないことを明記した。

### 4.4 warning・doctor・bootstrap

- 事前承認用`PlanWarningCode` 8件と、処理結果用`ResultWarningCode` 5件を分離した。
- doctorを10個の閉じた`Diagnostic.code`、`error|warn|info` severity、`healthy|degraded|unhealthy`集約へ固定した。unhealthyは診断成功JSONを返しつつexit 12、healthy/degradedはexit 0とした。
- bootstrap再実行は検証済みstagingからactive distributionを置換する。開発版では旧distributionをbackupせず、自動rollbackもしない。旧版へ戻す場合は開発利用者が完全versionを指定して再取得する。
- tool payload/state/storageはdistribution置換から除外し、利用者の任意`gdtvm.toml`はraw bytesで新distributionへ引き継ぐ。commit失敗時だけ設定copyの復旧pathを報告する。

## 5. 利用者判断

各項目は1問ずつ確認した。採用案と比較対象を以下に残す。

| 項目 | 採用した案 | メリット | デメリット | 他の選択肢 | 他案のメリット | 他案のデメリット |
|---|---|---|---|---|---|---|
| .NET global tool | **B: P10-06で両OS実測するまで管理外** | 公式契約のない隔離を推測せずfail closed | 実測完了までgdtvm管理外 | A: 今すぐtyped storage化 | 早く隔離できる可能性 | 実配置と違えば利用者dataを見失う |
| InstallSummary | **A: channel/lifecycleを削除** | receiptだけから再現できる | installed一覧では当該値を表示しない | B: receiptへ保存 | 表示情報が増える | 永続schemaとmigrationが増える |
| bootstrap更新 | **利用者指定: backup/自動rollbackなし** | 開発版の実装と試験を簡潔にできる | 失敗時は旧版の再取得が必要 | 旧distributionを1世代保持 | 即時rollback可能 | self-update前に複雑な更新stateを持ち込む |
| current path | **A: payload_path** | 複数command toolでも一意 | command実体は別途receipt参照 | command_pathを1件返す | 直接実行先が見える | primary commandを一意に選べない |
| warning型 | **A: Plan/Resultを分離** | approval対象と事後警告を型で区別 | enumが2種類になる | 単一warning enum | 型数が少ない | 事後警告を承認対象と誤解しやすい |
| doctor status | **A: 3状態** | warnとerrorを区別できる | booleanより分岐が1つ増える | healthy boolean | 単純 | degradedを表せない |
| doctor exit | **A: unhealthyだけexit 12** | scriptが診断失敗を検出しつつ詳細JSONを得られる | `ok=true`でもnonzeroという専用規則が必要 | 全診断完了をexit 0 | 一般的なJSON成功規則 | unhealthyを別途JSON解析しないと検出できない |
| .NET published_at | **A: 親日時継承fieldを追加** | upstream日時を正確に保持 | schema fieldが1つ増える | 空にする | schema変更不要 | 提供済み日時を失う |
| Plan read path | **A: reads[]を追加** | 全入力path/digestをstale検査できる | Planが長くなる | inputsへ個別field追加 | 対象ごとに直接参照可能 | field追加が読み物ごとに増える |
| 公開path | **A: 共通PathValue** | mask・CI範囲検査・JSONを同じroleで扱える | objectが増えて冗長 | 裸stringまたは型ごとのpath | 短い | role判定が表示側の推測になる |
| Diagnostic.code | **A: 10件の閉じた集合** | 常に同じ診断軸を機械比較できる | 新項目追加にschema変更が必要 | 自由なcode集合 | 拡張が容易 | 欠落・表記揺れを検出できない |
| channel_pointer | **A: string/booleanを固定写像** | Go booleanと通常stringを追加fieldなしで扱える | booleanの意味をschema規約として覚える必要 | definitionごとのmap | 任意upstream表現に対応 | 未使用の汎用機構が増える |
| setup Plan | **A: SetupPlan専用object** | operation固有情報が明確でtool summaryを汚さない | Plan構造が1つ増える | B: summaryへ追加 / C: reads・writesから導出 | Bは平坦、Cはfield追加不要 | Bは空field多数、Cはmode/能力を一意に復元できない |

## 6. 検証

| 検証 | 結果 |
|---|---|
| `git diff --check` | PASS |
| Markdown相対linkとanchor | 169件、failure 0 |
| linked `§`参照と対象level-2章 | 54件、failure 0 |
| code fence balance | marker 150件、偶数、failure 0 |
| JSON / JSON Lines parse | JSON 5件、JSON Lines 1件、failure 0 |
| TOML fence parse（`tomlv`） | 24件、failure 0 |
| error/warning/diagnostic/path-role/SetupPlan件数 | error 34、Plan warning 8、Result warning 5、diagnostic 10、path role 22、SetupPlan field 15。全て期待値一致 |
| stale用語・削除済みfield検索 | 現行規範16文書＋`CLAUDE.md`で0件 |

文書のみの変更であり、Go source、registry TOML実体、CI workflowはまだ存在しないため`go test`と両OS CI matrixは本taskでは対象外。P0以降の各taskで仕様どおり実施する。

## 7. 一次資料

- [Microsoft Learn: .NET SDK/CLI environment variables](https://learn.microsoft.com/en-us/dotnet/core/tools/dotnet-environment-variables)
- [Microsoft Learn: .NET global tools](https://learn.microsoft.com/en-us/dotnet/core/tools/global-tools)
- [dotnet/core: .NET distribution license information](https://github.com/dotnet/core/blob/main/license-information.md)
- [Microsoft .NET release metadata index](https://builds.dotnet.microsoft.com/dotnet/release-metadata/releases-index.json)
- [Microsoft .NET 8 releases metadata sample](https://raw.githubusercontent.com/dotnet/core/main/release-notes/8.0/releases.json)

## 8. 後続task

- P10-04: `registry/tools/dotnet.toml`実体、fixture、schema/contract testを実装する。
- P10-06: `dotnet tool install -g`の配置をWindows/Linuxで実測し、管理範囲を確定する。
- 次に開始する全体taskはP0-01（CI matrix workflowの骨格）とする。
