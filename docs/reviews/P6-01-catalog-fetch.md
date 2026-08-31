# P6-01 決定記録（2/2）: catalog取得orchestrationとoffline方針

対象タスク: `docs/13-progress.md` P6-01の2本目。規範仕様は[04-storage-and-data.md](../04-storage-and-data.md)§15・§16.2、[03-cli.md](../03-cli.md)§3.2・§7、[06-tool-definition.md](../06-tool-definition.md)§6.1・§6.2、[08-install-runtime.md](../08-install-runtime.md)§3.2、[02-architecture.md](../02-architecture.md)§14。

## 1. 着手時の確認事項（1本目の停止記録より）

2件とも解決した。

### 1.1 offline判定をport境界へ移した

P5-01が`internal/install`へ置いた`isOffline`は`net.DNSError`とsyscall errno（`ENETUNREACH`／`EHOSTUNREACH`）を直接見る。これを`internal/catalog`へ複製すると規則が2箇所へ散る。しかも**fakeがsyscall errorを作れないため、どちらのtestでもofflineを再現できない**。

`port.ErrOffline`をsentinelとして置き、HTTPClient adapterがwrap、呼出し側は`errors.Is`で判定する形にした。

| | before | after |
|---|---|---|
| 判定の実体 | `internal/install`（syscall検査） | `internal/platform`（adapter内1箇所） |
| `internal/install` | 自前でsyscall検査 | `errors.Is(err, port.ErrOffline)` |
| `internal/catalog` | — | 同上 |
| fakeでの再現 | できない | `fmt.Errorf("%w: ...", port.ErrOffline)` |

判定はHTTPClientを実装するadapterだけが持つ。syscall errnoとnet packageの型を見分けられるのはそこだけであり、呼出し側へ持ち出すと規則が散る。

**P5-02から継続していた「offline判定はsyscall errnoに依存し、両OSでの実挙動をunit testで検証できていない」制約が、これで解けた。** wrap部分を`wrapNetworkError`として切り出し、実networkを使わずに固定できるようにした。

### 1.2 取得失敗時に既存cacheを消さない

[03-cli.md](../03-cli.md)§3.2「`--refresh`はcatalog cacheを**atomic置換**する運用data更新」。atomic置換は、失敗時に旧内容がそのまま残ることを意味する。

消してしまうと、[04-storage-and-data.md](../04-storage-and-data.md)§15が認めるoffline exactの退避先も同時に失われる。`json-index`の子文書が1件でも失敗すれば[06-tool-definition.md](../06-tool-definition.md)§6.2に従いcatalog全体をsource errorとするが、**cacheはそのまま残す**。

この結論は仕様から一意に決まるため、利用者判断を求めていない。

## 2. 判断

### 2.1 期限切れcacheへ退避できるのは`exact`だけ

[04-storage-and-data.md](../04-storage-and-data.md)§15「offline exactはidentity/digestが完全なら期限切れを`W_CACHE_STALE`付きで利用できる。**`--latest`は期限切れを黙って使わない**」。

| Intent | 期限切れcache | 理由 |
|---|---|---|
| `exact` | 使える | 利用者が版を名指ししており、古い一覧でもその版の情報は変わらない |
| `latest` | 使わない | 「今の最大」を問う操作。古い一覧から答えると別の版を返しうる |
| `list` | 使わない | 「今どの版があるか」を問う操作でlatestと同じ性質 |

`list`（`available`）を`latest`と同じ扱いにしたのは、§3.2が「cacheなしでonlineならrefresh、offlineなら`E_CATALOG_MISSING`」とだけ定め、期限切れを黙って表示してよいとは書いていないためである。

### 2.2 offline以外の失敗では退避しない

source layout違反、5xx、TLS検証失敗は上流かdefinitionの問題である。古いcacheで隠すと気付けない。offlineだけが「上流は正しいが今は見られない」状態であり、§15が退避を認めているのもその場合だけである。

### 2.3 判断の順序

```text
--refresh   → 鮮度によらず取り直す（§3.2）
cacheがfresh → そのまま使う
それ以外     → 取り直す
  成功        → 保存して使う
  offline失敗 → exactかつstale cacheがあれば退避（Stale=true）、無ければ E_CATALOG_MISSING
  その他失敗  → そのerrorを返す（退避しない）
```

### 2.4 `json-index`のitem件数は全子文書の合計で判定する

[06-tool-definition.md](../06-tool-definition.md)§6.1「全文書合計のitemsは10,000の組込み上限以下」。文書ごとに判定すると、合計で上限を超えられる。

### 2.5 static sourceの`source_identity`はdefinition記録

§15「source fieldならsource URL/fetch時刻、override/staticなら**definition記録**を使う」。networkを使わないためURLが無く、tool/platformで一意に決まる記録（`definition:<tool>/<platform>`）を使う。

## 3. 検査が固定したこと

| 検査 | 対象 |
|---|---|
| `TestRefreshFetchesWhenCacheMissing` | cacheが無ければ取り直し、結果を保存すること |
| `TestRefreshUsesFreshCacheWithoutNetwork` | 期限内cacheで上流へ行かないこと（HTTP呼出し回数で見る） |
| `TestRefreshForceRefetchesFreshCache` | `--refresh`が鮮度によらず取り直すこと |
| `TestRefreshFallsBackToStaleCacheForExactOffline` | exactだけが退避でき、latest／listは`E_CATALOG_MISSING`になること |
| `TestRefreshKeepsExistingCacheOnFailure` | 失敗しても既存cacheの内容が変わらないこと |
| `TestRefreshDoesNotFallBackOnNonOfflineFailure` | 5xxで退避しないこと |
| `TestCollectItemsUsesStaticSourceWithoutNetwork` | staticがHTTPを呼ばず、identityがdefinition記録になること |
| `TestCollectItemsRejectsUnknownKind` | 未知のsource kind |
| `TestRefreshRejectsInvalidRequest` / `TestNewRefresherRequiresDependencies` | 要求の前提違反と依存不足 |
| `TestStaleWarningCarriesCode` | `W_CACHE_STALE`とmessage ID、`expires_at` parameter |
| `TestRefreshReportsSaveFailure` | cache保存の失敗注入 |
| `TestIsOfflineDistinguishesUnreachableNetwork`（platform） | DNS失敗/network unreachable/host unreachable → offline、connection refused/5xx → 一時障害 |
| `TestWrapNetworkErrorMarksOffline` / `TestWrapNetworkErrorMasksURL`（platform） | sentinelでのwrapと、wrap後のcredential除去 |

## 4. 検証

Linux containerで実行した（Go 1.26.6）。両OSの判定はCI matrixで行った。

| command | 結果 |
|---|---|
| `gofmt -l .` / `go build ./...` / `GOOS=windows go build ./...` | 出力なし・成功 |
| `go vet ./...` / `GOOS=windows go vet ./...` | 成功 |
| `go test ./... -race -shuffle=on -covermode=atomic` | 全package成功。`internal/catalog` 87.5%・`internal/platform` 93.8% |
| `check_policy.py` / `check_imports.py` / `check_docs.py` / `check_licenses.py` / `check_messages.py` | すべて成功 |
| `git diff --check` | 出力なし |
| CI（PR #118） | 12 check成功 |

message IDを2件追加した（`catalog.cache_stale`、`catalog.missing_offline`）。`MessageCount`を85→87へ更新し、catalog分類内のID順も直した（sorted検査が指摘した）。

## 5. 未実施・制約

- **`Refresher`をまだ誰も使っていない。** 呼出し側はP6-02のPlanとP8のCLI adapterである。`RefreshResult.Stale`から`W_CACHE_STALE`を組み立てる`StaleWarning`は用意したが、結果へ載せるのは呼出し側である。
- **`internal/install`のcoverageが92.0%→91.8%へ下がった。** 自前の`isOffline`とそのtestを`internal/platform`へ移したためで、検査そのものは移動先で維持している。
- **P5-04から継続する未実装**: Registry portと`LinkManager` portの記録wrapper（P7）、[11-quality-and-ci.md](../11-quality-and-ci.md)§7.2のE2E照合（P6以降）、`port.FileSystem`／`port.Environment`のproduction実装（P8-01）、Windowsの起動とjob割当ての隙間。
- **1本目で埋めた仕様の空白が1件**: exact指定で`installable=false`のversionへ当たったときのerror codeを[08-install-runtime.md](../08-install-runtime.md)§3.1が明示しておらず、`E_PLATFORM_UNSUPPORTED`とした。
- **P5-03から継続する未決が1件**（`./`始まりのtar entryを[10-security.md](../10-security.md)§5に従って拒否しており、標準4 toolの実archiveを繋ぐP6-02以降で該当があれば仕様側で扱いを決める）。**仕様側の未決が2件継続**（[07-registry-and-tools.md](../07-registry-and-tools.md)§5第2項のlicense file size上限が§21の表に無い、§2の「license file名はASCII kebab grammar」が§2自身の例と食い違う）。`python.toml`の`lifecycle = "unknown"`はP3-04から継続。source error専用のerror code（`E_DEFINITION_INVALID`）、version数値要素の64 bit上限、`logs/` file名規約と§11「専用lock」の解釈は未決である。
