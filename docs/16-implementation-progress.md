# 実装・評価進捗チェックリスト

## 1. 目的

本書は、`go_dev_tool_version_manager`（`gdtvm`）の実装、標準registry作成、評価、リリース準備を途中停止・再開できる単一の進捗台帳である。実装順序はWindowsを先行し、Windowsの合格ゲートを通過してからLinux固有実装と評価へ進む。

本書のチェックだけを満たすために仕様を弱めてはならない。詳細な完了条件は各項目が参照する仕様を正とし、仕様変更が必要になった場合は先に該当文書、schema、試験、本書を同時更新する。

## 2. 進捗スナップショット

作業を開始・停止・再開するたびに、次の値を更新する。値を空欄にせず、該当しない場合は `なし` と記載する。

| 項目 | 現在値 |
|---|---|
| 全体状態 | `未着手` |
| 現在フェーズ | `W00` |
| 実行中タスクID | `なし` |
| 最後に完了したタスクID | `なし` |
| 次に開始するタスクID | `W00-01` |
| Windows評価状態 | `未着手` |
| Linux評価状態 | `Windows完了待ち` |
| blocker | `なし` |
| 最終更新日時 | `2026-07-23T00:00:00+09:00` |
| 更新者 | `未設定` |
| 作業branch/commit | `未設定` |
| 使用環境 | `Windows先行。詳細未記録` |
| 最新の証跡 | `なし` |

全体状態は `未着手`, `進行中`, `停止中`, `blocked`, `完了` のいずれかとする。実行中タスクは同時に1件だけとし、並列作業を行う場合も統合責任者が次の再開地点を1件に固定する。

## 3. チェック記法と更新規則

- `[ ]`：未着手
- `[x]`：完了。完了条件をすべて満たし、証跡を記録済み
- `[-]`：進行中。Markdown標準checkboxではないため、停止前に `[ ]` または `[x]` へ戻し、スナップショットへ状態を書く
- `[!]`：blocked。理由、試行内容、解除条件を停止記録へ書く
- `[~]`：仕様変更により不要。削除せず、置換先タスクIDと判断記録を残す

各タスクの完了時には、末尾の「証跡」にcommit、test commandと結果、report path、または判断記録pathを追記する。口頭確認だけで `[x]` にしない。失敗試験も削除せず、解決したcommitまたはissueへ関連付ける。

### 3.1 停止手順

1. 変更中fileを保存し、生成途中fileや一時状態を確認する。
2. 実行中test/processを安全に終了し、終了できないものを記録する。
3. 完了条件を満たしたタスクだけ `[x]` にする。
4. 未完了タスクは `[ ]` に戻し、下記停止記録へ「完了済み部分」と「残作業」を書く。
5. 進捗スナップショットの現在フェーズ、最後の完了、次タスク、blocker、branch/commit、環境、証跡、日時を更新する。
6. build/test結果と未commit差分を確認し、再開者が同じ入力を復元できる状態にする。

### 3.2 再開手順

1. 進捗スナップショットと最新の停止記録を読む。
2. 記録されたbranch/commitと作業tree差分が一致することを確認する。
3. Windows/Linux、OS version、architecture、filesystem、shell、Go version、registry snapshotを記録と照合する。
4. blockerの現状を確認する。
5. 最後に成功した関連testを再実行し、環境が再現できることを確認する。
6. 「次に開始するタスクID」だけを進行中にして作業を再開する。

### 3.3 停止・再開記録

新しい記録を上へ追加する。

| 日時 | 状態 | 完了済み部分 | 残作業・次の具体操作 | blocker/解除条件 | branch/commit | 環境 | 証跡 |
|---|---|---|---|---|---|---|---|
| 2026-07-23T00:00:00+09:00 | 仕様再監査 | 17文書との対応と規範領域を確認 | `W00-01`から開始 | なし | 未設定 | Windows先行 | 本書 |
| 2026-07-22T00:00:00+09:00 | 初期作成 | チェックリスト作成 | `W00-01`から開始 | なし | 未設定 | Windows先行 | 本書 |

## 4. フェーズゲート

後続フェーズへ進む前に、対応するgateをすべて満たす。

- [ ] **G-WIN-START**：W00完了。Windows開発環境と証跡保存先が再現可能
- [ ] **G-WIN-CORE**：W01～W09完了。Windows上で全共通機能とCLIを評価済み
- [ ] **G-WIN-TOOLS**：W10完了。Windows amd64必須17ツールのregistry contractを評価済み
- [ ] **G-WIN-E2E**：W11～W12完了。Windows標準ユーザーでE2E・非機能・security合格
- [ ] **G-LINUX-START**：G-WIN-E2E合格、既知問題を分類しLinux開始判断を記録
- [ ] **G-LINUX-CORE**：L01～L04完了。Linux固有portとshell/runtime評価済み
- [ ] **G-LINUX-TOOLS**：L05完了。Linux必須/条件/第三者tool contractを評価済み
- [ ] **G-CROSS**：X01完了。Windows/Linux共通contractと配布物を評価済み
- [ ] **G-RELEASE**：R01完了。clientとregistryのrelease手順、rollback、署名を評価済み
- [ ] **G-DONE**：全必須タスク完了、延期項目に承認済み判断記録あり

Linux固有コード、Linux向けregistry recipe、Linux E2EをG-LINUX-STARTより前に実装しない。ただしplatform抽象のinterface、fake、Windows上で動くplatform-neutral test fixtureはWindowsフェーズで実装する。

## 5. W00 Windows開発準備・仕様固定

- [ ] **W00-01** `docs/README.md`と番号付き01～17の全18文書を読み、規範用語、未決事項、矛盾のissue一覧を作成する。依存: なし。完了: 未決事項が実装者の暗黙判断として残らない。証跡: 未記録
- [ ] **W00-02** Windows対象matrixを固定する。Windows 10/11、amd64必須、arm64 build/評価方法、NTFS、標準ユーザー、cmd、Windows PowerShell 5.1、PowerShell 7、VS Codeを含める。依存: W00-01。証跡: 未記録
- [ ] **W00-03** Go toolchain、module path、format、lint、unit/integration/E2E command、coverage取得方法を固定する。依存: W00-01。証跡: 未記録
- [ ] **W00-04** test artifact、log、coverage、SBOM、署名、benchmarkを保存するdirectoryと命名規則を定める。secretを保存しない。依存: W00-03。証跡: 未記録
- [ ] **W00-05** Git branch、commit、review、schema変更、registry変更、release tagの運用を定める。依存: W00-01。証跡: 未記録
- [ ] **W00-06** Windows CI runnerを標準ユーザー権限で構成し、管理者権限を必要としないことを確認する。依存: W00-02, W00-03。証跡: 未記録
- [ ] **W00-07** fake clock、fake HTTP、fake process、temporary filesystem、failure injectionのtest基盤方針を固定する。依存: W00-03。証跡: 未記録
- [ ] **W00-08** G-WIN-STARTを判定し記録する。依存: W00-01～W00-07。証跡: 未記録

## 6. W01 Go project・アーキテクチャ骨格

- [ ] **W01-01** `cmd/gdtvm` と全`internal/*`責務境界を02章どおり作成する。依存: G-WIN-START。証跡: 未記録
- [ ] **W01-02** Domain値（ToolID、Version、Platform、Scope、Mode、Digest、InstallKey、Selection）と不変条件を実装・unit testする。依存: W01-01。証跡: 未記録
- [ ] **W01-03** FileSystem、LinkManager、HTTPClient、ProcessRunner、ArchiveExtractor、security、lock、clock、eventのportを定義する。依存: W01-01。証跡: 未記録
- [ ] **W01-04** Application Service生成時の依存注入を実装し、package global mutable stateがないことをtestする。依存: W01-03。証跡: 未記録
- [ ] **W01-05** CoreError、stable error code、cause、remediation、CLI終了code mappingを実装・testする。依存: W01-02。証跡: 未記録
- [ ] **W01-06** operation ID、context cancellation、structured logger、secret maskの共通基盤を実装・testする。依存: W01-03。証跡: 未記録
- [ ] **W01-07** locale-neutral message IDとja/en catalog loaderを実装しplaceholder一致をtestする。依存: W01-01。証跡: 未記録
- [ ] **W01-08** 13章1.1節に従いpackage、export宣言、重要な内部契約、非自明な処理へドキュメントコメントと意図説明コメントを記載し、review基準を確立する。依存: W01-01。証跡: 未記録

## 7. W02 設定・path・保存状態

- [ ] **W02-01** portable/user mode、bootstrap locator、CLI/env/config優先順位を実装・testする。依存: W01。証跡: 未記録
- [ ] **W02-02** global TOML schema、既定値、未知key、duration、上限、security制約をstrict実装・testする。依存: W02-01。証跡: 未記録
- [ ] **W02-03** `.gdtvm.toml`完全版、disabled、未知tool、Git境界、境界越え設定、symlink loopを実装・testする。依存: W02-01。証跡: 未記録
- [ ] **W02-04** Windows portable/user directory構造とPathResolverを実装し、管理root containmentをtestする。依存: W02-01。証跡: 未記録
- [ ] **W02-05** state/schema、selection、registry、approval、setup、shim index、receipt、catalog、journalのcodecを14章どおり実装する。依存: W01-02, W02-04。証跡: 未記録
- [ ] **W02-06** atomic write、flush、backup、revision、optimistic conflict、corruption recoveryを実装・failure injection testする。依存: W02-05。証跡: 未記録
- [ ] **W02-07** lock順序、Windows process間lock、stale判定、timeoutを実装・並行testする。依存: W02-04。証跡: 未記録
- [ ] **W02-08** cache保持、参照snapshot保護、LRU、log rotation、audit保持を実装・testする。依存: W02-05。証跡: 未記録
- [ ] **W02-09** schema migration、未来schema read-only failure、backup/rollbackを実装・testする。依存: W02-05, W02-06。証跡: 未記録

## 8. W03 定義schema・設定駆動engine

- [ ] **W03-01** tool definition TOMLの全field、enum、default、strict unknown-key検査を実装する。依存: W02-02。証跡: 未記録
- [ ] **W03-02** version scheme、完全一致、sort、channel、transform pipelineを実装・testする。依存: W03-01。証跡: 未記録
- [ ] **W03-03** 全version source adapter、parser上限、pagination、ETag、append-observedを実装・fake upstream testする。依存: W03-01, W01-03。証跡: 未記録
- [ ] **W03-04** artifact条件式、template grammar、asset selection、一意性、redirect host、availabilityを実装・testする。依存: W03-01。証跡: 未記録
- [ ] **W03-05** checksum/signature source、pending resolution、catalog固定化を実装・testする。依存: W03-03, W03-04。証跡: 未記録
- [ ] **W03-06** dependency DAG、cycle/conflict、managed/helper/system/optional解決を実装・testする。依存: W03-01。証跡: 未記録
- [ ] **W03-07** install step型、型付きoutput、DAG、logical root、条件、制限を実装・testする。依存: W03-01。証跡: 未記録
- [ ] **W03-08** runtime command、launcher、environment merge、shell export、validation probeを実装・testする。依存: W03-01。証跡: 未記録
- [ ] **W03-09** hook schema、hash承認、argv、writes、timeout、shell policyを実装・testする。依存: W03-01, W01-06。証跡: 未記録
- [ ] **W03-10** helper definition schemaとreceipt contractを実装・testする。依存: W03-04, W03-07。証跡: 未記録
- [ ] **W03-11** 15章のGo定義fixtureをstrict parseし、Planまで生成するcontract testを作る。依存: W03-01～W03-10。証跡: 未記録
- [ ] **W03-12** 新tool fixtureをTOML追加だけでavailable/install/use/shim/uninstallでき、tool名によるGo分岐がないことをtestする。依存: W03-11, W06～W08の該当機能。完了は後続実装後に判定。証跡: 未記録

## 9. W04 Registry・署名・bootstrap

- [ ] **W04-01** Ed25519 embedded trust root、manifest raw byte署名、key ID検証を実装・testする。依存: W01-03。証跡: 未記録
- [ ] **W04-02** manifest canonical生成・strict parse、path、role、size、SHA-256、互換版検査を実装・testする。依存: W03-01, W04-01。証跡: 未記録
- [ ] **W04-03** GitHub tag列挙、annotated/lightweight tag解決、commit SHA pin、rate limit/token maskを実装・testする。依存: W01-03。証跡: 未記録
- [ ] **W04-04** registry archive安全取得、全file照合、snapshot atomic有効化、previous保持を実装する。依存: W04-02, W04-03, W02-06。証跡: 未記録
- [ ] **W04-05** 初回auto bootstrap、offline、互換候補探索、downgrade防止、明示rollback確認を実装・testする。依存: W04-04。証跡: 未記録
- [ ] **W04-06** key rotation、keys.toml、revoked.toml、min/max client compatibilityを実装・testする。依存: W04-01, W04-04。証跡: 未記録
- [ ] **W04-07** local definition discovery、precedence、content hash承認、変更時再承認を実装・testする。依存: W03, W02-05。証跡: 未記録
- [ ] **W04-08** registry release generatorのvalidation、manifest、signature、tag入力検査を実装・testする。秘密鍵をrepository外から受ける。依存: W04-02。証跡: 未記録
- [ ] **W04-09** orphan branch作成、公開鍵commit後switch、manifest生成、署名、tag発行の手順をdry-run評価する。依存: W04-08。証跡: 未記録
- [ ] **W04-10** maintainer CLI `gdtvm-registry`の`key inspect`, `key add`, `manifest build`, `manifest verify`, `release check`を07章4.3節どおり実装し、秘密鍵拒否・非漏えい、trust store atomic更新、positive/negative署名をtestする。依存: W04-01～W04-09。証跡: 未記録
- [ ] **W04-11** registryの`keys.toml`と`revoked.toml`を07章4.1節どおりstrict実装し、鍵rotation、空失効一覧、severity別install/runtime/doctor動作、offline/force非回避をtestする。依存: W04-01～W04-10。証跡: 未記録

## 10. W05 HTTP・download・archive・外部process

- [ ] **W05-01** Windows HTTP transport、proxy、TLS、timeout、retry、redirect再検査、response上限を実装・testする。依存: W01-03, W02-02。証跡: 未記録
- [ ] **W05-02** `.part`、stream hash、Range再開、ETag/Last-Modified、server非対応時再開を実装・testする。依存: W05-01。証跡: 未記録
- [ ] **W05-03** zip/tar/gzip/xz等の必要archive形式、entry事前列挙、path traversal、衝突、bomb上限を実装・testする。依存: W01-03。証跡: 未記録
- [ ] **W05-04** staging root、permission正規化、atomic commit、cleanup、failure rollbackを実装・testする。依存: W02-06, W05-03。証跡: 未記録
- [ ] **W05-05** Windows ProcessRunnerのargv分離、shellなし、cwd/env/stdin/stdio/timeout、process tree終了、output上限を実装・testする。依存: W01-03。証跡: 未記録
- [ ] **W05-06** OS componentをSystem32等の既知pathから解決し、PATH hijackを防ぐprobeを実装・testする。依存: W05-05。証跡: 未記録
- [ ] **W05-07** managed helperをdownload、SHA-256検証、展開、probe、atomic cache、receipt化して絶対entrypointで実行する。依存: W03-10, W05-02～W05-05。証跡: 未記録
- [ ] **W05-08** supplemental artifactとbackend bootstrapの取得・検証・計画表示・監査を実装・testする。依存: W03, W05-02, W05-05。証跡: 未記録
- [ ] **W05-09** third-party、unverified、hook、外部programのApproval policyと非対話拒否を実装・testする。依存: W01-06, W05-07。証跡: 未記録

## 11. W06 Catalog・導入・削除・修復

- [ ] **W06-01** refreshの単体/全tool、並列上限、部分成功、cache revisionを実装・testする。依存: W03-03, W05-01。証跡: 未記録
- [ ] **W06-02** exact versionと`--latest`解決、Plan fingerprint、TTL、stale拒否を実装・testする。依存: W03, W02-05。証跡: 未記録
- [ ] **W06-03** Resolve→Plan→Approve→Execute→Commit→Cleanup状態機械とevent/journalを実装する。依存: W01-06, W02-05, W05。証跡: 未記録
- [ ] **W06-04** archive方式install、依存install、probe、receipt、冪等再実行を実装・testする。依存: W03-06～W03-08, W06-03。証跡: 未記録
- [ ] **W06-05** rustup backend方式の完全selector、managed home、inventory、rollback、receiptを実装・testする。依存: W03 backend, W05-08, W06-03。証跡: 未記録
- [ ] **W06-06** cancellation、download/process/lock応答、commit区間、cancelled-with-commitを実装・testする。依存: W06-03。証跡: 未記録
- [ ] **W06-07** uninstallの参照検査、force、trash rename、shared保持、backend除去を実装・testする。依存: W06-04, W06-05。証跡: 未記録
- [ ] **W06-08** doctor全診断項目とdeep/network制約を実装・testする。依存: W02～W06。証跡: 未記録
- [ ] **W06-09** repair plan、dry-run、safe repair、unsafe拒否、journal回復を実装・testする。依存: W06-08。証跡: 未記録

## 12. W07 Windows選択・junction・shim・shell

- [ ] **W07-01** user/project/exec/disabled優先順位とEffectiveSelection説明を実装・testする。依存: W02-03, W06-04。証跡: 未記録
- [ ] **W07-02** Windows junctionの作成、target検査、一般ユーザー操作、短い置換欠落、shim-only fallbackを実装・testする。依存: W01-03, W02-04。証跡: 未記録
- [ ] **W07-03** selection state、junction、shim index、shell snapshotのtransactionとrepairを実装・failure testする。依存: W07-01, W07-02。証跡: 未記録
- [ ] **W07-04** multi-call shim、argv[0]、command owner、一意選択、再帰防止、direct targetを実装・testする。依存: W03-08, W07-01。証跡: 未記録
- [ ] **W07-05** Windows hardlink shimとsmall fallback shim、client更新時再生成を実装・testする。依存: W07-04。証跡: 未記録
- [ ] **W07-06** native/cmd-script/PowerShell launcher、固定args、UTF-8 codepage、stdio/exit code/signal透過を実装・testする。依存: W05-05, W07-04。証跡: 未記録
- [ ] **W07-07** environment DAG merge、PATH重複、case-insensitive key、conflict、安定current pathを実装・testする。依存: W03-08, W07-01。証跡: 未記録
- [ ] **W07-08** cmd AutoRunの安全解析、marker、backup、既存command保護、undoを実装・testする。依存: W02-05。証跡: 未記録
- [ ] **W07-09** Windows PowerShell 5.1/PowerShell 7 profileのmarker、backup、execution policy非変更既定、undoを実装・testする。依存: W02-05。証跡: 未記録
- [ ] **W07-10** Windows shell environment snapshotとVS Code `code.exe .`継承を実装・testする。依存: W07-07～W07-09。証跡: 未記録
- [ ] **W07-11** completionのcmd/Windows PowerShell/pwsh出力を実装・offline testする。依存: W07-08, W07-09。証跡: 未記録

## 13. W08 Application Service・event・Wails境界

- [ ] **W08-01** 10章の全read operation request/resultを実装・contract testする。依存: W02～W07。証跡: 未記録
- [ ] **W08-02** 全PlanX/ExecutePlan、approval回答完全性、二重実行拒否を実装・testする。依存: W06。証跡: 未記録
- [ ] **W08-03** Event schema、sequence、coalesce、必須event非欠落を実装・testする。依存: W01-06, W06-03。証跡: 未記録
- [ ] **W08-04** operation lifecycle、Start/Get/Cancel/Subscribe、ring buffer/gapをUI非依存adapterで実装・testする。依存: W08-02, W08-03。証跡: 未記録
- [ ] **W08-05** Service boundaryにCLI/TOML/Wails/OS concrete typeが漏れていないことをarchitecture test/reviewする。依存: W08-01～W08-04。証跡: 未記録
- [ ] **W08-06** shim minimal Serviceがnetwork、installer、prompt、write portを持たないことをtestする。依存: W07-04, W08-01。証跡: 未記録

## 14. W09 CLI・表示・国際化

- [ ] **W09-01** global option、位置、排他、case、alias、完全version入力規則を実装・table testする。依存: W08。証跡: 未記録
- [ ] **W09-02** `setup`, `tools`, `available`, `refresh`を薄いadapterとして実装・contract testする。依存: W09-01。証跡: 未記録
- [ ] **W09-03** `install`, `uninstall`, `installed`, `use`, `disable`, `current`を実装・contract testする。依存: W09-01。証跡: 未記録
- [ ] **W09-04** `registry update`, `doctor`, `repair`, `exec`, `completion`, `version`を実装・contract testする。依存: W09-01。証跡: 未記録
- [ ] **W09-05** text/table、stdout/stderr、TTY progress、quiet/verbose/color、promptを実装・golden testする。依存: W09-02～W09-04。証跡: 未記録
- [ ] **W09-06** JSON document/NDJSON event、exec/completion禁止、error envelopeを実装・schema testする。依存: W08-03, W09-02～W09-04。証跡: 未記録
- [ ] **W09-07** ja/en help、message、placeholder、幅、encoding、Windows console表示を評価する。依存: W01-07, W09-05。証跡: 未記録
- [ ] **W09-08** typo提案、remediation、終了code、exec child code透過をtestする。依存: W01-05, W09。証跡: 未記録
- [ ] **W09-09** CLI packageに禁止責務がないことをstatic review/testする。依存: W09-02～W09-08。証跡: 未記録

## 15. W10 Windows標準registry・17ツール

各項目はtool TOML作成、strict parse、catalog、最新stable＋旧stable、artifact/digest、install、probe、2版切替、shim、uninstall、offline cacheをWindows amd64標準ユーザーで確認する。条件付きplatformは未検証assetを公開せず、理由を記録する。

- [ ] **W10-01** `android-sdk.toml`：mutable SDK root、latest layout、sdkmanager/avdmanager、Android環境を評価する。依存: W03～W09。証跡: 未記録
- [ ] **W10-02** `bazel.toml`：公式release asset、digest、単一実体/zip、cacheを評価する。証跡: 未記録
- [ ] **W10-03** `cmake.toml`：top-level正規化、cmake/ctest/cpack/cmake-guiを評価する。証跡: 未記録
- [ ] **W10-04** `dart.toml`：SDK archive、PUB_CACHE、公開commandを評価する。証跡: 未記録
- [ ] **W10-05** `dotnet.toml`：SDK、検証済みNuGet補助artifact、分離cacheを評価する。証跡: 未記録
- [ ] **W10-06** `flutter.toml`：stable archive、mutable cache、PUB_CACHE、dart command非競合を評価する。証跡: 未記録
- [ ] **W10-07** `go.toml`：公式metadata、GOROOT/GOPATH/GOBIN/cache、go/gofmtを評価する。証跡: 未記録
- [ ] **W10-08** `gradle.toml`：distribution、JDK依存、GRADLE_HOME/cache、script launcherを評価する。証跡: 未記録
- [ ] **W10-09** `jdk.toml`：Temurin 11/17/21、asset選択、JAVA_HOME、command群を評価する。証跡: 未記録
- [ ] **W10-10** `kotlin.toml`：compiler archive、JDK依存、bat launcher、command群を評価する。証跡: 未記録
- [ ] **W10-11** `llvm.toml`：7-Zip helper、self-extracting展開、LIBCLANG_PATH、競合を評価する。証跡: 未記録
- [ ] **W10-12** `mingw.toml`：long ID、7z、compiler layout、競合を評価する。証跡: 未記録
- [ ] **W10-13** `ninja.toml`：単一exe、内蔵shim、PATH、引数、codepage 65001を評価する。証跡: 未記録
- [ ] **W10-14** `node.toml`：index JSON、SHASUMS、node/npm/npx/corepack固定targetを評価する。証跡: 未記録
- [ ] **W10-15** `python.toml`：WiX helper、MSI抽出、ensurepip、python/pip commandを標準ユーザーで評価する。証跡: 未記録
- [ ] **W10-16** `rust.toml`：rustup、GNU legacy variant、CARGO/RUSTUP_HOME、selector、任意sccacheを評価する。証跡: 未記録
- [ ] **W10-17** `winlibs.toml`：GCC/LLVM long ID、7z、layout、LIBCLANG_PATH、競合を評価する。証跡: 未記録
- [ ] **W10-18** `helpers/seven-zip.toml`の完全版、digest、license、receipt、共有cacheを評価する。証跡: 未記録
- [ ] **W10-19** `helpers/wix.toml`の完全版、digest、dark.exe entrypoint、licenseを評価する。証跡: 未記録
- [ ] **W10-20** 17 tool/2 helper/schema/message/license/key/revocationをmanifestへ完全収録し署名検証する。証跡: 未記録
- [ ] **W10-21** tool固有Go分岐がないこととTOML-only変更testを完了し、W03-12を判定する。証跡: 未記録

## 16. W11 Windows integration・E2E評価

- [ ] **W11-01** clean portable folderで`setup`→registry bootstrap→install→use→current→shim→uninstallを評価する。依存: G-WIN-TOOLS。証跡: 未記録
- [ ] **W11-02** user modeを標準ユーザーで評価し、HKLM/system PATH/UACを使用しないことを確認する。証跡: 未記録
- [ ] **W11-03** cmd、Windows PowerShell 5.1、PowerShell 7でsetup/再実行/backup/removeを評価する。証跡: 未記録
- [ ] **W11-04** NTFS junction成功、junction不可時shim-only、hardlink不可時small shim fallbackを評価する。証跡: 未記録
- [ ] **W11-05** `.gdtvm.toml`探索、Git境界切替、project disabled、user fallback、exec overrideを評価する。証跡: 未記録
- [ ] **W11-06** cmd/PowerShellから`code.exe .`を起動し、VS Code本体、統合terminal、拡張processの環境継承を評価する。証跡: 未記録
- [ ] **W11-07** 2版切替でtool tree copyがなく、junctionまたはshim resolverが新versionを参照することを評価する。証跡: 未記録
- [ ] **W11-08** offline install、cache不足、registry取得失敗、previous snapshot継続を評価する。証跡: 未記録
- [ ] **W11-09** interrupted download、process failure、commit failure、電源断相当、journal repairをfailure injectionで評価する。証跡: 未記録
- [ ] **W11-10** concurrent install/refresh/use、lock順序、partial success、cancelを評価する。証跡: 未記録
- [ ] **W11-11** third-party/unverified/local definition/hook/profile変更の対話・`--yes`・非対話policyを評価する。証跡: 未記録
- [ ] **W11-12** archive traversal、digest mismatch、signature不正、PATH hijack、secret maskを攻撃fixtureで評価する。証跡: 未記録

## 17. W12 Windows品質・合格判定

- [ ] **W12-01** 全unit test、race test、integration test、contract test、E2Eを実行し結果を保存する。依存: W11。証跡: 未記録
- [ ] **W12-02** coverageを取得し、security/path/state/transaction/parserの未試験branchを解消または判断記録する。証跡: 未記録
- [ ] **W12-03** `go vet`、format、lint、dependency/license/vulnerability scanを合格させる。証跡: 未記録
- [ ] **W12-04** current/shim性能、download memory、large archive、log rotation、disk上限を評価する。証跡: 未記録
- [ ] **W12-05** Windows amd64 release build、metadata、SBOM、checksum、署名、再現性を評価する。証跡: 未記録
- [ ] **W12-06** Windows arm64をnative runnerまたは承認済み評価方法でbuild/testし、未評価範囲を明記する。証跡: 未記録
- [ ] **W12-07** 01章の旧機能対応表と17 tool固有監査を行単位でsign-offする。証跡: 未記録
- [ ] **W12-08** open defectをblocker/Windows限定/Linuxで確認/延期へ分類し、G-WIN-E2Eを判定する。証跡: 未記録
- [ ] **W12-09** G-LINUX-STARTの開始判断とWindows baseline commit/registry tagを記録する。証跡: 未記録
- [ ] **W12-10** 全Go packageのドキュメントコメント、export宣言コメント、security・transaction・platform固有処理の意図説明コメントをreviewし、陳腐化、自明な反復、追跡先のないTODOがないことを確認する。証跡: 未記録

## 18. L01 Linux開発環境・platform実装

- [ ] **L01-01** Linux評価matrixを固定する。glibc/musl、amd64/arm64、distribution、filesystem、bash/zsh/fish、非rootを含める。依存: G-LINUX-START。証跡: 未記録
- [ ] **L01-02** CGO無効Linux amd64/arm64 buildとmusl環境でのclient起動を評価する。証跡: 未記録
- [ ] **L01-03** Linux user/portable path、permission、umask、XDG、filesystem containmentを実装・testする。証跡: 未記録
- [ ] **L01-04** symlink/hardlink、atomic rename、file lock、process signal/tree、executable permissionを実装・testする。証跡: 未記録
- [ ] **L01-05** libc判定をOS metadataとELF interpreterで実装しunknownを安全に扱う。証跡: 未記録
- [ ] **L01-06** tar系archiveのLinux symlink安全性、case-sensitive path、permission正規化を評価する。証跡: 未記録
- [ ] **L01-07** Linux system prerequisite/interpreterを絶対pathと版でprobeし、package manager/sudoを起動しないことをtestする。証跡: 未記録

## 19. L02 Linux shell・shim・runtime

- [ ] **L02-01** Linux multi-call shim、symlink/hardlink、argv[0]、signal/exit codeを評価する。依存: L01。証跡: 未記録
- [ ] **L02-02** sh-script/bash interpreter固定、PATH非依存、missing prerequisite hintを評価する。証跡: 未記録
- [ ] **L02-03** bash profile marker、backup、idempotence、removeを実装・testする。証跡: 未記録
- [ ] **L02-04** zsh profile marker、backup、idempotence、removeを実装・testする。証跡: 未記録
- [ ] **L02-05** fish startup integration、環境snapshot、completion、removeを実装・testする。証跡: 未記録
- [ ] **L02-06** Linux環境変数case-sensitive merge、PATH、current symlink、project selectionを評価する。証跡: 未記録
- [ ] **L02-07** bash/zsh/fishからVS Codeを起動する対応範囲を評価し、GUI起動差を記録する。証跡: 未記録

## 20. L03 Linux core回帰

- [ ] **L03-01** config/state/registry/catalog/install/selection/shimのplatform-neutral testをLinuxで再実行する。依存: L01, L02。証跡: 未記録
- [ ] **L03-02** registry bootstrap、署名、offline、local definition、rollbackをLinux非rootで評価する。証跡: 未記録
- [ ] **L03-03** download、resume、archive、external process、cancel、lock、repairをLinuxで評価する。証跡: 未記録
- [ ] **L03-04** JSON/NDJSON、ja/en、UTF-8、TTY/non-TTY、signal終了codeを評価する。証跡: 未記録
- [ ] **L03-05** Linux固有実装がWindows contractを変更していないことをWindows CI再実行で確認する。証跡: 未記録

## 21. L04 Linux E2E基盤

- [ ] **L04-01** portable mode初心者scenarioを非root clean homeで評価する。依存: L03。証跡: 未記録
- [ ] **L04-02** user mode/XDG、shell setup/remove、project固定、execを評価する。証跡: 未記録
- [ ] **L04-03** glibc環境とmusl環境でclient起動、platform選択、unsupported reasonを評価する。証跡: 未記録
- [ ] **L04-04** offline、third-party warning、system prerequisite不足、security攻撃fixtureを評価する。証跡: 未記録
- [ ] **L04-05** interrupted operation、concurrency、repair、portable root移動を評価する。証跡: 未記録

## 22. L05 Linux標準tool評価

各項目は12章のplatform表に従い、必須は最低2版、条件は公式検証可能assetがある版、第三者は警告・license・digestを評価する。非対応の`mingw`と`winlibs`は誤ったversionを表示せず理由を返す。

- [ ] **L05-01** Android SDK Linux amd64必須、arm64条件を評価する。依存: G-LINUX-CORE。証跡: 未記録
- [ ] **L05-02** Bazel Linux amd64必須、arm64条件を評価する。証跡: 未記録
- [ ] **L05-03** CMake Linux amd64必須、arm64条件を評価する。証跡: 未記録
- [ ] **L05-04** Dart Linux amd64必須、arm64条件を評価する。証跡: 未記録
- [ ] **L05-05** .NET Linux amd64/arm64必須とNuGet補助artifact非必須を評価する。証跡: 未記録
- [ ] **L05-06** Flutter Linux amd64必須、arm64条件とbash prerequisiteを評価する。証跡: 未記録
- [ ] **L05-07** Go Linux amd64/arm64必須を評価する。証跡: 未記録
- [ ] **L05-08** Gradle Linux amd64/arm64必須とJDK依存を評価する。証跡: 未記録
- [ ] **L05-09** JDK Linux amd64/arm64必須を評価する。証跡: 未記録
- [ ] **L05-10** Kotlin Linux amd64/arm64必須とJDK/bash依存を評価する。証跡: 未記録
- [ ] **L05-11** LLVM Linux amd64/arm64条件を評価する。証跡: 未記録
- [ ] **L05-12** MinGWをLinux非対応として正しく診断する。証跡: 未記録
- [ ] **L05-13** Ninja Linux amd64必須、arm64条件を評価する。証跡: 未記録
- [ ] **L05-14** Node.js Linux amd64/arm64必須を評価する。証跡: 未記録
- [ ] **L05-15** Python third-party portable amd64/arm64の警告、provider、license、digest、target tripleを評価する。証跡: 未記録
- [ ] **L05-16** Rust Linux amd64/arm64必須、完全selector、managed homeを評価する。証跡: 未記録
- [ ] **L05-17** WinLibsをLinux非対応として正しく診断する。証跡: 未記録
- [ ] **L05-18** Linux platformを含むregistry manifest、catalog、全required probeを検証する。証跡: 未記録

## 23. L06 Linux品質・合格判定

- [ ] **L06-01** Linux unit/race/integration/contract/E2Eを全実行し証跡を保存する。依存: G-LINUX-TOOLS。証跡: 未記録
- [ ] **L06-02** Linux amd64/arm64 build、CGO無効、SBOM、checksum、署名、再現性を評価する。証跡: 未記録
- [ ] **L06-03** glibc/musl、非root、shell、filesystem別の未評価範囲を確認する。証跡: 未記録
- [ ] **L06-04** Linux性能、memory、large archive、signal/cancel、permissionを評価する。証跡: 未記録
- [ ] **L06-05** open defectを分類しG-LINUX-TOOLSを判定する。証跡: 未記録

## 24. X01 クロスプラットフォーム総合評価

- [ ] **X01-01** Windows/Linuxで同じconfig、definition、state schema、error/event/JSON contractを使用していることを確認する。依存: L06。証跡: 未記録
- [ ] **X01-02** OS別path、case、link、launcher、libc以外に不要なplatform分岐がないことをreviewする。証跡: 未記録
- [ ] **X01-03** Windows変更後Linux、Linux変更後Windowsの双方向CI gateを構成する。証跡: 未記録
- [ ] **X01-04** 17 toolのplatform対応表と実registry entryの一致を検査する。証跡: 未記録
- [ ] **X01-05** local TOMLだけで追加した同一fixture toolをWindows/Linux双方で実行する。証跡: 未記録
- [ ] **X01-06** Wails bridge向けPlan/Start/Get/Cancel/Subscribe contractをheadless testする。証跡: 未記録
- [ ] **X01-07** telemetryなし、外部通信先、log mask、license noticeを両OSで監査する。証跡: 未記録
- [ ] **X01-08** G-CROSSを判定する。証跡: 未記録

## 25. R01 Release・配布・運用評価

- [ ] **R01-01** 日本時間のrelease日と日次通番からrepository rootの`/VERSION`へclient version `YYYY.mm.DD.XX`を確定し、commit、build time、Go version、schema versionとともに再現可能に埋め込む。`00`開始、同日increment、実在日、全表示・state・archive・tag一致をtestする。依存: G-CROSS。証跡: 未記録
- [ ] **R01-02** Windows amd64/arm64、Linux amd64/arm64のrelease artifact、checksum、SBOM、signatureを生成・検証する。証跡: 未記録
- [ ] **R01-03** Windows Defender/SmartScreen注意とLinux実行permissionを含む導入文面を検証する。証跡: 未記録
- [ ] **R01-04** registry鍵生成、repository外保存、公開鍵code登録、commit、registry switch、manifest/signature生成を実地確認する。証跡: 未記録
- [ ] **R01-05** orphan `registry` branch、17 tool、2 helper、schema、message、license、key、revocationをrelease generatorで検証する。証跡: 未記録
- [ ] **R01-06** immutable `registry-vX.Y.Z` tag、commit SHA pin、GitHub Release asset一致を検証する。証跡: 未記録
- [ ] **R01-07** clean machineでexeだけからregistry bootstrapし、Windows→Linuxの順でsmoke testする。証跡: 未記録
- [ ] **R01-08** client update、registry update、旧snapshot保持、downgrade拒否、repair fallbackを評価する。証跡: 未記録
- [ ] **R01-09** release checklist、known issues、supported matrix、license notice、security連絡先を確定する。証跡: 未記録
- [ ] **R01-10** 17章に従いrepository rootの日本語`README.md`と`USER_GUIDE.md`を作成し、badge、導入、基本操作、build、全command、設定、外部program、registry署名鍵を記載する。証跡: 未記録
- [ ] **R01-11** README/USER_GUIDEのlink、anchor、badge、全command例、build再現、鍵positive/negative test、secret非混入をWindows→Linuxの順で評価する。証跡: 未記録
- [ ] **R01-12** G-RELEASEとG-DONEを判定し、最終スナップショットを`完了`へ更新する。依存: R01-01～R01-11。証跡: 未記録

## 26. 仕様章別カバレッジ確認

各章の要求が少なくとも1件の完了タスクと試験証跡へ結び付いたことを最終確認する。

- [ ] **COV-01** 01 製品要求・旧機能対応：W10～W12、L05、X01
- [ ] **COV-02** 02 アーキテクチャ：W01、W08、W09-09
- [ ] **COV-03** 03 保存・状態：W02、W06、L01
- [ ] **COV-04** 04 CLI：W09、W11、L03
- [ ] **COV-05** 05 設定：W02、W11-05、L03
- [ ] **COV-06** 06 定義schema：W03、W10、L05
- [ ] **COV-07** 07 registry：W04、W10-20、R01
- [ ] **COV-08** 08 install/runtime：W05～W07、W11、L03
- [ ] **COV-09** 09 platform/shell：W07、W11、L01～L02
- [ ] **COV-10** 10 内部API/Wails境界：W08、X01-06
- [ ] **COV-11** 11 security：W04、W05-09、W11-11～W11-12、X01-07
- [ ] **COV-12** 12 標準tool：W10、L05
- [ ] **COV-13** 13 品質/release：W12、L06、X01、R01
- [ ] **COV-14** 14 data contract：W02-05、W03-10、W06-03、W09-06
- [ ] **COV-15** 15 完全定義fixture：W03-11～W03-12、X01-05
- [ ] **COV-16** 16 進捗運用：停止・再開記録と全gate
- [ ] **COV-17** 17 GitHub公開文書：R01-10～R01-11

## 27. 最終未完了一覧

完了時に未チェック項目がある場合だけ記入する。仕様上必須の項目を「時間不足」で延期してG-DONEにしてはならない。

| タスクID | 未完了理由 | 利用者影響 | 回避策 | 承認者 | 期限・追跡先 |
|---|---|---|---|---|---|
| なし | なし | なし | なし | 未設定 | なし |
