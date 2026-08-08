# 保守・拡張workflowとAI prompt

## 1. 目的

本章はv0.1の運用中に発生するtool追加、upstream変更、不具合の調査修正、definition/config/data schema、OS/architecture/libcの追加を安全に行う標準手順を定める。手順は人手、Codex、Claude Code、その他agentで共通に使う。

v0.1で意図的に延期した機能を実装する場合は本章ではなく[15-deferred.md](15-deferred.md)を入口とする。同章は延期理由、再導入gate、機能ごとの実装promptを持つ。

追加候補は事前対応toolではなく未仕様である。調査前にplaceholder definition、tool ID分岐、hook/script、寛容fallbackを追加しない。

## 2. 共通開始gate

すべての保守作業は次から始める。

1. 利用者の現在指示、[README.md](README.md)全文、対象番号付き仕様、[13-progress.md](13-progress.md)のsnapshot/停止記録を読む。
2. branch、commit、worktree、OS/arch/shell/Go versionを記録し、既存差分の所有者を推測して上書きしない。
3. 対象task ID、依存gate、受入条件を進捗へ追加/確認し、同時に1件だけ`[-]`にする。
4. upstream一次資料、公式repository/release/API/licenseだけを根拠に調査する。technical factは取得URLと確認日を記録する。
5. 仕様の未決・矛盾を見つけたら実装を止め、影響と選択肢を提示して仕様を先に確定する。

選択が必要な指摘は次の形にする。

| 選択肢 | 内容 | メリット | デメリット/リスク | 推奨 |
|---|---|---|---|---|
| A | ... | ... | ... | 推奨理由 |
| B | ... | ... | ... | 代替になる条件 |

質問は1件ずつ行い、回答をdecision logへ記録する。安全・正確性を下げる案を「初心者向け」として推奨しない。

## 3. tool追加workflow

### T01 候補定義

`docs/research/tools/<tool-id>.md`を作る前に、正規ID候補、表示名、対象version scheme、必須platform、利用者が必要とする公開command、非対象機能を明文化する。標準4 toolとID/alias/commandが衝突しないことを確認する。

### T02 upstream調査

両platformで次を一次資料と実物から調査する。

- 公式version一覧/API、stable/prerelease/EOL表現、rate limit、layout変更履歴
- 公式artifactのOS/arch/libc、archive形式、portable/nonroot、複数version共存、再配置可否
- checksum/digestの公開有無、公開時期、asset差替え可能性
- license、同梱dependency license、再配布条件
- archive top-level、link、case collision、permission、最大size/entry、必須file
- required/optional command、version probe、command同士の同一distribution確認
- 上流標準config command/file、cache、global package/tool、project-local環境
- version間で共有できるstorageとABI/API競合。共有を推測せず実験する
- 必要なsystem library/command、admin/root要否
- install/uninstall/update時に実行される外部programとnetwork

artifactを実際に取得する場合は隔離tmpへ保存し、digest、取得時刻、URL、sizeを記録する。repositoryへbinaryをcommitしない。archiveは展開前にlistし、安全な検査環境だけで開く。

### T03 provider選択

優先順位は公式portable artifact、公式source build、third-party portable build。ただし公式source buildがsystem compiler/library、長時間build、platform差を利用者へ要求するなら、初心者の再現性と非root要件を含めて比較する。

公式以外を採用する場合はprovider、repository、license、更新頻度、checksum、採用理由、公式へ戻す方法をPlan/仕様へ必須化する。公式artifactが要件を満たすのにthird-partyを利便性だけで選ばない。

**checksumを公開しないproviderのartifactはv0.1のschemaで表現できない**。その場合はT04で必ずgapとして扱い、[15-deferred.md](15-deferred.md) D-06のgateを先に完了する。

### T04 definitionで表現可能か

[06-tool-definition.md](06-tool-definition.md) schema 1のfieldだけで、version→artifact→checksum→install parameter→storage→command→probeが完全に書けるかtrace tableを作る。

| 要求 | upstream根拠 | schema field | fixture | 結果 |
|---|---|---|---|---|
| version discovery | URL/field | `version_source...` | file | pass/gap |

gapが0件ならdefinition-only。1件でもあればTOMLへ未知key、tool ID `if`、hook、shell、precomputed hidden dataを足さず、§6のschema拡張gateへ進む。

### T05 仕様化

最低限、次を同じ変更で更新する。

- `01`: 対応範囲/非対象/初心者flowに影響する場合
- `06`: schema変更時
- `07`: registry exact tree/tool件数/license/provider/storage/command表
- `04`, `08`～`10`: transaction/platform/data/security差がある場合
- `11`: contract/live/E2E matrix
- `12`: 公開対応表/guide更新条件
- `13`: task、依存、証跡
- 本章: 新しい保守判断が再利用可能な場合

仕様だけのPRでもlink、TOML/JSON例、用語、件数、enumを検証する。

### T06 fixture/definition

positiveはversion source、checksum source、最小安全archive、expected catalog/Plan/receipt/probe output。negativeは0/2 asset、checksum mismatch、layout変更、version mismatch、path/link/bomb、missing command/storage conflictを含む。

`registry/tools/<id>.toml`、license/message、`registry.toml` digestを更新する。live最新versionをfixtureへ自動copyせず、review可能な最小値へ固定する。

### T07 実装

definition-only追加ではproduction Go codeを変更しないことを原則とする。schema拡張ならtool非依存parser/domain/planner/executor/portを実装し、fake testを先に通す。CLIへtool判断を置かない。

Windows/Linuxの両方を同じ変更で実装する。片方だけを先に完成させず、CI matrixで両OSがgreenになるまでtaskを完了にしない。

### T08 完了

format/unit/contract/security/E2E/live smokeを実行し、command/result/report pathを[13-progress.md](13-progress.md)へ記録する。片方のOSが未検証ならtaskを`[x]`にしない。公開文書は実装・CI合格後だけ対応済みへ変える。

## 4. 不具合・upstream変更workflow

### B01 再現と分類

期待値を仕様章・field・受入条件で引用し、実際値、最小再現command、OS/arch/client/tool完全version、provider build、config/receipt digest、secret除去logを記録する。

**利用者からの報告がある場合は、まず`gdtvm doctor --report`の出力を正本とする**。手元で再現する前に、reportに記録された環境・導入状態・診断結果と、報告された症状の整合を確認する。

分類は`spec ambiguity|definition|upstream metadata|artifact|schema/engine|platform|state|CLI presentation|documentation`。

specから期待値を一意に決められない場合はcodeを修正せず、選択肢を1件ずつ確定する。

### B02 最小再現fixture

live network、現在時刻、実user homeへ依存しないfailure fixture/testを先に追加する。security issueは実credential/悪性binaryを保存せず、最小bytes/pathで境界を再現する。既存testをskip/弱体化しない。

### B03 原因と影響

最初に壊れた契約境界を特定し、症状が出た下流だけを修正しない。全tool/platform/state schema/releaseへの影響を`rg`とcontract matrixで調べる。upstream layout/digest変化はasset差替え、正規新release、取得途中、metadata bugを区別する。

checksum/digest不一致を新値へ自動更新して解消しない。固定Python build等の同一version差替えは、新しいCPython完全versionとしての追加か、entryを外した新clientのreleaseかを先に判断する。

### B04 修正

observable contractが変わるなら仕様→fixture→実装→公開文書の順で同じ変更にする。definitionだけで直る場合はGo分岐を追加しない。state/data変更はrollback/failure injectionを付ける。

security regressionは通常success、error、cancel、rollback、concurrent raceのnegative testを追加する。修正後に元の再現が失敗→成功へ変わったことと周辺回帰を記録する。

### B05 完了

CI matrixの両OSがgreenであることを確認する。原因、修正、検証、残存risk、再発検知方法をreport/[13-progress.md](13-progress.md)へ残す。利用者報告が起点の場合は、報告者が同じ手順で再確認できるようreproduction commandを回答へ含める。

## 5. ドッグフーディング不具合報告

v0.1は利用者が実際に使って問題を見つけ、修正する前提で作る。報告の質がそのまま修正速度になるため、次のtemplateを`README`/`USER_GUIDE`から案内する。

### 5.1 利用者向けテンプレート

````markdown
## 症状
（何をしたら何が起きたか。1〜3文）

## 期待した動作
（どうなると思ったか）

## 再現手順
1. `gdtvm ...`
2. `gdtvm ...`
3. ...

## 実行したcommandと出力
`-v` を付けて再実行した出力を貼る。長い場合は末尾50行。

```text
（ここに貼る）
```

## 診断report
`gdtvm doctor --report gdtvm-report.md` を実行し、**中身を目視確認してから**貼る。

```markdown
（ここに貼る）
```

## 影響
- [ ] 作業が完全に止まる
- [ ] 回避策があるが不便
- [ ] 表示や案内の問題

## 回避策として試したこと
（あれば）
````

reportにはsecretと個人pathが含まれない設計だが（[10-security.md](10-security.md)§9）、貼る前の目視確認を常に案内する。

### 5.2 受け取り側の初動

1. reportのclient version、mode、platform、registry digestを確認し、現在のreleaseと一致するか見る。
2. `installed`/`selection`/`diagnostics`節から、状態破損か操作の誤解かを分ける。
3. 症状がdiagnosticsに現れていない場合、診断項目自体の不足として`P9`相当のtaskを起こす。
4. 分類が決まったらB01へ進む。

**診断項目に現れない不具合が出たときは、修正と同じ変更で診断項目を追加する。** ドッグフーディングの目的は、次に同じ問題が起きたとき`doctor`だけで分かる状態へ近づけることである。

## 6. definition schema拡張gate

新field/kindは実在する追加要求からだけ導入する。将来用予約を作らない。

1. schema 1で表現不能な最小counterexampleとupstream根拠を示す。
2. definition-only、既存field一般化、新field、対応しないの少なくとも3案をメリット/デメリット比較する。
3. field path、型、必須条件、default、enum、上限、platform差、template context、unknown/duplicate時errorを決める。
4. Resolve/Plan/Execute/receipt/catalog/securityへのdata flowと、Planでの表示を決める。
5. old definition/receipt/client互換を決める。意味が変わる場合はschema revisionを上げ、黙示fallbackしない。
6. positive/全negative/上限/fuzz/failure injection fixtureを作る。
7. `04`, `06`, `07`, `10`, affected `08`, registry JSON schema、validator、test、`13`を同時更新する。
8. 追加tool抜きのsynthetic definitionでも機能し、tool ID分岐がないことをtestする。

外部process、署名、credential、script、backend、mutable payloadを追加する場合は専用threat model、取得/identity、Plan、timeout/cancel、rollback、secret maskを必須とし、通常field追加より強いreview gateを設ける。

## 7. config・data contract拡張

### 7.1 config

新keyは利用者が継続的に変更する必要があるものだけ。internal safety limitや一時debugを公開しない。

- 問題、利用者persona、利用例、未設定default、範囲、排他、platform差を決める。
- CLI一時option/既存key/組込みdefaultで代替できないことを示す。
- old fileの挙動、unknown key、comment保持、自動編集有無を決める。
- `05`, sample、parser、positive/negative/boundary fixture、公開文書、`13`を更新する。

### 7.2 state/receipt/catalog/JSON

field追加はproducer/consumer一覧、正本/cache区分、canonical順序、上限、old/new互換を決める。Go structの追加をそのままwire schema変更とみなさない。future fieldをunknownとして無視しない。

v0.1にschema migration機構はない。schema revisionを上げる必要が生じた場合は、[15-deferred.md](15-deferred.md) D-09のgateでmigration契約を先に仕様化する。

## 8. platform追加

Windows arm64、Linux arm64/musl等はhost判定、upstream artifact全4 tool、archive、shim/link、storage/native package、CI runnerを調査する。platform IDとtupleを`06`/`04`で追加し、4 toolすべてが対応しない場合はtoolごとのunsupported理由を明示する。既存amd64 artifactをemulationで黙って選ばない。

CI runnerを用意できないplatformを「対応」と表示しない。手動確認だけで支えるplatformはexperimentalとして区別する。

## 9. prompt共通変数

以下のpromptで`<...>`を具体値へ置換する。

- `<TASK_ID>`: `13-progress.md`へ追加したtask ID
- `<TOOL_ID>`/`<TOOL_NAME>`: 追加候補
- `<ISSUE>`: 症状と最小再現
- `<REPORT>`: `gdtvm doctor --report`の出力または貼り付け先
- `<TARGET>`: Windows/Linux、arch/libc、version
- `<EVIDENCE_PATH>`: report保存先

agentには質問を1件ずつ行わせ、仕様決定前に実装させない。最終報告は変更file、検証command/result、未実施、blocker、次taskを含める。

## 10. 汎用agent prompt

### 10.1 tool調査・追加

```text
このrepositoryへ <TOOL_NAME>（ID <TOOL_ID>）を追加できるか調査し、仕様・実装・検証まで行ってください。

最初にAGENTS.md/CLAUDE.md、docs/README.md全文、docs/04,06,07,08,09,10,11,13,14を読み、branch/commit/worktree/環境と既存差分を確認してください。進捗台帳の依存gateを確認し、<TASK_ID>だけを進行中にしてください。

まずdocs/14のT01～T04に従い、公式一次資料と実artifactからWindows amd64/Linux amd64-glibcのversion metadata、portable/nonroot artifact、checksum公開の有無、license、archive layout、required commands、標準設定、storage共有可否、system prerequisiteを調査してください。公式artifactを推奨候補にし、要件を満たさない場合だけthird-party/source buildと比較してください。checksumを公開しないproviderはschema 1で表現できないため、gapとして扱ってください。

未決事項は、推奨・他の選択肢・各メリット/デメリット・影響範囲を表で示し、質問を1件ずつ行ってください。回答が確定するまで仕様/コードを変更しないでください。

schema 1で完全に表現できるtrace tableを作り、gapがあればtool ID分岐/hook/fallbackを追加せずschema拡張gate（docs/14 §6）を先に行ってください。決定後は仕様、registry definition/license/message、fixture、test、進捗を同じ変更で更新してください。WindowsとLinuxを同じ変更で実装し、CI matrixの両OSがgreenになるまで完了にしないでください。
```

### 10.2 不具合triage（利用者報告の受け取り）

```text
利用者から次の不具合報告を受け取りました。修正の前に原因分類まで進めてください。

<REPORT>

最初にdocs/README.md、docs/14 §5、該当する番号付き仕様を読んでください。次の順で進めてください。

1. reportのclient version / mode / platform / registry digestが現在のreleaseと一致するか確認する。
2. installed / selection / diagnostics 節から、状態破損・操作の誤解・仕様どおりの挙動のどれかを切り分ける。
3. 症状がdiagnosticsに現れていない場合、診断項目自体の不足として別taskを起こす。
4. 期待値を仕様の章・節・fieldから引用する。引用できない場合は仕様の未決事項として報告し、修正しない。
5. 最小再現手順を、live network・現在時刻・実user homeに依存しない形で示す。

この時点では修正を実装しないでください。分類、壊れた契約境界、影響範囲（全tool/platform/state/release）、必要な追加情報を日本語で報告し、次に取るべきtaskを提案してください。追加情報が必要な場合は、利用者へ聞く質問を1件ずつ挙げてください。
```

### 10.3 不具合修正

```text
<ISSUE> を調査し、仕様に適合する修正と再発防止testを作成してください。対象は <TARGET>、証跡先は <EVIDENCE_PATH> です。

最初にrepository指示、docs/README.md、該当仕様、docs/13とdocs/14のB01～B05を全文確認し、既存差分を保持してください。仕様から期待値を一意に引用し、最小再現fixtureを作ってから原因を追ってください。仕様が未決/矛盾なら実装を止め、推奨と代替案のメリット/デメリットを示して質問を1件ずつ行ってください。

definitionで直せる問題にtool固有Go分岐を追加しないでください。checksum/identity mismatch、archive/path/security errorをwarningやforceで回避しないでください。observable contract変更は仕様、schema/fixture/test、公開文書、進捗を同期してください。

修正と同じ変更で、この不具合が次回`gdtvm doctor`だけで分かるよう診断項目を追加できないか検討してください。failure injection、rollback/cancel/concurrency、CI matrixの両OSを検証し、command/result/report path、未実施、次taskを記録してください。
```

## 11. Codex prompt

### 11.1 tool調査・追加

```text
Codexとして、このrepositoryの作業規則に従い <TOOL_NAME>（正規ID <TOOL_ID>）の追加を <TASK_ID> として進めてください。

作業開始前にroot AGENTS.md、docs/README.md全文、docs/04,06,07,08,09,10,11,13,14を自分で読み、git status/branch/commitとOS/arch/shell/Go versionを確認してください。利用者の差分を保持し、apply_patchで編集してください。

最初の成果は実装ではなくdocs/14 T01～T04のresearch dossierとschema traceです。一次資料が変化し得るため公式sourceをbrowseして確認日/URLを残し、artifactは隔離tmpでdigest/layoutを検査してください。未決事項は推奨・選択肢・メリット・デメリットを明確にし、私への質問は必ず1回1件にしてください。全判断が確定するまで変更を実装しないでください。

確定後はT05～T08を実行してください。schema gapをif tool/hook/scriptで回避せず、必要ならdocs/14 §6のschema gateを先に完了してください。WindowsとLinuxを同じ変更で実装し、CI matrixの両OSがgreenになるまで完了にしないでください。仕様、registry、fixture、test、docs/12/13を同期し、検証証跡なしに[x]へしないでください。最後は日本語で完了task、主要file、検証command/result、未実施/blocker、次taskを報告してください。
```

### 11.2 不具合調査・修正

```text
Codexとして <ISSUE> を <TARGET> で再現・修正してください。証跡は <EVIDENCE_PATH> に残します。

AGENTS.md、docs/README.md、該当仕様、docs/13、docs/14 B01～B05を先に読み、既存差分と依存gateを確認して <TASK_ID> だけを[-]にしてください。利用者報告がある場合はdocs/14 §5.2の初動から始めてください。まずread-only診断と最小failure fixtureを作り、仕様の期待値、実際値、最初に破られた契約境界、全tool/platformへの影響を示してください。診断依頼だけなら修正せず原因を報告し、修正依頼ならtestを弱めず最小変更を実装してください。

仕様が不足/矛盾する場合はコードで補わず、推奨/代替/メリット/デメリットを示す質問を1件ずつ行ってください。definition問題へtool固有Go分岐を追加せず、security failureをforce/fallbackで通さないでください。仕様・fixture・test・進捗を同期し、CI matrixの両OS、rollback/cancel/concurrencyを検証してください。同じ不具合が次回`doctor`で分かるよう診断項目を追加できないか必ず検討してください。最終報告は事実と未確認を分けて日本語で行ってください。
```

## 12. Claude Code prompt

### 12.1 tool調査・追加

```text
Claude Codeとして、repository規約に従い <TOOL_NAME>（ID <TOOL_ID>）追加の <TASK_ID> を実施してください。

最初にCLAUDE.mdとAGENTS.md、docs/README.md全文、docs/04,06,07,08,09,10,11,13,14を読み、branch/commit/worktree/OS/arch/shell/Go version、進捗gate、利用者の既存差分を確認してください。

docs/14 T01～T04に従い、公式一次資料と実artifactを調べたresearch dossier、provider比較、schema traceを先に提示してください。公式portable artifactを第一候補とし、満たせない要件を証拠で示した場合だけthird-party/source buildを比較してください。checksumを公開しないproviderはschema 1で表現できないためgapとして扱ってください。未決事項は推奨と代替案、それぞれのメリット/デメリット/影響を示し、質問は1件ずつにしてください。回答が揃う前に仕様やコードを編集しないでください。

判断確定後、T05～T08を順に実行してください。schemaで表せない挙動をtool名条件、hook、shell、黙示fallbackで実装しないでください。WindowsとLinuxを同じ変更で実装してください。仕様、registry、license/message、fixture、tests、docs/12/13を同期し、実行commandとreport pathを証跡化してください。CI matrixの両OSがgreenになるまでtaskを完了にせず、日本語でblockerと再現commandを報告してください。
```

### 12.2 不具合調査・修正

```text
Claude Codeとして <ISSUE> を <TARGET> で調査し、仕様準拠の修正と再発防止を行ってください。taskは <TASK_ID>、証跡先は <EVIDENCE_PATH> です。

CLAUDE.md/AGENTS.md、docs/README.md、該当仕様、docs/13、docs/14 B01～B05を先に読み、worktreeの利用者変更を保持してください。利用者報告がある場合はdocs/14 §5.2の初動から始めてください。仕様から期待値を引用し、変更前に最小failure fixtureで再現してください。症状ではなく最初に破られた契約境界を特定し、全tool/platform/schema/state/releaseへの影響を調べてください。

期待値を一意に決められなければ実装を止め、推奨・他案・メリット・デメリットを示して質問を1件ずつ行ってください。checksum/digest差を新値へ黙って更新せず、definitionで直せる問題にtool固有Goコードを追加しないでください。

修正と同じ変更で仕様、fixture/test、進捗/公開文書を同期し、この不具合が次回`gdtvm doctor`で分かるよう診断項目を追加できないか検討してください。failure injection、rollback/cancel/concurrency、CI matrixの両OSを検証してください。最後に原因、変更、command/result、未実施、残存risk、次taskを日本語で報告してください。
```

## 13. schema/config拡張prompt

汎用・Codex・Claude Codeのいずれにも、対応するagent名を先頭へ付けて次を使える。

```text
<REQUIREMENT> を満たすため、definition schemaまたはglobal/data schemaの拡張要否を調査してください。

docs/README.md、docs/04,05,06,07,10,13,14,15の該当節を読み、既存fieldだけで表現する案、既存field一般化、新field/schema revision、非対応維持の最低3案を、メリット/デメリット/互換性/security/実装cost/v0.1のscope方針との整合で比較してください。要求がdocs/15で既に延期済みの機能に該当する場合は、まず同章の再導入gateを確認してください。

質問は1件ずつ行い、判断確定前にfieldや実装を追加しないでください。採用後はfield path、型、必須/default、enum/上限、unknown/duplicate、platform差、Plan/receipt/catalog/JSONを仕様化し、positive/negative/boundary/fuzz/failure fixture、正規例、validator、全consumer、進捗を同期してください。将来用予約key、tool ID分岐、寛容fallbackを追加しないでください。
```

## 14. promptの保守

CLI、schema、task gate、対象platform、標準tool集合が変わったら本章の手順とpromptも同じ変更で更新する。promptは仕様の代替ではなく、必ず最新仕様を読ませる入口とする。agent固有機能名・model名・料金・一時的UIへ依存させず、製品仕様が変わってもrepository内作業契約が維持される文面にする。
