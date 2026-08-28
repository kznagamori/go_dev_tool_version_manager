# P6-01 決定記録（1/2）: catalog cacheと版解決

対象タスク: `docs/13-progress.md` P6-01の1本目。規範仕様は[04-storage-and-data.md](../04-storage-and-data.md)§15・§17.2・§21、[08-install-runtime.md](../08-install-runtime.md)§3.1・§3.2、[03-cli.md](../03-cli.md)§3.2、[02-architecture.md](../02-architecture.md)§2・§3・§14、[10-security.md](../10-security.md)§6。

## 1. 着手時の確認事項（P5-04の停止記録より）

2件とも解決した。

### 1.1 refreshの入口は`internal/catalog`

[02-architecture.md](../02-architecture.md)§2が`internal/catalog`へ「配布元照会、版正規化、channel/lifecycle判定、**カタログcache**」を割り当てている。cacheの読み書き方針はその責務そのものである。

| 層 | package | 持つもの |
|---|---|---|
| codec | `internal/store` | §15のJSON表現、`ParseCatalog`／`EncodeCatalog`、順序検査 |
| cache方針 | `internal/catalog` | 保存path、鮮度判定、atomic保存、不一致時の扱い |

§15の保存path `cache/catalogs/<tool-id>/<platform-id>.json` を組み立てるため、import表へ`internal/security`を追加した。[04-storage-and-data.md](../04-storage-and-data.md)§6のlogical rootからのpath組立て（`security.Join`）を使う。path検査規則を複製すると、規則を変えたときに片方だけが古いままになる。

roleは`catalog`とする。data root配下だが、§17.2が同roleを「tool/platform catalog JSON」と定め「最も具体的なroleを使う」としている。

### 1.2 利用者判断: 仕様未記載の上限5件を§21へ昇格した

P3-01が仕様に無いまま導入し、進捗台帳へ「利用者判断で仕様へ昇格させるか決める」と残していた5件である。P6-01が`cache_ttl`を実際に使う時点で決着させた。

| 案 | 内容 | 採否 |
|---|---|---|
| A | 5件すべてを§21へ昇格 | **採用** |
| B | `cache_ttl`だけ昇格 | 不採用。同じ性格の上限が仕様にあるものと無いものへ分かれ、次に触るtaskで同じ判断を繰り返す |
| C | 上限を撤廃 | 不採用。`cache_ttl = "10000h"`のような定義を通し、期限切れcatalogを事実上永続利用できる |

[04-storage-and-data.md](../04-storage-and-data.md)§21へ次を追加した。**実装は変えていない。** 仕様を実態へ同期させただけである。

| 対象 | 上限 |
|---|---|
| version source `cache_ttl` | 1分〜30日 |
| JSON pointer | 255 byte |
| `version_regex` | 1024 byte |
| SPDX expression | 128 byte |
| URL hostname | 253 byte |

定数と§21の値が食い違えば落ちるcontract test（`TestSpecifiedLimitsMatchSpec`）を置いた。あわせて、hostnameの`253`がliteralだったので`hostMaxBytes`として名前を付けた。名前が無いと、この検査が何を指しているのか実装から辿れない。

## 2. 判断

### 2.1 cacheの異常はmissingとして返す

読めない、parseできない、tool/platform/definition digestが一致しない——これらをすべて`FreshnessMissing`にし、errorにしない。

cacheは再取得できる派生dataである。壊れたcacheでoperationを止めると、利用者は手でfileを消すしかなくなる。一方でnetworkの失敗はcacheでは埋められないため、そちらは呼出し側（2本目）が扱う。

§15の「catalogはcacheであり**definition/platform不一致時に利用しない**」をこの形で満たす。definitionが変わればartifact templateもpointerも変わりうるため、同じversion一覧でも中身が古い。

`DefinitionSHA256`が空の呼出しはdigest一致検査を省く。registry読込み前の診断など、definitionを持たない経路のためである。

### 2.2 期限を持たないcatalogは常にfresh

§15「static sourceは`expires_at=null`を許す」。上流を見に行っても内容が変わらないため、期限の無いcatalogを期限切れ扱いしない。

期限ちょうど（`now == expires_at`）はstaleとする。`expires_at`は「この時刻まで有効」ではなく「この時刻に切れる」と読むのが自然で、境界をfresh側へ倒すと1回ぶん余計に古い内容を使う。

### 2.3 exactは`VersionText`のbyte完全一致

[08-install-runtime.md](../08-install-runtime.md)§3.1手順3「入力をtrim、補完、range展開せず、catalogの正規version文字列と**byte完全一致**で探す」。[02-architecture.md](../02-architecture.md)§3も「入力一致はcomparison keyではなく、catalogに保存された正規完全versionのbyte完全一致とする」と定める。

comparison keyで比べると`1.0`と`1.0.0`のように表記の違うものが同じversionとして通る。`Version`ではなく`VersionText`を見るのは、後者がschemeの有無によらず常に入るためである（CLI JSON envelopeから読んだitemは`Version`がzeroになる）。

### 2.4 latestは先頭候補を採り、同順位複数を拒否する

§15がcatalog itemを「version comparison降順、同値ならversion byte順」に並べ、`ParseCatalog`がその順序を検査済みである。先頭から条件を満たす最初のitemが最大になる。

§3.2「候補0件・比較不能・**同順位複数**は失敗する」。2件目が同順位なら「最大が1件に決まらない」ということなので、どちらを選ぶかを暗黙に決めない。

**lifecycle=unknownは候補に含める。** §3.1が「lifecycle=unknownは状態を明示するが**EOLと断定しない**」と定める。除外すると、上流がlifecycleを公開していないtoolの`--latest`が常に失敗する。選んだ場合は呼出し側がPlanへ状態を表示する（§3.2）。

### 2.5 一覧は並べ替えない

[03-cli.md](../03-cli.md)§3.2「完全versionをversion降順で…**常に全件表示する**。channel/lifecycleで絞り込むoptionはv0.1に存在しない」。`installable=false`も理由付きで含める。

保存順のまま返すのは、§15の順序を`ParseCatalog`が検査済みだからである。ここで並べ直すと、順序検査を通っていない並びを表示することになる。

## 3. 仕様の空白を1件、判断で埋めた

**exact指定で当たったversionが`installable=false`の場合のerror codeを§3.1が明示していない。** 同§は「platform artifactがないversionは理由付き`installable=false`として**表示する**」とだけ定め、exactで当たったときの扱いを書いていない。

versionは見つかっているため`E_VERSION_NOT_FOUND`ではなく、「このversionについてこのplatformが対象外」を表す`E_PLATFORM_UNSUPPORTED`とした。[03-cli.md](../03-cli.md)§7の終了codeはどちらも4であり、利用者が見る終了codeは変わらない。判断であることを`resolve.go`のコメントと本記録へ明示し、別の扱いが望ましければ直せるようにした。

## 4. 検査が固定したこと

| 検査 | 対象 |
|---|---|
| `TestCachePathFollowsSpecLayout` | §15の保存pathを両OSの区切りで、roleが`catalog`であること |
| `TestCachePathRejectsMissingInput` | data root／tool／platform／hostの未設定 |
| `TestSaveAndLoadCacheRoundTrips` | 保存と読取りの往復、fileが`0600`であること |
| `TestLoadCacheJudgesFreshness` | 期限内／期限ちょうど／期限切れ／期限なし（static） |
| `TestLoadCacheTreatsMismatchAsMissing` | tool／platform／definition digestの不一致 |
| `TestLoadCacheTreatsUnreadableAsMissing` | file無し／壊れたJSON／schema違い／読取り失敗 |
| `TestLoadCacheSkipsDigestCheckWhenUnspecified` | digest未指定時の扱い |
| `TestSaveCacheReportsFailure` | mkdir／書込みの失敗注入 |
| `TestParentDirHandlesBothSeparators` | 両OSの区切りとfilesystem root |
| `TestResolveExactMatchesVersionBytes` | 部分版・接頭辞・前後空白・余分な要素の6件を拒否 |
| `TestResolveExactRejectsUninstallableVersion` | `installable=false`の扱い |
| `TestResolveLatestPicksMaxStableNonEOL` | prerelease／`installable=false`／EOLを飛ばして最大を選ぶこと |
| `TestResolveLatestAcceptsUnknownLifecycle` | unknownを候補に含めること |
| `TestResolveLatestFailsWithoutCandidate` | 候補0件の4形 |
| `TestResolveLatestRejectsTiedMaximum` | 同順位複数 |
| `TestListAvailableReturnsEveryItem` / `TestListAvailableIsImmutable` | 全件・保存順・理由付き、返したsliceの独立性 |

## 5. 検証

Linux containerで実行した（Go 1.26.6）。両OSの判定はCI matrixで行った。

| command | 結果 |
|---|---|
| `gofmt -l .` / `go build ./...` / `GOOS=windows go build ./...` | 出力なし・成功 |
| `go vet ./...` / `GOOS=windows go vet ./...` | 成功 |
| `go test ./... -race -shuffle=on -covermode=atomic` | 全package成功。`internal/catalog` 89.1% |
| `check_policy.py` / `check_imports.py` / `check_docs.py` / `check_licenses.py` / `check_messages.py` | すべて成功 |
| `git diff --check` | 出力なし |
| CI（PR #115） | 12 check成功 |

## 6. 未実施・制約

- **2本目の範囲が残っている。** source kind（`static`／`json`／`json-index`）ごとの取得orchestration、`--refresh`の扱い、offline時の`E_CATALOG_MISSING`、offline exactでの`W_CACHE_STALE`付きstale利用（§15「`--latest`は期限切れを黙って使わない」）は`claude/feature-p6-01-catalog-fetch`で実装する。`Resolution.Stale`はそのための入れ物で、値を立てるのは2本目である。
- **cache層と解決層をまだ誰も使っていない。** 呼出し側はP6-02のPlanとP8のCLI adapterである。
- **P5-04から継続する未実装**: Registry portと`LinkManager` portの記録wrapper（P7）、§7.2のE2E照合（P6以降）、`port.FileSystem`／`port.Environment`のproduction実装（P8-01）。
- **P5-03から継続する未決が1件**（`./`始まりのtar entryを§5に従って拒否しており、標準4 toolの実archiveを繋ぐP6で該当があれば仕様側で扱いを決める）。**仕様側の未決が2件継続**（§5第2項のlicense file size上限が§21の表に無い、§2の「license file名はASCII kebab grammar」が§2自身の例と食い違う）。`python.toml`の`lifecycle = "unknown"`はP3-04から継続。source error専用のerror code（`E_DEFINITION_INVALID`）、version数値要素の64 bit上限、`logs/` file名規約と§11「専用lock」の解釈は未決である。
