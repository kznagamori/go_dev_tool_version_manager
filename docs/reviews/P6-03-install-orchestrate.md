# P6-03 決定記録（6/6）: install orchestrationと§8手順1〜5

対象タスク: `docs/13-progress.md` P6-03の6本目（最終）。規範仕様は[02-architecture.md](../02-architecture.md)§2・§4・§8・§12、[08-install-runtime.md](../08-install-runtime.md)§2・§6・§7、[04-storage-and-data.md](../04-storage-and-data.md)§16.2。

## 1. 着手時の確認事項（5本目の停止記録より）

2件とも**仕様から一意に決まった**ため、利用者判断を求めていない。

### 1.1 orchestrationは2層に分かれる

§2は`internal/app`へ「ユースケースの公開窓口、要求検証、**トランザクション境界**」、`internal/install`へ「ダウンロード、検証、安全展開、probe、receipt、**transaction**」を割り当てる。どちらも「transaction」を含むが、指すものが違う。

| 層 | 責務 | 実装 |
|---|---|---|
| `internal/app` | §8手順1〜5の**境界** — いつlockを取り、いつ`inputs`を照合し、何を許すか | `ExecuteInstall` |
| `internal/install` | installそのものの**順序** — staging→probe→command_targets→harden→commit | `Engine` |

§8の`ExecuteInstall(ctx, Plan, Approval)`はApplication Serviceの操作（§4の`Services.App`）であり、前提検査を済ませてからengineを呼ぶ。

**`internal/app`へ`internal/install`をimportさせない。** engineを`InstallEngine` interfaceで受ける形にした。app側が要るのは「Planを渡すと結果が返る」ことだけで、engineの生成（Guardで包んだportの注入）は合成側の仕事である。importを増やさずに済み、Execute側のtestもstubで書ける。

### 1.2 `app.Guard`はport注入で噛ませる

P5-04の`Guard`は`FileSystem(inner)`／`ProcessRunner(inner)`／`HTTPClient(inner)`でportを包む。install engineの部品はすべて構築時にportを受け取るため、**包んだportを渡すだけで済む**。engine自身はGuardを知らない — 知らせると、Guardを通さずに動かす経路をengine内に作れてしまう。

Scopeは**Plan固有**なので、engineの生成もoperationごとになる。`NewEngine`が`port.FileSystem`を別引数で取るのはそのためで、部品と同じ（包んだ）実装を渡す。

## 2. 判断

### 2.1 `inputs`の再取得をlockの前後で2回行う

§8手順3〜4「`inputs`に固定した…revision/digest identity」「**lock取得後に同じ検査を繰り返す**」。

1回目は承認直後の無駄な作業を避けるため、2回目はlock取得までの間に他processが状態を変えていないことを確かめるためである。**1回目だけだと、lockを待っている間の変化を見逃す。**

### 2.2 再取得できなければ「変わっていない」と見なさない

再取得が失敗したらstaleかどうかを判定できない。判定できないまま作用を始めるのは、判定して通すこととは違う。errorにする。

### 2.3 失敗時はcleanup失敗を握り潰す

§6が成功・失敗のどちらでもstagingの破棄を求める。engineが失敗し、かつcleanupも失敗した場合、**元の失敗のほうが利用者に必要な情報である**。cleanup失敗で上書きすると、何が起きたのか分からなくなる。

成功時のcleanup失敗は§2に従い`W_CLEANUP_INCOMPLETE`とする。

### 2.4 index更新だけの失敗を`W_CLEANUP_INCOMPLETE`へ写す

§7「手順7のrenameが完了した時点でinstallは成功とみなす…indexだけ古い状態であり、次回起動時の再構築で解消する」。専用のwarning codeは§16.2に無く、5値の閉じた集合である。「成功したが後始末が完了していない」という意味で`W_CLEANUP_INCOMPLETE`が最も近い。

**未使用のenum値を足さない**（CLAUDE.md §7）。新codeが要ると判断されれば§16.2の同期修正から始める。

### 2.5 `command_targets`をpermission正規化の前に集める

§7は手順4→5の順である。read onlyにした後だと、filesystemによってはread自体が制限されうる。

## 3. 検査が固定したこと

Executeで20 subtest、engineで11 subtestを追加した。

| 検査 | 対象 |
|---|---|
| `TestExecuteInstallRunsEngineUnderLock` | **engineがlock保持中に走ること**、`inputs`再取得が2回であること、lockが解放されること |
| `TestExecuteInstallDetectsStaleAfterLock` | **lockを待つ間の変化を捕まえること**、staleならengineを起動しないこと |
| `TestExecuteInstallChecksInOrder` | 手順1〜3の順序（前で落ちたら後へ進まない） |
| `TestExecuteInstallCleansUpOnBothPaths` | 成功・失敗のどちらでもcleanupすること |
| `TestExecuteInstallWarnsOnCleanupFailure` | 清掃失敗が成功＋warningになること |
| `TestExecuteInstallKeepsEngineErrorOverCleanupFailure` | **元の失敗を清掃失敗で上書きしないこと** |
| `TestExecuteInstallWarnsOnStaleIndex` / `ReportsAlreadyInstalled` | index失敗の扱い、冪等installの伝播 |
| `TestExecuteInstallFailsWhenInputsUnavailable` | 再取得できないときに進まないこと |
| `TestExecuteInstallMapsLockTimeout` | `E_LOCK_TIMEOUT`（終了code 8） |
| `TestEngineRunsFullSequence` | §7の全手順（rename、`command_targets`、probe結果、index revision） |
| `TestEngineHardensPayloadBeforeCommit` | **正規化がstaging側で行われること**（renameより前） |
| `TestEngineStopsWhenProbeFails` | **probe失敗でcommitしないこと**、失敗時もoperation directoryを返すこと |
| `TestEngineReportsStaleIndexWithoutFailing` | index失敗で完成先を巻き戻さないこと |

### 3.1 変異test

3件入れ、いずれも検査が落ちた。生き残りは無い。

| 変異 | 結果 |
|---|---|
| lock取得後の再検査を省く | 落ちた（2 case） |
| engineをlockの外で走らせる | 落ちた |
| probe失敗でもcommitへ進む | 落ちた |

## 4. 検証

Linux containerで実行した（Go 1.26.6）。両OSの判定はCI matrixで行う。

| command | 結果 |
|---|---|
| `gofmt -l .` / `go build ./...` / `GOOS=windows go build ./...` | 出力なし・成功 |
| `go vet ./...` / `GOOS=windows go vet ./...` | 成功 |
| `go test ./... -race -shuffle=on -covermode=atomic` | 全package成功。`internal/app` 92.4%・`internal/install` 88.4% |
| `check_policy.py` / `check_imports.py` / `check_docs.py` / `check_licenses.py` / `check_messages.py` | すべて成功 |
| `git diff --check` | 出力なし |

**`internal/app`のimport表は変えていない。** engineをinterfaceで受けたためである。

## 5. P6-03の分割について

**当初2分割の想定が6分割になった。** §7の9手順が、失敗時の巻き戻し先・依存する仕様・必要な新規primitiveの点で素直に1本へ収まらず、着手のたびに線を引き直した。

| 本 | 内容 | 分割の理由 |
|---|---|---|
| 1 | Approvalとstaging | §7が9手順あり、前半後半で巻き戻し先が違う |
| 2 | probe cwd規定の是正と`command_targets` | §11違反の修正がPlan builderへ及び、内部streaming digestが新規に要った |
| 3 | permission正規化と`E_CONFLICT` | `Chmod`で両OSを表現できずportへ操作追加が要った |
| 4 | probe実行 | 3で見つかった論点の整理後、実行だけで1本 |
| 5 | commit transaction | atomic renameの前後で巻き戻しの意味が変わる |
| 6 | orchestrationと§8手順1〜5 | 部品が揃ってから繋ぐ |

**2と3で自分が入れた欠陥を2件見つけて直した**（probe cwdをpayloadにしていた、`SameInstall`が往復後に常に不一致だった）。どちらも次の層へ繋いだ時点で表面化しており、部品を単体で作っている間は見えなかった。

## 6. 未実施・制約

- **合成側（`InstallEngine`の実装adapter）が無い。** `install.Engine`と`app.InstallEngine`の型が違うため、両者を繋ぐ薄いadapterが要る。置き場所は`cmd/gdtvm`または`internal/app`のservice構築部で、**P8-01（CLI adapterとInitialize）の範囲**である。P6-03の対象は「Resolve→Plan→Approve→Execute→Commit」の各段であり、CLIからの起動はP8である。
- **`app.Guard`を実際に噛ませた経路のtestが無い。** 注入の形（構築時にport実装を渡す）は固定したが、Guard済みportでengineを動かすE2Eは[11-quality-and-ci.md](../11-quality-and-ci.md)§7.2の照合として合成後（P8以降）に行う。
- **`W_CLEANUP_INCOMPLETE`をindex staleへ流用している。** §16.2の5値に「indexが古い」専用のcodeが無いためである。新codeが要ると判断されれば§16.2の同期修正から始める。
- **receipt indexの再構築は実装しない。** §7が「次回起動時」と定めており、起動時処理（`Initialize`、P8-01）の責務である（4本目からの継続）。
- **P6-02から継続する仕様の食い違いが1件**: `internal/store`のtemplate grammarが`internal/definition`と一致しない（`{{storage.9x}}`をdefinitionは拒否・storeは通す）。fail closedは保たれ正当なdefinitionからは生じない値である。`internal/store`を`internal/definition`へ依存させるかは§2の責務表を要する判断であり、**P6-03の範囲では扱えなかった**。P6-04以降かmaintenance taskで扱う。
- **P6-01から継続する未実装**: Registry portと`LinkManager` portの記録wrapper（P7）、§7.2のE2E照合（P6以降）、`port.FileSystem`／`port.Environment`のproduction実装（P8-01。`HardenReadExecute`のproduction実装もここ）、Windowsの起動とjob割当ての隙間、起動時の孤児staging cleanup。
- **P5-03から継続する未決が1件**（`./`始まりのtar entryを[10-security.md](../10-security.md)§5に従って拒否している）。**標準4 toolの実archiveをまだ繋いでいない** — P6-03はfake archiveでの検証であり、該当の有無はP6-04以降の実toolで分かる。**P6-01で埋めた仕様の空白が1件継続**（exact指定で`installable=false`のときの`E_PLATFORM_UNSUPPORTED`）。**仕様側の未決が2件継続**（[07-registry-and-tools.md](../07-registry-and-tools.md)§5第2項のlicense file size上限が§21の表に無い、§2の「license file名はASCII kebab grammar」が§2自身の例と食い違う）。`python.toml`の`lifecycle = "unknown"`はP3-04から継続。source error専用のerror code（`E_DEFINITION_INVALID`）、version数値要素の64 bit上限、`logs/` file名規約と§11「専用lock」の解釈は未決である。
