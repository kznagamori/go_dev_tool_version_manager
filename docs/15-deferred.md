# 延期scopeと再導入gate

## 1. 目的と使い方

本章はv0.1で意図的に**仕様から削除**した機能を記録する。目的は3つ。

1. 「検討していない」ことと「検討した上で延期した」ことを区別する。
2. 再導入するとき、同じ調査をやり直さずに済むようにする。
3. 実装したくなったとき、どの手順から始めるかを一意に決める。

番号付き仕様（`01`〜`14`）に本章の機能を予約key、未使用enum値、コメントとして先行実装しない。実装は必ず§3の再導入手順から始める。

延期機能を1件実装する要求が来たら、まず本章の該当節を読み、`v0.1での代替`で目的が達成できないことを確認してから§4のpromptを使う。

## 2. 延期機能一覧

| ID | 機能 | 主な延期理由 | v0.1での代替 |
|---|---|---|---|
| [D-01](#d-01-self-update) | `self-update` | 単一機能として最大の複雑さ（apply子process、状態遷移、rollback） | bootstrap script再実行 / 手動archive展開 |
| [D-02](#d-02-再現可能build) | byte単位で再現可能なbuild | build環境侵害の検知は企業向け要件 | `checksums.txt`と公開後再download検査 |
| [D-03](#d-03-use---systemと外部tool-identity) | `use <tool> --system` と `file_identity` | 探索・identity固定の実装量が大きく、mtime依存で誤検知しやすい | shim directoryを外したPATHでsystem toolを直接使う |
| [D-04](#d-04-追加command) | `exec` `refresh` `tools` `disable` `repair` `completion` | v0.1の4操作に不要 | 各節の代替commandを参照 |
| [D-05](#d-05-revocation) | `revoked.toml` と実行時revocation | 更新手段（self-update）がないv0.1では実効性がない | 問題entryを外した新clientのreleaseと告知 |
| [D-06](#d-06-checksum非提供artifact) | `checksum.kind="not-provided"` | 標準4 toolが全てchecksumを公開している | checksum公開providerのartifactだけを採用 |
| [D-07](#d-07-windows-shell-profile-integration) | Windows PowerShell profile方式 | `user-path`がcmd/PowerShell/GUIを全て覆う | `user-path` または `none` |
| [D-08](#d-08-operation-journal) | `state/operations/*.jsonl` | staging＋atomic renameとindex再構築で整合性を保てる | operation tmpのdirectory単位削除 |
| [D-09](#d-09-schema-migration機構) | schema revision migration | schema 1しか存在しない | 未知schemaはfail closed |
| [D-10](#d-10-install-step-dag) | install stepのDAGと任意step種別 | 標準4 toolが全てdownload→verify→extractの同一形 | `[platforms.install]`の展開parameter |
| [D-11](#d-11-plan-fingerprintとttl) | Plan全体のcanonical digestとTTL | 承認すり替え対策は企業向け要件 | Planの`inputs`再検査とapproval category集合 |
| [D-12](#d-12-多言語) | 日本語以外の表示言語 | 対象利用者が日本語 | message ID機構は維持済み |
| [D-13](#d-13-payload全fileのmanifest) | payload全regular fileのSHA-256 | 記録数が数千件になる割に、実害は実行対象の破損 | `command_targets`のsize/SHA-256 |
| [D-14](#d-14-性能目標と計測基盤) | 起動・解決・lookupの数値目標 | 計測基盤の構築コストが機能価値を上回る | なし（体感で判断） |
| [D-15](#d-15-状態変更commandの--json) | `setup`/`install`/`use`/`uninstall`の`--json` | internal typed Planとは別に、安定した公開Plan/result JSON契約が必要 | 終了code（13値）と読取り5 commandの`--json` |
| [D-16](#d-16-gui) | GUI | CLIが安定してから | CLI |
| [D-17](#d-17-追加platform) | Windows/Linux arm64、Linux musl、macOS | CI runnerとupstream artifactの調査が未了 | 非対応（fail closed） |
| [D-18](#d-18-追加tool) | Go/Node.js/Python/.NET SDK以外 | 標準4 toolの安定を優先 | 非対応 |
| [D-19](#d-19-再配置できないpayload) | `relocation="reinstall-required"` | 標準4 toolが全て再配置可能 | 全payloadを再配置可能と仮定 |
| [D-20](#d-20-portable-root移動サポート) | setup後のroot移動と自動再生成 | `repair`（D-04）に依存する | `gdtvm setup` の再実行 |
| [D-21](#d-21-追加archive形式) | `tar.xz` 等 | Go標準ライブラリ外の圧縮moduleが必要 | Node.js Linuxは`.tar.gz`を採用 |
| [D-22](#d-22-archive-entry-filter) | definitionによるarchive entry include/exclude | 標準4 toolは全entryを展開でき、未使用fieldになる | 安全検査後に全entryを展開 |
| [D-23](#d-23-plan完全列挙) | Planの読書き完全列挙（`reads[]`/網羅`writes[]`/`rollback[]`） | 個人利用の承認・診断には利用者可視変更の列挙と封じ込め検査で足りる | `inputs`再検査＋封じ込め検査＋利用者可視`writes[]` |
| [D-24](#d-24-download再開) | download Range再開 | partial照合の実装量に対し、やり直しが簡潔 | 最初から再取得。完了済みcacheは再利用 |
| [D-25](#d-25-availableのfilter-option) | `available`の`--channel`/`--lifecycle` filter | 全件表示で目的を満たす | 全件表示（channel/lifecycle列を常に表示） |
| [D-26](#d-26-並行download) | 並行download設定（`download.concurrency`） | v0.1に並行downloadを使う操作がない | 逐次download固定 |

---

### D-01 self-update

**削除した内容**: `gdtvm self-update [--version] [--check]`、apply子processによるdistribution置換、`state/update.toml`、forward 8状態 / move 5状態の遷移、one-time tokenによる親process終了待ち、直前1世代のrollback保持、`cache/updates/`。

**v0.1での代替**: 更新は`install.ps1`/`install.sh`の再実行、または新しいrelease archiveの手動展開で行う。bootstrapは検証済みstagingからactive distributionを置換するが、開発版では旧distributionのbackup/自動rollbackを持たない。distribution隣接の利用者`gdtvm.toml`だけはbyte完全で新distributionへ引き継ぐ。旧版へ戻す場合は完全client versionを指定して再取得する。公開文書はこれを明記し、存在しない`self-update`を案内しない。

**再導入gate**:
1. rollbackが必要な失敗点（親未終了、move途中、verify失敗、次回起動時の未完了状態）を列挙し、各点での回復先を決める。
2. 状態遷移とその永続先を決める。D-08を戻すのか、`update.toml`単独で足りるのかを先に決定する。
3. Windows（実行中binaryを置換できない）とLinuxで同じ状態機械が成立することを示す。
4. client/registry混在状態を公開しないことをfailure injection testで証明する。
5. downgrade拒否、devel build拒否、asset identity再確認をtestする。

**影響する文書**: `03`(command追加)、`04`(update.toml契約)、`08`(transaction)、`10`(client release検証)、`11`(release手順/E2E)、`12`(更新方法)、`13`(task追加)。

---

### D-02 再現可能build

**削除した内容**: `SOURCE_DATE_EPOCH`のtag commit由来算出、entry順・timestamp・uid/gid固定、gzip header/ZIP metadata/Go build IDの決定化、別clean runnerでのrebuild digest一致というrelease gate。

**v0.1での代替**: archiveの完全性は`checksums.txt`のSHA-256と、公開後に全assetを再downloadして再検査する手順で担保する。

**再導入gate**:
1. 何を検知したいのか（build環境侵害 / 誤ったsource / 非決定的toolchain）を明示する。
2. 2 runnerでのdigest一致をCI jobとして安定して通せることを実験で示す。
3. 失敗時にreleaseを止める運用と、原因切り分け手順を決める。

**影響する文書**: `11`(build/archive/release手順)、`13`。

---

### D-03 `use --system`と外部tool identity

**削除した内容**: `gdtvm use <tool> --system`、definitionの`[platforms.system]`（`primary_command`/`candidates`/`probe_ids`）、PATH候補列挙とgdtvm shim除外、Windows reparse検査・Linux ELF arch判定・symlink chain追跡、`file_identity`（Windows: volume serial + file ID + mtime + owner SID、Linux: symlink chain全entryのcanonical JSON digest）、required siblingの固定、`selections.toml`のsystem entry、`display_origin="system/external"`、identity変化時のfail closed。

**v0.1での代替**: gdtvmのshim directoryを含まないPATHでsystem toolを直接使う。project固定を使う場合は`.gdtvm.toml`から該当toolを外す。

**再導入gate**:
1. **`file_identity`にmtimeを含めるかを先に再検討する。** v0.1の設計はアンチウイルスのスキャン、バックアップ復元、`touch`相当の操作で識別子が変わり、fail closedにより再選択を強制する。初心者が詰まる経路になるため、mtime除外・再検証の緩和・変化時の自動再probeのいずれかを選ぶ。
2. Windows/Linuxそれぞれで、候補が同一distributionに属することを何で判定するかを決める（root suffix除去、sibling probe、報告prefix）。
3. 候補0件・複数件・probe不合格の表示と、fallbackしないことの利用者への伝え方を決める。
4. 配布元provenanceを証明しない旨の表示（`system/external`）が誤解を生まない文言かをレビューする。
5. identity変化・消失・permission変化の各caseをE2Eで検証する。

**影響する文書**: `01`(用語)、`03`(option)、`04`(selections契約)、`06`(system table)、`07`(tool別sibling表)、`08`(selection)、`09`(PATH除外)、`10`(process/identity)、`13`。

---

### D-04 追加command

| command | 削除した内容 | v0.1での代替 |
|---|---|---|
| `exec` | `gdtvm exec <tool>@<version> -- <cmd>` | `.gdtvm.toml`のproject固定、またはpayload内実体のabsolute path |
| `refresh` | 明示的なcatalog更新command、`E_PARTIAL_SUCCESS`、`RefreshResult` | `available --refresh` |
| `tools` | tool一覧表示 | `installed` と `available <tool>` |
| `disable` | user/project selectionの無効化、project `disabled`配列 | `.gdtvm.toml`から該当toolを外す |
| `repair` | 診断結果に基づく自動修復Plan | `setup`再実行、`use`再実行、対象versionの再install |
| `completion` | shell補完script生成 | なし |

**再導入gate（共通）**:
1. その操作がv0.1の既存commandで代替できないことを、実際の利用者report（[14-maintenance.md](14-maintenance.md)§5）で示す。
2. 状態変更を伴う場合はPlan/approval/rollbackの契約を決める。
3. `--json`を持たせるかを決める。持たせる場合はD-15と同じ判断が必要。
4. `03`のcommand一覧、`12`の公開文書、`13`のtask、`11`のE2E scenarioを同時更新する。

`repair`を戻すときはD-20（portable root移動）も同時に検討する。

---

### D-05 revocation

**削除した内容**: `registry/revoked.toml`、catalog itemの`installable=false/revoked`、既存receiptに対する実行拒否、`--yes|--force`での迂回禁止。

**v0.1での代替**: 問題のあるartifactが判明した場合は、当該entryを外した新しいclient releaseを公開し、release noteと公開文書で告知する。

**再導入gate**:
1. registryの更新手段（D-01 self-update）が存在することを前提にする。存在しない状態で戻しても実効性がない。
2. 既存payloadを自動削除しない方針を維持するかを決める。
3. revocation解除の運用（新releaseでentry削除、理由をrelease noteへ）を決める。

**影響する文書**: `07`(registry tree)、`08`(version解決)、`10`(registry)、`13`。

---

### D-06 checksum非提供artifact

**削除した内容**: `checksum.kind="not-provided"`、receiptの`verification="sha256-recorded"`、Planのchecksum非提供warningとその承認category、初回offline導入の禁止規則、同一identityでのdigest変化の扱い。

**v0.1での代替**: upstream checksumを公開しているartifactだけを標準registryへ採用する。

**再導入gate**:
1. 実際に追加したいtoolがcheck sumを公開しないことを一次資料で示し、代替provider（公式mirror、別build）がないことを確認する。
2. Plan表示、承認category、receiptの検証状態表現、cache再利用条件、offline時の制約を同時に決める。
3. 「初回取得物の真正性は未検証」という表示が、`sha256-verified`と混同されない文言・レイアウトであることをsnapshot testで確認する。
4. 同一catalog identityで別digestを得た場合に自動更新しないことをtestする。

**影響する文書**: `04`(receipt/catalog)、`06`(checksum kind)、`07`(採用方針)、`08`(download)、`10`(artifact)、`13`。

---

### D-07 Windows shell-profile integration

**削除した内容**: Windowsの`--path-integration shell-profile`、PowerShell 5.1 / PowerShell 7のCurrentUserAllHosts profile path解決（Known Folder `FOLDERID_Documents`）、PowerShell literalのescape、cmd.exeに対する`E_UNSUPPORTED_SHELL`。

**v0.1での代替**: Windowsは`user-path`（cmd/PowerShell/GUIすべてを覆う）または`none`。

**再導入gate**:
1. `user-path`が使えない具体的な環境を、利用者reportで示す。
2. PowerShell 5.1と7のprofile pathを、shell childを起動せずに決定できることを確認する。
3. Linux側のmarker/escape/backup規則と共通化できるかを決める。

**影響する文書**: `03`(setup option)、`04`(setup.toml enum)、`09`(integration)、`12`、`13`。

---

### D-08 operation journal

**削除した内容**: `state/operations/<operation-id>.jsonl`、共通12 key、type 10種、state遷移表、sequence連番、`owned_paths`、before/after digest。

**v0.1での代替**: installはstagingへ全て作ってからatomic renameで公開するため、rename前の中断は未導入、rename後の中断は導入成功でindexだけ古い状態になる。indexはreceipt走査から再構築でき、operation tmpは`tmp/operations/<operation-id>/`のdirectory単位で削除できる。

**再導入gate**:
1. journalがないと復旧できない具体的な失敗sequenceを示す。多くはD-01（self-update）を戻すときに初めて必要になる。
2. 削除権限の根拠をjournalへ移すのか、現在のdirectory構造による保証を維持するのかを決める。
3. journal自体が破損した場合の扱いを決める。

**影響する文書**: `04`(JSON Lines契約、layout)、`08`(transaction)、`10`(削除の根拠)、`13`。

---

### D-09 schema migration機構

**削除した内容**: migration lock、sibling stagingへの新一式書出しとstrict再parse、old一式のowner-only backup、失敗時rollback、複数revisionの扱い。

**v0.1での代替**: schema 1だけを作成・読込みし、未知schemaはfail closed。

**再導入gate**:
1. old/new契約、default、不可逆点、必要disk量、rollbackを仕様化する。
2. old→newとfailure injectionのfixtureを先に用意する。
3. 複数revisionを飛び越す場合の扱いを決める。破損値を組込み既定へ黙って置換しない。

**影響する文書**: `04`(migration節)、`14`(拡張手順)、`13`。

---

### D-10 install step DAG

**削除した内容**: `[[platforms.install.steps]]`、step kind（`download` `verify-digest` `extract` `move` `set-output`）、`depends_on`、`{{outputs.<id>}}`、`{{staging}}`/`{{download}}` template root、cycle・未定義output・複数writerの検証。

**v0.1での代替**: engineが全toolで`download → verify → extract`を固定順で実行し、definitionは`[platforms.install]`の`strip_components`/`include`/`exclude`だけを与える。

**再導入gate**:
1. 固定順で表現できない具体的なtoolを示す（複数artifact、展開後の再配置、条件付きstep等）。
2. step kindを増やすのか、固定順のparameterを増やすのかを比較する。
3. DAGを戻す場合、cycle・未定義output・複数writer・download前extractの各negative testを先に用意する。

**影響する文書**: `06`(install節、template root)、`08`(展開)、`13`。

---

### D-11 Plan fingerprintとTTL

**削除した内容**: RFC 8785 JSON Canonicalizationによる`plan_sha256`、domain separationつきdigest、`input_fingerprints`（最大64件のmap）、Plan TTL 15分、Approvalとdigestの結合。

**v0.1での代替**: Planは`inputs`（root identity、config/project/definition/catalog/registry digest、selections/setup/receipt-index revision）を持ち、Executeは実体から再取得して一致を確認する。Approvalは`requires_explicit_approval=true`の`PlanWarningCode`集合を満たすかで判定する。

**再導入gate**:
1. `inputs`の再検査で防げない攻撃・事故を具体的に示す。
2. TTLを戻す場合、対話中にPlanが失効する体験が初心者にどう見えるかを設計する。
3. canonicalization実装の正しさをRFC 8785のtest vectorで検証する。

**影響する文書**: `04`(Plan契約)、`08`(承認)、`13`。

---

### D-12 多言語

**削除した内容**: `--lang ja|en`、`registry/messages/en.toml`、locale自動判定、locale間のplaceholder集合一致test。

**v0.1での代替**: 表示は日本語のみ。message ID機構とcatalog fileの構造は維持しているため、file追加と言語選択の実装だけで戻せる。

**再導入gate**: message IDの網羅性、placeholder集合の一致、no-color/狭幅でのlayout、locale判定の決定性をtestする。

**影響する文書**: `03`(global option)、`04`(catalog)、`05`(config)、`11`(test)、`12`。

---

### D-13 payload全fileのmanifest

**削除した内容**: receiptの`manifest`（payload全regular fileのpath/size/SHA-256）と、`doctor --deep`によるその全件照合。

**v0.1での代替**: receiptの`command_targets`が、required runtime commandのtargetとfixed argsが参照するpayload内fileだけを記録する。`doctor`はこれを照合する。

**再導入gate**:
1. 実行対象以外のpayload file破損が、実際に利用者へ影響したreportを示す。
2. install時のSHA-256計算コストとreceiptサイズ（Python約4,000 / Node約2,000ファイル）を許容するか決める。
3. 全件照合を`doctor`の既定にするか、明示optionにするかを決める。

**影響する文書**: `04`(receipt)、`08`(commit/doctor)、`10`、`13`。

---

### D-14 性能目標と計測基盤

**削除した内容**: warm `--version` 100 ms、warm shim resolve 150 ms、10,000 item lookup 200 msの各目標と、p50/p95・cold/warmの計測レポート。

**再導入gate**: 実測hostの条件を固定し、CIで安定して測れることを確認してから目標値を決める。security/正確性を弱めて達成しない。

**影響する文書**: `11`、`13`。

---

### D-15 状態変更commandの`--json`

**削除した内容**: `setup`/`install`/`use`/`uninstall`の`--json`、内部service resultを公開する`SetupResult` `StorageResult` `RefreshResult` `RepairResult` `UpdateResult` `ToolSummary`、human確認用typed Planを外部互換契約として出力する公開Plan JSON。

**v0.1での代替**: 状態変更の機械判定は終了code（0～12の13値、34 error codeとdoctor unhealthy専用値）で行い、詳細が必要なら直後に`installed --json` / `current --json`を呼ぶ。内部では[04-storage-and-data.md](04-storage-and-data.md)§16のtyped Planを厳密に使うが、そのJSON表現をCLI stdoutや互換APIとして公開しない。

**再導入gate**:
1. 終了codeと読取りcommandの組合せで足りない具体的なscriptを示す。
2. PlanをJSONへ落とす場合、D-11（fingerprint）を戻すかを同時に判断する。
3. warning/approvalのJSON表現が、human表示と同じ情報量を持つことをcontract testで示す。

**影響する文書**: `03`(option許可範囲)、`04`(envelope)、`11`、`12`、`13`。

---

### D-16 GUI

**削除した内容**: bridge DTO、event購読protocol、operation polling API。

**v0.1で維持しているもの**: Application Serviceがlocale-neutralであること、`ProgressSink`と`CancelToken`がrequest単位で渡ること、承認がadapter責務であること。GUIはこれらを再利用できる。

**再導入gate**: bridge DTO version、operation lifecycle、progress backpressure、cancel、approval、single-instance、error表示、CLIとのcontract一致を仕様化する。Go internal structを直接公開せず変換testを持つ。GUI完成前にCLI behaviorを変更しない。v0.1の読取り`--json`をGUI transportとして流用しない。

---

### D-17 追加platform

**削除した内容**: Windows/Linux arm64、Linux musl、macOSのplatform ID、tuple、artifact、CI runner。

**再導入gate**: host判定、upstream artifactの4 tool分、archive、shim/link、storage/native package、**CI runnerの入手可否**を調査する。platform IDとtupleを`06`/`04`で追加し、4 toolすべてが対応しない場合はtoolごとのunsupported理由を明示する。既存amd64 artifactをemulationで黙って選ばない。CI runnerを用意できないplatformを「対応」と表示しない。

---

### D-18 追加tool

Go/Node.js/Python/.NET SDK以外のtoolは[14-maintenance.md](14-maintenance.md)§3のT01〜T08で追加する。共有storageの可否を言語名の印象で決めない。

---

### D-19 再配置できないpayload

**削除した内容**: definitionの`relocation`（`portable|reinstall-required`）とreceiptの同field、再配置不可payloadに対するunhealthy判定。

**v0.1での代替**: 全payloadが再配置可能である前提を固定する。標準4 toolはこれを満たす。

**再導入gate**: 再配置できない具体的toolを示し、rootが移動した場合の検出方法（保存したroot pathとの比較）と利用者への案内を決める。D-20と同時に検討する。

---

### D-20 portable root移動サポート

**削除した内容**: setup後にportable rootのdirectoryを移動した場合の、current link / shim / integration snapshotの自動再生成。

**v0.1での代替**: `doctor`がroot不一致を検出し、`gdtvm setup`の再実行を案内する。`setup`は冪等なので再実行で作り直せる。

**再導入gate**: `repair`（D-04）を先に導入する。再生成の対象と、payload内absolute pathを書き換えないこと、D-19との関係を決める。

---

### D-21 追加archive形式

**削除した内容**: `format = "tar.xz"` とその展開。

**v0.1での代替**: Node.js Linuxは公式が同じ`SHASUMS256.txt`で配布する`.tar.gz`を採用する。schema 1は`zip`と`tar.gz`だけを扱い、圧縮のための外部moduleを持たない。

**再導入gate**:
1. `.tar.gz`等の代替が存在しないtoolを示す。
2. 追加する圧縮moduleのlicense、maintainer状況、既知脆弱性、transitive dependencyを[02-architecture.md](02-architecture.md)§17に従って記録する。
3. 展開時のsize/ratio上限とdecompression bombの検査が新形式でも成立することをtestする。

---

### D-22 archive entry filter

**削除した内容**: `[platforms.install]`の`include`/`exclude` glob配列と、選択entryだけを展開する処理。

**v0.1での代替**: 標準4 toolの採用archiveは全entryをpayloadへ展開し、全entryへtraversal、link、collision、size/count/ratio検査を適用する。不要entryを除くためのfilterは持たない。

**再導入gate**:
1. 全entry展開では導入不能または安全要件を満たせない標準tool/platformのfixtureを示す。
2. glob grammar、match前後の`strip_components`順序、directory親の暗黙包含、0件matchの失敗規則を確定する。
3. filter対象外entryも展開前安全検査とarchive bomb count/sizeへ含めるかを明記し、bypass negative testを作る。

**影響する文書**: `06`(install schema)、`08`(安全展開)、`11`(contract/security test)、`13`。

---

### D-23 Plan完全列挙

**削除した内容**: Planの`reads[]`（全入力fileのpath＋expected SHA-256の列挙）、`writes[]`による全書込み先（staging、cache、state、receipt、index、shim、storage）の網羅列挙と`before/after_sha256`、`rollback[]`配列、CI E2EでのPlan `writes[]`と実書込みの1件ずつの突合せ。

**v0.1での代替**: `inputs`（root identity、config/project/definition/catalog/registryのdigest、selection/setup/receipt-indexのrevision）を承認前とlock取得後に実体から再検査してstaleを防ぐ。`writes[]`は利用者可視の変更（integration対象、project file、current link）だけを列挙し、それ以外の書込みはExecuteの封じ込め検査と[11-quality-and-ci.md](11-quality-and-ci.md)§7.2の書込み範囲検査（許可root/role内であること）で保証する。rollbackはengine内部動作としてfailure injection testで検証する。

**再導入gate**:
1. 封じ込め検査で防げなかった事故・攻撃の具体例を示す。管理root内の意図しないfileを書き換えた不具合が実際に発生し、事前の全列挙があれば検出できた場合が該当する。
2. 列挙の粒度（path単位かrole単位か）と、`before/after_sha256`の必要性を分けて決める。
3. D-11（Plan fingerprint）と同時に導入するかを判断する。

**影響する文書**: `02`(Execute検査)、`04`(Plan契約)、`08`(Plan表示)、`10`(§13)、`11`(§6/§7.2)、`13`。

---

### D-24 download再開

**削除した内容**: partial download（`.part`）のURL/ETag/Last-Modified/expected size照合によるRange再開、`download.resume` config key、serverがRangeを無視した場合の0 byte再開規則。

**v0.1での代替**: 中断したdownloadのpartial fileは所有と作成時刻を検査して破棄し、次回実行時に最初から取得し直す。digest/identityが一致する完了済みcache fileの再利用は維持する。

**再導入gate**:
1. 全量再取得で実害が出る利用者report（大型artifactと不安定回線の組合せ）を示す。
2. URL identity、validator（ETag/Last-Modified）、expected sizeの全一致時だけ再開し、不一致partialを破棄することをfixtureで検証する。
3. 再開後のdigest計算が全量取得と一致することをtestする。

**影響する文書**: `04`(§5)、`05`(download key)、`08`(§5.1)、`10`(§10)、`11`(E2E scenario)、`13`。

---

### D-25 availableのfilter option

**削除した内容**: `available`の`--channel stable|prerelease`と`--lifecycle supported|eol|unknown` option、`ListAvailable` requestのchannel/lifecycle field。

**v0.1での代替**: `available <tool>`は全versionをchannel/lifecycle列付きのversion降順で表示する。機械処理は`--json`の全item出力を利用側でfilterする。

**再導入gate**:
1. 全件表示では不足する具体的な利用場面（表示量・目視性）を利用者reportで示す。
2. filter組合せと0件時のexit挙動を決める。
3. `03`のoption表、`02`のrequest field、testを同時更新する。

**影響する文書**: `02`(§7)、`03`(§3.2)、`13`。

---

### D-26 並行download

**削除した内容**: `download.concurrency` config key（1～8）と、複数downloadの並行実行制御。

**v0.1での代替**: 1つのoperationのdownload（artifact 1件と必要なchecksum文書）は逐次実行する。異なるInstallKeyの操作は別invocationとして並行でき、lock順（[02-architecture.md](02-architecture.md)§12）で保護する。

**再導入gate**:
1. 並行downloadを必要とする操作（複数tool一括install等）が先に仕様化されている。
2. progress表示、cancel、失敗時cleanupが並行数分で成立することをtestする。

**影響する文書**: `02`(§12)、`05`(download key)、`08`(§2)、`13`。

---

## 3. 共通の再導入手順

1. 本章の該当節を読み、`v0.1での代替`で目的が達成できないことを具体例で示す。
2. [13-progress.md](13-progress.md)へtaskを追加し、依存する他の延期機能（例: D-05はD-01に依存）を明記する。
3. 該当節の`再導入gate`を1件ずつ確定する。判断が必要な項目は[14-maintenance.md](14-maintenance.md)§2の選択肢表で、質問を1件ずつ行う。
4. schemaやdata contractに触れる場合は[14-maintenance.md](14-maintenance.md)§6/§7のgateを先に通す。
5. 仕様 → fixture/test → 実装 → 公開文書 → 進捗の順で同じ変更にまとめる。
6. 本章の該当節を、実装済みであることと実装したclient versionへ書き換える。節ごと削除せず、判断の履歴を残す。
7. CI matrixの両OSがgreenになるまでtaskを完了にしない。

## 4. 実装prompt

### 4.1 共通prompt（全機能で使う）

`<FEATURE_ID>`に`D-01`等を入れる。

```text
docs/15-deferred.md の <FEATURE_ID> をv0.1へ再導入できるか調査し、確定した範囲まで実装してください。

最初に次を読んでください。
- docs/README.md 全文（特に §2 release段階 と §5 固定された製品判断）
- docs/15-deferred.md の <FEATURE_ID> の節と §3 共通の再導入手順
- 同節の「影響する文書」に挙がっている番号付き仕様
- docs/13-progress.md のsnapshotと最新の停止記録
- docs/14-maintenance.md §2 共通開始gate

branch、commit、worktree、OS/arch/shell/Go versionを確認し、利用者の既存差分を保持してください。

最初の成果は実装ではありません。次を順に提示してください。

1. その機能がないと達成できないことの具体例。可能なら実際の利用者reportを引用する。v0.1での代替で目的が達成できるなら、実装せずその旨を報告して終了する。
2. 該当節の「再導入gate」の各項目に対する現時点の答えと、未確定の項目。
3. 依存する他の延期機能があるか（例: D-05はD-01に、D-20はD-04のrepairに依存する）。
4. 影響する文書ごとの変更範囲と、新たに必要になるfixture/testの一覧。

未確定の項目は、推奨・他の選択肢・それぞれのメリット/デメリット/影響範囲を表で示し、質問を1件ずつ行ってください。すべての回答が確定するまで仕様もコードも変更しないでください。

確定後は、仕様 → fixture/test → 実装 → 公開文書 → 進捗の順に、同じ変更としてまとめてください。次を守ってください。

- 将来用の予約key、未使用enum値、tool ID分岐、寛容fallbackを追加しない。
- security errorをwarningやforceで回避できるようにしない。
- WindowsとLinuxを同じ変更で実装し、CI matrixの両OSがgreenになるまで完了にしない。
- docs/15-deferred.md の該当節を、実装済みであることと実装したclient versionへ書き換える。節ごと削除しない。

最後に日本語で、完了したtask ID、変更した主要file、実行した検証commandと結果、未実施の検証、残存risk、次のtask IDを報告してください。事実と推測を区別してください。
```

### 4.2 D-01 `self-update` 専用の追加指示

共通promptに続けて渡す。

```text
D-01 は v0.1 で最も複雑な延期機能です。実装前に次を必ず確定してください。

1. 状態の永続先。D-08（operation journal）を戻すのか、self-update専用の state file 1件で足りるのかを先に決める。両方を導入しない。
2. 失敗点の完全な列挙。少なくとも「親process未終了」「old移動途中」「new移動途中」「verify失敗」「commit直後の異常終了」「次回起動時に未完了状態を発見」の各点で、どこへ戻すかを決める。
3. Windowsは実行中binaryを置換できないため、apply子processが親の終了を待つ必要がある。待機の合図（inherited pipe、PID監視、token）をどれにするか、偽の親を騙れないことを含めて決める。
4. client と registry が混在した状態を絶対に公開しないことを、failure injection test で証明する。
5. downgrade拒否、devel build拒否、release/asset identityのExecute直前再確認をtestする。

実装後は、正常更新、download失敗、apply中断、次回起動時の復旧、任意 gdtvm.toml の保持を E2E で検証してください。中断の再現には実際にprocessを強制終了するscenarioを含めてください。
```

### 4.3 D-03 `use --system` 専用の追加指示

共通promptに続けて渡す。

```text
D-03 は利用者体験を壊しやすい延期機能です。実装前に次を必ず確定してください。

1. **`file_identity` に mtime を含めるかを最初に決める。** v0.1の設計案はWindowsで last_write_filetime、Linuxで mtime を digest 入力に含めており、identity変化時は fail closed で再選択を強制する。この設計だと、アンチウイルスのスキャン、バックアップからの復元、`touch` 相当の操作だけで `use --system` が壊れる。mtime除外、変化時の自動再probe、警告のみで継続、のいずれを選ぶかを比較して決める。
2. 候補が同一distributionに属することの判定方法（primary targetのrelative suffix除去によるroot導出、sibling probe、報告prefixの照合）を両OSで決める。
3. Windowsのreparse point / alias、Linuxのsymlink chainとELF archの扱いを決める。shell scriptやWindows aliasを候補にしない。
4. 候補0件・複数件・probe不合格の表示と、別候補へfallbackしないことを利用者にどう伝えるかを決める。
5. 配布元provenanceを検証しない旨の表示が「非公式」と誤読されない文言かを、日本語のsnapshot testで確認する。

実装後は、identity変化、実体消失、permission変化、siblingの片方だけ差し替え、PATH順位変更の各caseをE2Eで検証してください。
```

### 4.4 D-06 checksum非提供artifact 専用の追加指示

共通promptに続けて渡す。

```text
D-06 は security 契約を緩める方向の変更です。実装前に次を必ず確定してください。

1. 対象toolがchecksumを公開していないことを一次資料で示し、代替provider（公式mirror、別build、source配布＋公式digest）が存在しないことを確認する。存在するならそちらを採用し、この機能を導入しない。
2. Plan表示で「提供者によるchecksum未公開・初回取得物の真正性未検証」が、検証済みartifactと視覚的に区別されることを日本語snapshot testで確認する。色だけで区別しない。
3. receiptの検証状態をどう表現するか（v0.1は全artifactがverifiedのためfieldがない）。field追加はdocs/04のreceipt契約変更になる。
4. 同一catalog identityで別digestを得た場合に自動更新せず失敗すること。
5. offline時の制約（初回offline導入を許さない、保存済みdigestと一致する既存cacheだけ再利用可）。

この機能を「利便性のため」に導入しないでください。対象toolを追加しないという選択肢を必ず比較対象に残してください。
```

### 4.5 D-04 追加command 専用の追加指示

共通promptに続けて渡す。

```text
D-04 は複数commandの集合です。1回のtaskで複数commandをまとめて追加しないでください。1 commandずつ、次を確定してから実装してください。

1. そのcommandがないと達成できないことを、実際の利用者report（docs/14-maintenance.md §5）で示す。
2. v0.1の代替（`available --refresh`、`installed`、`.gdtvm.toml`編集、`setup`再実行、`use`再実行、再install）で足りないことを示す。
3. 状態変更を伴うなら Plan / approval category / rollback / `inputs` 検査を決める。
4. `--json` を持たせるかを決める。持たせるなら D-15 と同じ判断が必要になる。
5. docs/03 のcommand一覧、docs/11 §8 のE2E scenario、docs/12 の公開文書、docs/13 のtaskを同時に更新する。

`repair` を追加する場合は D-20（portable root移動）を同時に検討し、修復対象の範囲を明示的に列挙してください。checksum / security 違反を回避する修復を含めないでください。
```
