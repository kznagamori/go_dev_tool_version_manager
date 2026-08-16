# P4-01 決定記録: registry manifestとload範囲

対象タスク: `docs/13-progress.md` P4-01。規範仕様は[07-registry-and-tools.md](../07-registry-and-tools.md)§2（exact tree）・§3（`registry.toml`）・§4（runtime load）、[04-storage-and-data.md](../04-storage-and-data.md)§21（size上限）、[11-quality-and-ci.md](../11-quality-and-ci.md)§2（client version）。

## 1. 着手時の確認事項（P3-04停止記録より）

P3-04の停止記録は「`registry/registry.toml`のmanifestが持つfile digestを標準4 toolのdefinitionから生成する手順と、CIでdefinition変更時にmanifestを同期させる仕組みを確認する」を着手時の確認事項として残していた。

**別のscriptやgenerator commandを足さず、testで同期を強制する形にした。** `TestRepositoryManifestMatchesDefinitions`が`registry/registry.toml`をparseし、各entryの`path`が指す実fileを読み、`VerifyDefinitionDigest`で照合する。definitionを変えてmanifestを更新し忘れると`unit` jobが両OSで落ちる。

生成用のscriptを別に持たない理由は3つある。

1. digestの計算は`DefinitionDigest`（`crypto/sha256`）1行であり、生成側と検証側で別実装を持つ意味がない。
2. scriptを足すと、それ自体が§2のexact treeへ入らない「helper」になる。§2は「helper、key、script、local bundle directoryは存在しない」と定める。repository側の`scripts/ci/`へ置くことはできるが、更新を強制する力はtestの方が強い。
3. 更新手順は「test failureのmessageが出す実digestをmanifestへ書く」で足りる。errorは`manifest <期待> / 実file <実際>`の両方を出す。

## 2. 判断

### 2.1 `ClientVersion`をdomainへ置いた

§3の`client_min_version`/`client_max_version`は大小比較を要する。[11-quality-and-ci.md](../11-quality-and-ci.md)§2が「比較は4個の10進整数tuple。SemVerではなく、SemVerへ変換せずprerelease/build suffixを付けない」と定めるため、`domain.Version`（semver/go/python scheme）とは別の型にした。

grammarに加えて**実在日付を検査する**。`2026.02.30.00`は§2のgrammarを満たすが存在しない日付であり、日付として比較する意味を持たない。`time.Date`が範囲外の日をnormalizeする性質を使い、正規化後と一致するかで判定する。

`internal/store`は既に`clientVersionRe`を持つが、そちらは§7のstate fileが持つ文字列の書式検査であり、比較を必要としない。統合は`internal/store`がdomain型を使う形になり、P2-04で確定したcodecの契約に触れる。P4-01の範囲を超えるため行っていない（§5に記載）。

### 2.2 `path`を`tools/<id>.toml`固定として検査した

§3は「path=`tools/<id>.toml`」と定める。任意pathを許すと、manifestがregistry treeの外のfileを指せる。`buildTool`で`id`から期待pathを組み立てて完全一致を要求し、`../go.toml`のような入力を通さない。

### 2.3 `client_max_version < client_min_version`を拒否した

§3は範囲の向きを明示していないが、maxがminより小さい宣言はどのclientも読めないregistryを表す。registryを作る側の書き間違いとして扱い、parse時点で拒否する。**parserの寛容なfallbackではなく、上位の不変条件として足した判断である。**

### 2.4 command別load範囲を表で持った

§4はcommandごとに検証範囲を定める。表（`scopes`）で持ち、未知commandは既定値を返さずerrorにした。既定値で「registryを読まない」を返すと、範囲を決め忘れたcommandが検証なしで動く。

`CommandCount = 10`はv0.1の9 commandに`shim runtime`を加えた数である。件数を定数にすることで、commandを足したときにtestが範囲の定義漏れへ気付く。

`--help`/`<command> --help`/`--version`はcommandではなくflagのため表へ入れていない。§4が`version`と同じ扱いを定めており、registryを読まない点で差がない。

`installed|current|uninstall`は§4が「state、receipt、indexを正本とする。正規tool IDならregistryなしで扱い、alias入力を正規化するときだけ対象definitionを要求する」と定める。aliasかどうかは入力を見るまで決まらないため、scopeとしては読まない側へ置き、alias正規化が必要になった時点で呼出し側が`available`相当の検証を行う形にした。この分岐はP6のcommand実装で使う。

### 2.5 `registry-v1.json`は補助成果物として書いた

§5が「schema JSONはTOML parser/semantic validatorの補助成果物であり、JSON Schemaだけで適合を宣言しない」と定める。JSON Schemaはclient version範囲の照合、definition fileのdigest照合、§2のexact tree検査を表現できない。`description`へその旨を書き、`TestSchemaJSONIsAuxiliary`で固定した。

`tool-definition-v1.json`と同じく、key集合・固定値・正規表現・`additionalProperties: false`をGo側とtestで同期させている。片方だけにkeyを足すと、strict parserが通すmanifestをschema JSONが落とす（またはその逆）状態になる。

## 3. 検査が固定したこと

| 検査 | 対象 |
|---|---|
| `TestParseManifestAcceptsSpecExample` | §3の正規例が通り、maxが任意であること |
| `TestParseManifestRejects` | unknown key、schema不一致、CalVer違反、path不一致、digest表記、ID grammar、ID順（16件） |
| `TestParseManifestRejectsEmptyValues` | 必須keyの欠落と空文字を別々に拒否すること（9件） |
| `TestParseManifestRejectsMalformedTOML` | 型違い、syntax error、key重複（6件） |
| `TestParseManifestRejectsMissingToolsTable` | `[[tools]]`が1件も無いmanifest |
| `TestParseManifestRequiresExactlyFourTools` | 3件・5件・ID重複 |
| `TestParseManifestRejectsOversizedFile` | 上限ちょうどは通し、1 byte超過をsize検査で拒否すること |
| `TestParseManifestRejectsInvertedRange` | max < min |
| `TestCheckClientVersion` | 下限ちょうど/未満、上限ちょうど/超過、未設定 |
| `TestCheckTree` | §2のexact treeと欠落/余分/両方 |
| `TestVerifyDefinitionDigest` | 1 byte違いの検出 |
| `TestScopeFor` / `TestLoadScopeReadsRegistry` | §4の10 command全件と未知command |
| `TestRepositoryManifestMatchesDefinitions` | repositoryのmanifestが実digestと一致すること |
| `TestRepositoryRegistryHasNoExtraEntries` | §2に無いentryがregistryへ混ざっていないこと |
| `TestParseClientVersion*` | §2のCalVer grammar 27件の拒否と実在日付、4整数tupleの全順序 |
| `TestSchemaJSON*` | schema JSONとstrict parserのkey集合・固定値・正規表現の一致 |

## 4. 検証

すべてLinux containerで実行した。

| command | 結果 |
|---|---|
| `gofmt -l .` | 出力なし |
| `go build ./...` | 成功 |
| `go vet ./...` | 成功 |
| `go test ./... -race -shuffle=on -covermode=atomic` | 全package成功。`internal/registry` 99.4%、`internal/domain` 97.4% |
| `python3 scripts/ci/check_policy.py` | 成功（production Go file 118件） |
| `python3 scripts/ci/check_imports.py` | 成功（package 20件／internal import 151件） |
| `python3 scripts/ci/check_docs.py` | 成功（48 file） |
| `python3 scripts/ci/check_licenses.py` | 成功（module 14件） |
| `git diff --check` | 出力なし |

`internal/registry`で覆えていないのは`describeDecodeError`の最終fallback 1行だけである。`toml.StrictMissingError`でも`toml.DecodeError`でもないdecode errorを作る方法が無く、libraryの契約が変わった場合に備えた防御である。

## 5. 未実施・制約

- **§2のexact treeはまだrepositoryで完全一致しない。** `messages/ja.toml`と`licenses/python-build-standalone-MPL-2.0.txt`はP4-02の範囲であり、この時点では存在しない。`TestRepositoryRegistryHasNoExtraEntries`は余分の検出だけを行い、完全一致は行っていない。P4-02でtree全体が揃った時点で`CheckTree`へ切り替える。
- **§5のsource validation 10項目は実装していない。** P4-02の範囲である。P4-01が実装したのは§2のtree検査関数、§3のmanifest parser、digest照合、§4のload範囲であり、release前検査としての組み立ては行っていない。
- **registry rootからのfile読取りはportを通していない。** P4-01が公開するのは`ParseManifest`（byte列）、`CheckTree`（path集合）、`VerifyDefinitionDigest`（byte列）で、いずれも外部作用を持たない純関数である。実際の読取りはApplication Serviceが`port.FileSystem`経由で行う。`_test.go`だけがrepositoryの実fileを`os.ReadFile`で読み、manifest同期を検査している。
- **`internal/store.clientVersionRe`と`domain.ClientVersion`が重複している。** 前者は§7のstate fileの書式検査、後者は§3の範囲比較で、用途が違う。統合はP2-04で確定したcodecの契約に触れるため行っていない。
- `python.toml`の`lifecycle = "unknown"`はP3-04から引き続き未解決である。release前に`devguide.python.org`へ到達できる環境で確認する。
