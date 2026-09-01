# P6-03 決定記録（1/4）: ApprovalとExecute前半（staging）

対象タスク: `docs/13-progress.md` P6-03の1本目。規範仕様は[02-architecture.md](../02-architecture.md)§2・§4・§8、[08-install-runtime.md](../08-install-runtime.md)§2・§4・§5・§6、[04-storage-and-data.md](../04-storage-and-data.md)§2・§16・§16.1・§17.2、[03-cli.md](../03-cli.md)§7。

## 1. 着手時の確認事項（P6-02の停止記録より）

2件とも**仕様から一意に決まった**ため、利用者判断を求めていない。

### 1.1 `inputs`の実体再取得はApplication Serviceが束ねる

[02-architecture.md](../02-architecture.md)§8が「Executeは次を順に検査する」として手順1〜5をExecuteへ割り当て、§2が`internal/app`へ「トランザクション境界」を割り当てる。したがって再取得の順序と時点（承認前・lock取得後の2回）を決めるのはApplication Serviceである。

**これをportにしない。** §4「効果がすべて既存portの背後へ閉じているorchestrationはportにしない」。各供給元の読取りはすべて`port.FileSystem`の背後にあり、束ねる側は呼び分けるだけである。portにすると、再取得そのものを差し替えられてしまい`E_PLAN_STALE`の検査をtestで確かめられなくなる。

### 1.2 `install --use`の`writes[]`はP6-03では生じない

台帳のP6-03は**`install`非選択**を対象とする。current linkとproject fileはP6-04の`use`が扱う。そのとき選択の決定は§2が`internal/selection`へ割り当てており、Plan builderへは`inputs`と同じく**供給する**形にする（P6-02で確立した「集めずに受け取る」形）。

## 2. 分割

[08-install-runtime.md](../08-install-runtime.md)§7のvalidation→commitだけで9手順あり、§4のApprovalと§5・§6のdownload/stagingとは**失敗時の巻き戻し先が異なる**。前半はoperation directoryの削除1回で復旧するが、後半はatomic renameの前後で意味が変わる。そのため2 PRへ分けた。

1本目（本PR）: Approval型、§8手順1〜2の前提検査、download→verify→extract→staging、operation tmpのlifecycleとcancel境界。
2本目: §7のvalidation→commit（probe実行、`command_targets`、permission正規化、receipt、atomic rename、index更新）とidempotence/`E_CONFLICT`。

## 3. 判断

### 3.1 `--yes`の承認集合を呼出し側に並べさせない

[08-install-runtime.md](../08-install-runtime.md)§4「`--yes`は§16.1で明示承認が必要な7件すべてを承認できる」。7件を呼出し側が並べると、§16.1の表を変えたときに片方だけが古いままになる。

`store.ApprovalRequiredCodes()`を公開し、表から導く経路を1つにした。P6-02で`store.NewPlanWarning`を公開したのと同じ理由である。

**`W_RESTART_REQUIRED`を含めない。** §16.1が「情報提供であり承認の対象にしない」と定めており、含めると承認対象が8件あるように見える。

### 3.2 mode未設定のApprovalはwarning無しのPlanでも通さない

zero値の`Approval`は「承認していない」である。warningが1件も無いPlanなら実害はないが、通すと**Approvalを渡し忘れた呼出しがそのまま実行へ進む**。承認は明示的に構築されたものだけを受ける。

### 3.3 client/invocation不一致は`E_PLAN_STALE`とする

§8手順1「Plan schema/client/invocationの一致」。専用のerror codeは無い。利用者から見ると「Planが古い」であり、取る行動はstale判定と同じ（作り直して再実行）のため`E_PLAN_STALE`を使う。

**Planは承認と対で意味を持つ一時値である。** 前回実行のPlanを今回のapprovalで実行できると、利用者が見た内容と実際の対象が食い違う。

### 3.4 staging失敗時に部分削除しない

§6「中断・失敗・cancel時は`tmp/operations/<operation-id>/`をdirectory単位で削除すれば復旧する」。

途中で部分削除すると、**何が残っているかが失敗経路ごとに変わり**、復旧手順が経路の数だけ増える。`Stage`は失敗時もoperation directoryを結果へ返し、呼出し側が`Cleanup`を1回呼ぶ形にした。

### 3.5 `Cleanup`は自分のoperationのdirectoryだけを消す

§2「root ID・owner・作成時刻を検査したうえでこのdirectoryをまとめて削除する」。この検査が要るのは**他のoperationが残したdirectory**を掃除する場合（起動時のcleanup）である。自分が今作ったdirectoryは所有が自明なため、operation IDの一致だけを確かめる。

末尾componentの一致で見る。`...xabc`が`abc`で終わることを一致と誤判定しないよう、直前がpath区切りであることまで確かめる。**同時実行中の別operationの作業領域を消すと、そちらが不整合な状態で継続する。**

### 3.6 `domain.Version`をPlanから復元せず受け取る

progress通知が`domain.Version`を取る。`summary.version`は正規化済みの文字列だが**version schemeを持たない**ため、型へ戻すにはtoolのschemeが要る。解決したcatalog itemを持つ呼出し側から受け取る形にした（P6-02の「集めずに受け取る」形）。

## 4. 検査が固定したこと

Approvalで12 subtest、stagingで22 subtestを追加した。

| 検査 | 対象 |
|---|---|
| `TestAssumeYesApprovalCoversExactlySevenCodes` | `--yes`が7件を承認し、`W_RESTART_REQUIRED`を含めないこと |
| `TestCheckApprovalRejectsMissingCode` | 1件でも不足すれば`E_APPROVAL_REQUIRED`（終了code 11） |
| `TestCheckApprovalIgnoresRestartRequired` | 承認不要codeが承認を強いないこと |
| `TestCheckApprovalRejectsUnsetMode` | zero値のApprovalを通さないこと |
| `TestNewApprovalRejectsUnknownValue` / `FoldsDuplicates` | 未知mode/code、重複の畳み込みとcode順 |
| `TestCheckPlanIdentityRejectsForeignPlan` | 別client/invocationのPlan |
| `TestStageDownloadsAndExtractsIntoOperationDir` | `tmp/operations/<operation-id>/`配下への配置、payload roleへの付け替え、data root外へ書かないこと |
| `TestStageCleanupRemovesOperationDir` | directory単位の削除、download cacheを消さないこと |
| `TestStageCleanupRejectsForeignDirectory` | 別operation・operations root自体・末尾一致・role違いを消さないこと |
| `TestStageStopsOnCancel` | cancel境界（HTTPを1回も呼ばない） |
| `TestStageFailsWhenDownloadFails` / `WhenExtractFails` | 失敗の伝播と、失敗時にoperation directoryを消さないこと |
| `TestStageReportsFilesystemFailure` | mkdir失敗の注入 |
| `TestStageRejectsInvalidRequest` | 前提違反8件 |

### 4.1 変異test

4件の変異を入れ、いずれも検査が落ちることを確かめた。生き残りは無い。

| 変異 | 結果 |
|---|---|
| `Cleanup`のoperation照合を外す | 落ちた（3 case） |
| `--yes`へ`W_RESTART_REQUIRED`を含める | 落ちた |
| 未承認codeを見逃す | 落ちた |
| 別invocationのPlanを許す | 落ちた |

## 5. 検証

Linux containerで実行した（Go 1.26.6）。両OSの判定はCI matrixで行う。

| command | 結果 |
|---|---|
| `gofmt -l .` / `go build ./...` / `GOOS=windows go build ./...` | 出力なし・成功 |
| `go vet ./...` / `GOOS=windows go vet ./...` | 成功 |
| `go test ./... -race -shuffle=on -covermode=atomic` | 全package成功。`internal/app` 92.4%・`internal/install` 90.2% |
| `check_policy.py` / `check_imports.py` / `check_docs.py` / `check_licenses.py` / `check_messages.py` | すべて成功 |
| `git diff --check` | 出力なし |

message IDを1件追加した（`plan.approval_required`）。`MessageCount`を93→94へ更新した。

## 6. 未実施・制約

- **`Stager`と`Approval`をまだ誰も呼んでいない。** 呼出し側は2本目のExecute orchestrationである。§8手順3〜5（`inputs`実体再取得、lock取得後の再検査、Scope適用）も2本目で繋ぐ。
- **`Stage`は`app.Guard`を通していない。** [11-quality-and-ci.md](../11-quality-and-ci.md)§7.2の記録wrapperを噛ませるのはExecute orchestrationの責務で、2本目で行う。現状のstagingは`Stage`自身のpath組立て（[security.Join]）でoperation directory内に収まる。
- **lock取得を実装していない。** §8手順4がlock取得後の再検査を求め、[02-architecture.md](../02-architecture.md)§12がlock順序を定める。Execute orchestrationと同じ2本目で扱う。
- **起動時の孤児staging cleanupは未実装。** §2の「root ID・owner・作成時刻を検査したうえで削除」は**他のoperationが残したdirectory**の掃除であり、`Stager.Cleanup`が扱う自分のdirectoryとは別経路である。`doctor`／起動時処理としてP6以降で扱う。
- **P6-02から継続する仕様の食い違いが1件**: `internal/store`のtemplate grammarが`internal/definition`と一致しない。fail closedは保たれ正当なdefinitionからは生じない値である。2本目で扱う。
- **P6-01から継続する未実装**: Registry portと`LinkManager` portの記録wrapper（P7）、§7.2のE2E照合（P6以降）、`port.FileSystem`／`port.Environment`のproduction実装（P8-01）、Windowsの起動とjob割当ての隙間。
- **P5-03から継続する未決が1件**（`./`始まりのtar entryを[10-security.md](../10-security.md)§5に従って拒否しており、標準4 toolの実archiveを繋ぐ2本目で該当があれば仕様側で扱いを決める）。**P6-01で埋めた仕様の空白が1件継続**（exact指定で`installable=false`のときの`E_PLATFORM_UNSUPPORTED`）。**仕様側の未決が2件継続**（[07-registry-and-tools.md](../07-registry-and-tools.md)§5第2項のlicense file size上限が§21の表に無い、§2の「license file名はASCII kebab grammar」が§2自身の例と食い違う）。`python.toml`の`lifecycle = "unknown"`はP3-04から継続。source error専用のerror code（`E_DEFINITION_INVALID`）、version数値要素の64 bit上限、`logs/` file名規約と§11「専用lock」の解釈は未決である。
