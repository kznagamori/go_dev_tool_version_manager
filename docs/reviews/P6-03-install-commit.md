# P6-03 決定記録（2/5）: probe cwd規定の是正と`command_targets`

対象タスク: `docs/13-progress.md` P6-03の2本目（着手時は3分割の想定だった。3本目で4分割へ変更した）。規範仕様は[06-tool-definition.md](../06-tool-definition.md)§10.1・§11・§12、[08-install-runtime.md](../08-install-runtime.md)§7、[04-storage-and-data.md](../04-storage-and-data.md)§7・§14・§16・§21、[02-architecture.md](../02-architecture.md)§2・§4。

## 1. P6-02で入れた仕様違反を1件修正した

**着手時の確認事項の調査中に、自分がP6-02で入れた欠陥を見つけた。**

P6-02の`buildPlanProbe`はprobeのcwdをpayload rootにしていた。添えたコメントは「§11はcwdを宣言させないため、payload外を指す余地を作らない一意な選び方はこれだけである」だったが、**§11は明示的にcwdを定めている**。

> [06-tool-definition.md](../06-tool-definition.md)§11「probeごとに空のowner-only probe tempを作り、成功/失敗/cancel後にengineが削除する。**probeのcwdはその probe temp とし、呼出し元のcurrent directoryを継承しない。** 利用者のproject file（`global.json`、`.nvmrc`等）がprobe結果を変えないようにするための規定である」

[08-install-runtime.md](../08-install-runtime.md)§7手順2も「**probe専用のowner-only temp directoryをcwdとして**required probeを実行する」と同じことを定める。私は§11のこの文を読み落として「宣言させない」と判断した。

| | P6-02（誤） | 本PR（修正後） |
|---|---|---|
| probeのcwd | payload root | probe専用temp |
| probe temp | operation単位・任意 | **probeごと**・必須 |
| `write_paths` | probe tempがあれば1件、無ければ0件 | 常にそのprobeのtemp 1件 |

**payloadをcwdにすると規定の目的そのものを損なう。** §11がこの規定を置く理由は「利用者のproject fileがprobe結果を変えないようにする」ことであり、payload内に`global.json`や`.nvmrc`を含むtoolでは、payloadをcwdにした時点で同じ問題が起きる。

`TestBuildInstallPlanExpandsProbe`へ「cwdがpayload rootになっていないこと」を明示的に見るassertionを足した。cwdをpayloadへ戻す変異でこれが落ちることを確かめている。

## 2. 着手時の確認事項（1本目の停止記録より）

### 2.1 probe tempはoperation staging内へprobeごとに置く

§11「**probeごとに**空のowner-only probe tempを作り」。probe間で共有すると、先に走ったprobeが残したfileが後のprobeの結果を変えうる。

`tmp/operations/<operation-id>/probes/<probe-id>/`とした。probe IDはplatform内一意（§11）であり、そのままdirectoryの一意性になる。

### 2.2 `command_targets`のSHA-256に`StreamHasher`は使えない

`security.StreamHasher`はupstream algorithmを必ず要求し、internalとupstreamの両方を計算する。`command_targets`にupstream digestは無く、渡すと**意味のないalgorithmを選ばせる**ことになる。

`security.InternalStreamDigest`を足した。payload内fileはinstall済みの実体で大きくなりうるため、`SHA256Hex`のように全体をmemoryへ読まず流す。上限を必須引数にしたのは、上限なしで読むと壊れたpayloadやsymlink差し替えで無制限にreadしうるためである。

## 3. 分割の見直し

1本目の停止記録では2本目を「§7のvalidation→commit全体」としていたが、§11違反の修正がPlan builderへ及び、`command_targets`の前提（内部streaming digest）も新規に要ることが分かった。commit transaction（§7手順5〜8）は失敗時の巻き戻しがatomic renameの前後で変わる別の主題であり、同じPRへ混ぜると変更の意図が読みにくくなる。**3分割へ変更した。**

## 4. 判断

### 4.1 `command_targets`は`required=false`のcommandを含めない

§7手順4が`required` runtime commandだけを対象とする。任意commandまで記録すると、そのcommandを使わない利用者のpayloadで`doctor`が**存在しないfileの破損**を報告する。

### 4.2 `command_targets`はpayload外を記録しない

command targetは`{{payload}}`配下と決まっている（§10.1）が、fixed argsは`{{storage.<id>}}`も取れる。**storageは利用者が書き換える領域**であり、完全性記録の対象にすると正常な変更を`doctor`が破損として報告する。§14も「payload内fileだけ」と定める。

### 4.3 `path`は`payload/`prefixを持つ

§14の表記は「payload相対path」だが、同§の例は`payload/node.exe`であり、`payload_path=payload`固定と併せて読むとprefixを含む形が正である。例に合わせた。

### 4.4 実体が無ければcommit前に失敗させる

required commandのtargetが無いpayloadをcommitすると、`doctor`が破損として報告する状態を作ってしまう。`CollectCommandTargets`は開けないfileをerrorにする。

## 5. 検査が固定したこと

| 検査 | 対象 |
|---|---|
| `TestBuildInstallPlanExpandsProbe` | **probeのcwdがprobe tempであり、payload rootでないこと**（§11） |
| `TestBuildInstallPlanRequiresProbeTempPerProbe` | probe temp rootの必須性、**probeごとに別directory**、role検査 |
| `TestBuildInstallPlanResolvesProbeTempTemplate` | `{{probe_temp}}`がprobe固有directoryへ解決すること |
| `TestCollectCommandTargetsRecordsRequiredTargets` | `path`/`size`/`sha256`の記録内容 |
| `TestCollectCommandTargetsSkipsOptionalCommands` | `required=false`を含めないこと |
| `TestCollectCommandTargetsIncludesPathArgs` | fixed argsのpayload内fileとbyte順 |
| `TestCollectCommandTargetsExcludesStorage` | payload外を記録しないこと |
| `TestCollectCommandTargetsDeduplicates` | 一意性 |
| `TestCollectCommandTargetsReportsMissingFile` / `ReportsReadFailure` | 実体欠落と読取り失敗の注入 |
| `TestInternalStreamDigestMatchesBufferDigest` | stream版とbuffer版の一致（receiptと`doctor`の照合値が食い違わないこと） |
| `TestInternalStreamDigestEnforcesLimit` / `ReportsReadError` | 上限ちょうど／超過、読取り失敗 |

### 5.1 変異test

4件入れ、いずれも検査が落ちた。生き残りは無い。

| 変異 | 結果 |
|---|---|
| **probeのcwdをpayloadへ戻す（P6-02の欠陥そのもの）** | 落ちた |
| probe間でtempを共有する | 落ちた |
| payload外（storage）も記録する | 落ちた |
| `required=false`のcommandも記録する | 落ちた |

## 6. 検証

Linux containerで実行した（Go 1.26.6）。両OSの判定はCI matrixで行う。

| command | 結果 |
|---|---|
| `gofmt -l .` / `go build ./...` / `GOOS=windows go build ./...` | 出力なし・成功 |
| `go vet ./...` / `GOOS=windows go vet ./...` | 成功 |
| `go test ./... -race -shuffle=on -covermode=atomic` | 全package成功。`internal/security` 97.2%・`internal/app` 92.4%・`internal/install` 89.7% |
| `check_policy.py` / `check_imports.py` / `check_docs.py` / `check_licenses.py` / `check_messages.py` | すべて成功 |
| `git diff --check` | 出力なし |

`internal/install`のcoverageが90.2%→89.7%へ下がった。`CollectCommandTargets`のerror経路のうち、fake filesystemでは到達させにくい`security.Join`失敗分岐が残っているためである。

## 7. 未実施・制約

- **probeの実行そのものは未実装。** 本PRが用意したのはPlan側の規定（cwd、per-probe temp）と`command_targets`計算である。§7手順2〜3のprobe起動（sanitized最小環境、`ProcessRunner`経由）は3本目で扱う。**probe tempの実際の作成・削除も3本目**である——Planはpathを確定するだけで、directoryを作らない。
- **`CollectCommandTargets`をまだ誰も呼んでいない。** 呼出し側は3本目のcommit transactionである。
- **§7手順5〜8（permission正規化、receipt書込み、atomic rename、receipt index更新）とidempotence/`E_CONFLICT`は未実装。** §8手順3〜5（`inputs`実体再取得、lock、`app.Guard`適用）も3本目である。
- **P6-02から継続する仕様の食い違いが1件**: `internal/store`のtemplate grammarが`internal/definition`と一致しない。fail closedは保たれ正当なdefinitionからは生じない値である。3本目で扱う。
- **P6-01から継続する未実装**: Registry portと`LinkManager` portの記録wrapper（P7）、[11-quality-and-ci.md](../11-quality-and-ci.md)§7.2のE2E照合（P6以降）、`port.FileSystem`／`port.Environment`のproduction実装（P8-01）、Windowsの起動とjob割当ての隙間。起動時の孤児staging cleanupも未実装（1本目から継続）。
- **P5-03から継続する未決が1件**（`./`始まりのtar entryを[10-security.md](../10-security.md)§5に従って拒否しており、標準4 toolの実archiveを繋ぐ3本目で該当があれば仕様側で扱いを決める）。**P6-01で埋めた仕様の空白が1件継続**（exact指定で`installable=false`のときの`E_PLATFORM_UNSUPPORTED`）。**仕様側の未決が2件継続**（[07-registry-and-tools.md](../07-registry-and-tools.md)§5第2項のlicense file size上限が§21の表に無い、§2の「license file名はASCII kebab grammar」が§2自身の例と食い違う）。`python.toml`の`lifecycle = "unknown"`はP3-04から継続。source error専用のerror code（`E_DEFINITION_INVALID`）、version数値要素の64 bit上限、`logs/` file名規約と§11「専用lock」の解釈は未決である。
