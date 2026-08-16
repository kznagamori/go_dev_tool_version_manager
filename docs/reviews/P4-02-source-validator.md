# P4-02 決定記録（2/2）: source validator（P4-02完了）

対象タスク: `docs/13-progress.md` P4-02の2本目。規範仕様は[07-registry-and-tools.md](../07-registry-and-tools.md)§5（source validation 10項目）・§6（標準tool集合と共通規則）・§7〜§10（tool別契約）、[06-tool-definition.md](../06-tool-definition.md)§13-11。

## 1. 着手時の確認事項（1本目の停止記録より）

「第6項が既存の`internal/definition/registry_test.go`のcontract testとどこまで重複するかを先に洗い出す」を最初に行った。

| 既存test | §5との関係 | 扱い |
|---|---|---|
| `TestRegistryToolDefinitionsParse` | 定義がschema 1を通ること | そのまま残す（parser test） |
| `TestRegistryToolDefinitionsMatchSpec` | **§5第6項そのもの**。§7〜§10の表（version_scheme／source種別／artifact source／checksum種別／`strip_components`／required command）を独自の表で持つ | **削除し、validatorへ集約** |
| `TestRegistryDotnetDeclaresWindowsLicenseNotice` | §5第9項と一部重なるが、`license_notice`の有無だけを見る | 残す |
| `TestRegistryDefinitionsDeclareOfficialProvider` | §5.1の`adoption_reason`規則が主。`artifact_kind`が一部重複 | 残す |
| `TestRegistryPythonDeclaresThirdPartyProvider` | 同上（`repository`/`adoption_reason`必須） | 残す |
| `TestRegistryPythonPinsUpstreamDigests` | §6.6・§9.2の固定catalog契約 | 残す |
| `TestRegistryPythonVersionSetsMatchAcrossPlatforms` | **§5第8項**。definition parserが担保する | 残す |

`TestRegistryToolDefinitionsMatchSpec`だけを削除した。**同じ仕様表を2か所に持つと、§7〜§10を変えたときに片方だけが古いままになる。** 削除で失われる検査は無く、validatorの§5-6が同じfieldをより広く（provider license、homepage、tool license、storage kind/scope、lifecycle_mapを含めて）見る。使われなくなった`commandNames` helperも消した。

あわせて`internal/definition/definition.go`のコメント「registry全体のID/alias/command衝突（11）は**P4-01の範囲**である」を実態へ直した。§13-11はP4-01（manifest／tree／load範囲）ではなく本taskの範囲である。

## 2. 判断

### 2.1 §5をfile読取りを持たない純関数として実装した

`ValidateSource(Source) SourceReport`は、registry rootからの相対path → raw bytesのmapを受ける。fileの読取りはApplication Serviceが`port.FileSystem`経由で行う（[02-architecture.md](../02-architecture.md)§1）。P4-01の`ParseManifest`／`CheckTree`／`VerifyDefinitionDigest`と同じ形である。

1件目で止めず全件を集約する。release前検査で1件ずつしか直せないと修正の往復が実用にならない（§13の診断集約と同じ理由）。`SourceFinding`は§5の項番を持ち、`§5-6 tools/go.toml: windows-amd64: strip_components = 0, want 1`の形で出す。

**manifestが読めない場合だけ途中で返す。** definitionとの対応付けができず、以降の項目は無関係な findingを大量に出すだけになる。

### 2.2 OSI承認判定をfail-closed allowlistにした

§5第9項はOSI承認の有無で分岐するが、**OSI承認listそのものが仕様に無い**。SPDXの全listを持ち込むと、承認状態の更新をregistryと無関係に追うことになる。

§6がregistryを標準4 toolへ閉じているため、判定に必要な識別子は`BSD-3-Clause`／`MIT`／`MPL-2.0`（承認）と`LicenseRef-dotnet-library`（非承認）の4件だけである。表に無い識別子は承認/非承認のどちらとも扱わずerrorにした。未知を承認とみなすと制限的licenseが`license_notice`なしで通り、非承認とみなすと正当なOSS licenseへ不要な承認要求が出る。

判定は**両方向**を見る。`license_notice`があるのにOSI承認licenseである場合も報告する。「OSS licenseのplatformへ宣言しない」（[06-tool-definition.md](../06-tool-definition.md)§5）——不要な承認要求は、本当に承認が要る場面の重みを下げる。

### 2.3 checksum algorithmの宣言場所をtoolごとに分けた

当初は全toolへ`checksum.algorithm`を要求する表を書いたが、実定義と一致しなかった。**providerが公開したalgorithmがdefinitionのどこに現れるかは、sourceの形で変わる。**

| tool | algorithmの在処 |
|---|---|
| Go / .NET SDK | asset側にalgorithm fieldが無いため`checksum.algorithm`で宣言 |
| Python | static assetごとの`digest_algorithm`（§9.2） |
| Node.js | `checksum.line_format`（`SHASUMS256.txt`の行形式）が決める（§8.1） |

宣言の無い場所へ既定値を補うと、providerが公開したalgorithmとdefinitionの宣言が一致しているかを検査できなくなる。表を3 fieldへ分け、宣言する場所ごとに突き合わせた。

### 2.4 到達しない検査を残さなかった

実装中に2件、definition parserが先に拒否するため到達しない分岐が見つかった。

- **`lifecycle_map`を宣言しないtoolがmapを持つ場合**: `lifecycle_map`は`kind=json-index`でだけ書けるkeyであり、標準4 toolでjson-indexなのは.NET SDKだけである。definitionが`definition.kind_key_forbidden`で拒否する。registry側の分岐を削除した。
- **required commandを減らす方向**: probeが`runtime_command`で参照しているため、commandを消すとdefinition自体が不正になる。§7.2の「完全な集合」は過剰側でも破れるので、testはそちらで検査する。

同様に、`version_scheme`・`line_format`・storage IDは表に持つが、実定義で変えるとdefinition parseが先に落ちるため単独の不一致を作れない。これらは合成した`definition.Platform`／`definition.Definition`をcheck関数へ直接渡して突き合わせを検査した。

## 3. §5各項目の担当

| 項 | 内容 | 実装 |
|---:|---|---|
| 1 | directory/entry集合が§2と一致 | `CheckTree`（P4-01） |
| 2 | TOML/message/licenseがsize上限内 | `checkSizes`＋`ParseMessageCatalog` |
| 3 | ID/file/path/digest/schemaが一致 | `ParseManifest`＋`VerifyDefinitionDigest`＋`definition.Parse` |
| 4 | aliasが4 tool全体で衝突しない | `checkAliasCollisions` |
| 5 | platform tupleが2件だけ | `checkPlatformTuple` |
| 6 | §7〜§10の表と一致 | `checkToolContract`／`standardTools` |
| 7 | Python third-party license textが存在 | `ValidateSource` |
| 8 | static sourceのversion集合が両platform一致 | `definition.Parse`（`platform_version_set_mismatch`）。§5-3として報告 |
| 9 | `license_notice`とOSI承認識別子の対応 | `checkLicense`／`osiApprovedLicenses` |
| 10 | `lifecycle_map`がupstream enum全値を明示 | `checkLifecycleMap` |

第10項は**過不足の両方**を見る。余分な値を許すと、upstreamが廃止したphaseの写像が残っていることに気付けない。

第4項はtool IDとaliasを同じ名前空間として扱う。あるtoolのaliasが別のtoolのIDと同じだと`use <name>`がどちらを指すか決まらない。definition単体では他のdefinitionを見られないため、registry全体を見るここで行う（§13-11）。

## 4. 検査が固定したこと

| 検査 | 対象 |
|---|---|
| `TestValidateSourceAcceptsRepositoryRegistry` | repositoryのregistryが§5の10項目をすべて満たすこと |
| `TestValidateSourceReportsEveryCheck` | 項番1〜7・9・10それぞれが実際に不適合を検出すること（9件） |
| `TestValidateSourceCoversEveryCheckNumber` | 10項目すべてに担当があること（第8項の委譲先を明示） |
| `TestValidateSourceChecksEveryContractField` | §7〜§10の表のfieldごとの不一致（9件） |
| `TestCheckPlatformContractReportsEveryMismatch` | 合成platformでの突き合わせ（7件）と、契約どおりなら無findingであること |
| `TestValidateSourceRejectsUnknownLicenseIdentifier` | 判定表に無い識別子を承認/非承認のどちらとも扱わないこと |
| `TestValidateSourceRejectsLicenseNoticeOnOSSLicense` | OSI承認licenseへの`license_notice`宣言（第9項の逆向き） |
| `TestValidateSourceRejectsExtraLifecycleMapEntry` | upstream enumに無い写像が残っていること |
| `TestValidateSourceStopsWhenManifestUnreadable` / `Broken` | manifest欠落・parse失敗で以降を実行しないこと |
| `TestStandardToolsCoverManifest` | 契約表が標準4 toolと一致し、各toolが2 platformの契約を持つこと |

## 5. 検証

すべてLinux containerで実行した。

| command | 結果 |
|---|---|
| `gofmt -l .` | 出力なし |
| `go build ./...` / `go vet ./...` | 成功 |
| `go test ./... -race -shuffle=on -covermode=atomic` | 全package成功。`internal/registry` 98.9% |
| `python3 scripts/ci/check_messages.py` | 成功（catalog key 85／source参照ID 85） |
| `check_policy.py` / `check_imports.py` / `check_docs.py` / `check_licenses.py` | すべて成功 |
| `check_pr_refs.py` / `git diff --check` | 成功・出力なし |

`internal/registry`で覆えていないのは6箇所で、いずれもdefinition parserが先に拒否するため実定義から到達できない不一致分岐（manifestとdefinitionのtool ID食い違い、platform IDの並び違い、size検査の対象外path）と、`describeDecodeError`の最終fallbackである。

## 6. 未実施・制約

- **§5はrelease workflowへ接続していない。** `ValidateSource`は純関数として用意したが、release前に実行する経路（[11-quality-and-ci.md](../11-quality-and-ci.md)§13のrelease工程）は未接続である。現状はCIの`unit` jobが`TestValidateSourceAcceptsRepositoryRegistry`としてrepositoryのregistryを常時検査する。
- **§12のlive metadata smokeは実装していない。** 第10項の「live smokeで未知値が出ないこと」はupstreamへ接続する検査であり、release workflowの範囲である。本taskが実装したのは静的側（`lifecycle_map`がupstream enum表と一致すること）だけである。
- §5第2項のlicense file size上限（1本目から継続）と§2のfile名grammarは仕様側の未決事項である。
- `python.toml`の`lifecycle = "unknown"`はP3-04から継続。release前に`devguide.python.org`へ到達できる環境で確認する。
- 標準4 tool以外のtoolを足す場合、`standardTools`と`osiApprovedLicenses`の両方を[14-maintenance.md](../14-maintenance.md)の手順で更新する必要がある。表に無いtool IDとlicense識別子はfail closedで拒否される。
