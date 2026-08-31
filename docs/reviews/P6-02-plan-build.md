# P6-02 決定記録（2/2）: install Planの組立てと検証

対象タスク: `docs/13-progress.md` P6-02の2本目。規範仕様は[04-storage-and-data.md](../04-storage-and-data.md)§16・§16.1・§17.2、[02-architecture.md](../02-architecture.md)§2・§8・§8.1、[06-tool-definition.md](../06-tool-definition.md)§10.1・§11・§12、[10-security.md](../10-security.md)§8、[11-quality-and-ci.md](../11-quality-and-ci.md)§7.2、[03-cli.md](../03-cli.md)§7。

## 1. 着手時の確認事項（1本目の停止記録より）

2件とも**仕様から一意に決まった**ため、利用者判断を求めていない。

### 1.1 `internal/app`へ`internal/store`を追加してよい

[02-architecture.md](../02-architecture.md)§8手順5がExecuteの不変条件を「全書込みがdata root、distribution root、宣言済みintegration対象、project fileの中にあり、任意helper/backend processを起動しないこと」と定め、§2が`internal/app`へ「トランザクション境界」を割り当てる。この不変条件をPlanから導くにはPlan型を読む必要がある。

`internal/store`は`internal/app`をimportしないためcycleにならない。`internal/catalog`が§15のcatalog型で同じ形の先例を持つ。

### 1.2 `writes[]`の粒度は「利用者可視の変更」であり、粒度の問題ではなかった

[04-storage-and-data.md](../04-storage-and-data.md)§16が対象を列挙している。

| 対象 | `writes[]`へ出すか |
|---|---|
| setup/setup-removeのintegration対象（Windows user PATH、shell profile） | 出す |
| project fileの作成・更新 | 出す |
| current linkの`junction\|symlink\|remove` | 出す |
| staging、download cache、state、receipt、index、shim、storage | **出さない**（data root内部） |

file単位かdirectory単位かではなく、**data root内部か外か**が判定である。したがってinstall単体の`writes[]`は空になる。

**この結論は1本目の決定記録の記述を1点修正する。** そこでは「`Scope`は`writes[]`／`probes[]`／download URLを受けて設計されている」と書いたが、`writes[]`だけをrootにすると§16が列挙しないと定めた内部書込み（staging、receipt、index、shim）がすべて拒否される。§8手順5の範囲は`writes[]`∪{data root, distribution root}であり、後者はPlanの外から渡す。[PlanScopeRequest]がその形になっている。

## 2. 判断

### 2.1 definitionのenumをstoreへ移すときstring castを使わない

`internal/definition`と`internal/store`は同じ値集合（storage kind、probe stream、archive formatなど8種）を**別の型**で持つ。`store.StorageKind(def.Kind)`と書けるが、castだとdefinition側へ値が増えたときに、storeが知らない値を持つPlanを黙って作れてしまう。

変換表で受けて未知値をerrorにした。値が増えれば変換が落ち、追随漏れがその場で分かる。件数と値の一致は`TestEnumTablesCoverDefinition`が固定する。

### 2.2 warningの承認要否をPlan作成側に決めさせない

§16.1「`requires_explicit_approval=true`のcode集合がApprovalの単位」。真偽はcodeごとに表が一意に決める。ところが表（`planWarningApproval`）は`internal/store`の非公開値であり、外のpackageは正しい値を得られない。

複製すると、同じcodeが場面によって承認要否を変えられる。`store.NewPlanWarning`を公開し、**表から引く経路をこれ1つにした**。

install operationで立てうるのは4件だけである（`W_THIRD_PARTY`、`W_RESTRICTIVE_LICENSE`、`W_PRERELEASE`、`W_EOL`）。残る4件はuninstall・setupの条件であり、installで立てない。

### 2.3 probeは完全展開し、Planにtemplateを残さない

§16「definition probeを**完全展開**した値」。Planに残ったtemplateをExecuteが評価すると、利用者が承認した文字列と実際に起動するargvが食い違いうる。`TestBuildInstallPlanExpandsProbe`はencode結果に`{{`が現れないことまで見る。

argsはentry 1件がargv 1要素に対応する（§16「definitionの1個のargs entryを複数argvへ分割せず、pathをliteralやwarning parameterへ埋め込まない」）。templateを含むentryは`kind=path`、含まないentryは`kind=literal`とし、部分置換を許さない。

probeのcwdはpayload rootとした。§11がcwdを宣言させないため、payload外を指す余地を作らない一意な選び方がこれだけである。

### 2.4 registry valueをfilesystem rootにしない

§17.2「Windows user PATHのregistry valueはfilesystem pathではないが変更対象の識別が必要なため…`path`はexact locator `HKCU\Environment\Path`とする。これはPlan `PathValue.path`をabsolute filesystem pathとしない唯一の例外である」。

これをfilesystem書込みのrootとして扱うと、その文字列で始まるpathを許すことになる。registry値の変更はfilesystem writeを通らないため、Scopeの対象から外した。

### 2.5 stale判定の現在値は呼出し側が集める

§16「Executeは`inputs`の各値を実体から再取得して一致を確認する」。1本目で決めたとおり、計算の主体はfileを所有するpackageである。`CheckPlanFreshness`が自分で集めると、Plan作成時と同じ経路を2回通るだけになり、判定が「同じ関数を2度呼んだ結果の比較」に退化する。

変化したfield名は診断へ出すが、**値そのものは持たない**。digest/revisionは秘密ではないが、片方だけを見せても利用者の判断材料にならず、log/reportへ流れる情報を増やすだけである。利用者が取る行動は「作り直して再実行」で一定のため、公開messageにもfield名を載せない（[02-architecture.md](../02-architecture.md)§14）。

## 3. 検査が固定したこと

Plan組立てで50 subtest、Scope導出とstale判定で32 subtestを追加した。

| 検査 | 対象 |
|---|---|
| `TestBuildInstallPlanProducesEncodablePlan` | `EncodePlan`→`ParsePlan`を通ること。§16の全契約はP2-04のcodecが持つため、そこへ通して組立ての誤りを当てる |
| `TestBuildInstallPlanExpandsProbe` | probeの完全展開とtemplateが残らないこと |
| `TestBuildInstallPlanSeparatesPathArgs` | path argがliteralへ埋まらないこと、command args→probe argsの順序 |
| `TestBuildInstallPlanRejectsUndeclaredCommand` | 宣言外runtime commandの拒否 |
| `TestBuildInstallPlanRejectsProbeTempOutsideProbe` | `{{probe_temp}}`がprobe文脈だけで解決すること |
| `TestBuildInstallPlanOmitsInternalWrites` | data root内部を`writes[]`へ出さないこと |
| `TestBuildInstallPlanBuildsWarnings` / `WarnsOnLicenseNotice` | §16.1の4件と`warning_count`の一致 |
| `TestBuildInstallPlanRejectsInvalidRequest` | 前提違反20件（digest無し、role違い、installable=false ほか） |
| `TestBuildInstallPlanIsDeterministic` / `CarriesInputs` | 純関数であること、`inputs`をそのまま載せること |
| `TestEnumTablesCoverDefinition` / `PreserveValue` / `TestConvertRejectsUnknownValue` | 変換表8種の網羅・値保存・未知値拒否 |
| `TestPlanWarningApprovalComesFromTable` | 承認要否が§16.1の表から来ること（8件・うち7件が承認対象） |
| `TestScopeFromPlanAllowsContainmentRange` | §8手順5の範囲。`writes[]`へ出さない内部書込みが通ること |
| `TestScopeFromPlanSkipsRegistryValue` | registry locatorをfilesystem rootにしないこと |
| `TestScopeFromPlanAllowsOnlyListedProcesses` | 任意helper processの拒否、argsの完全一致 |
| `TestScopeFromPlanConvertsPathArgs` | `kind=path`のargv復元 |
| `TestPlanFreshnessComparesEveryInput` | §16の`inputs` 9 fieldすべてを1件ずつ |

### 3.1 変異testが検査の穴を1件見つけた

5件の変異を入れた。**2件目が生き残った。**

`TestBuildInstallPlanRejectsUndeclaredCommand`は「errorになった」だけを見ており、未宣言時に`lookupCommand`がzero値の`definition.Command`を返し、その**空targetを`RenderPath`が拒否する**経路でも通っていた。名前の照合そのものを固定できていない。errorが未宣言のcommand名を挙げることまで見るよう直した。

4件目（`writes[]`のtarget未設定検査）も生き残ったが、これは`NewScope`のzero root検査が同じ入力を落とすためで、**封じ込めの保証は失われていない**。この検査の価値は「どのentryが悪いか」の診断にあるため、それを固定する`TestScopeFromPlanNamesInvalidWrite`を足した。

| 変異 | 初回 | 検査追加後 |
|---|---|---|
| registry locatorをfilesystem rootにする | 落ちた | — |
| 宣言外runtime commandを許す | **通った** | 落ちる |
| `kind=path`のargvを空にする | 落ちた | — |
| `writes[]`のtarget未設定検査を外す | 通った（`NewScope`が担保） | 診断を固定 |
| stale比較表から1件落とす | 落ちた（3検査） | — |

## 4. 検証

Linux containerで実行した（Go 1.26.6）。両OSの判定はCI matrixで行う。

| command | 結果 |
|---|---|
| `gofmt -l .` / `go build ./...` / `GOOS=windows go build ./...` | 出力なし・成功 |
| `go vet ./...` / `GOOS=windows go vet ./...` | 成功 |
| `go test ./... -race -shuffle=on -covermode=atomic` | 全package成功。`internal/app` 91.4%・`internal/install` 90.1% |
| `check_policy.py` / `check_imports.py` / `check_docs.py` / `check_licenses.py` / `check_messages.py` | すべて成功 |
| `git diff --check` | 出力なし |

message IDを6件追加した（`plan.eol`、`plan.prerelease`、`plan.probe_reason`、`plan.restrictive_license`、`plan.stale`、`plan.third_party`）。`MessageCount`を87→93へ更新し、message catalogへ`plan`分類を新設した。分類集合を固定する`TestRepositoryMessageCatalog`が新分類を検出したため、同じ変更で期待値を更新した（分類追加を無自覚に通さないための検査であり、これは意図した追加である）。

## 5. 未実施・制約

- **`BuildInstallPlan`／`ScopeFromPlan`／`CheckPlanFreshness`をまだ誰も呼んでいない。** 呼出し側はP6-03のResolve→Plan→Approve→Execute→Commitである。stale判定は比較だけを実装しており、**実体からの再取得はP6-03**が行う。
- **`install --use`のcurrent linkとproject fileを`writes[]`へ載せていない。** §16はcurrent linkの`junction|symlink|remove`とproject fileを`writes[]`の対象とするが、選択はselection層（P6-04）の責務であり、installのPlan builderが単独で決められない。P6-03で`install --use`を繋ぐときに載せる。
- **setup/use/uninstallのPlan builderは未実装。** 本PRはinstallだけである。`W_DESTRUCTIVE`／`W_SHELL_MODIFICATION`／`W_MODE_CHANGE`／`W_RESTART_REQUIRED`の4 warningと`SetupPlan`は該当taskで扱う。
- **1本目から継続する仕様の食い違いが1件**: `internal/store`のtemplate grammarが`internal/definition`と一致しない（`{{storage.9x}}`をdefinitionは拒否・storeは通す、`{{a{{b}}`の切り出しも異なる）。fail closedは保たれており正当なdefinitionからは生じない値である。本PRでも`internal/store`へは手を入れていない——`internal/definition`へ依存させるかは[02-architecture.md](../02-architecture.md)§2の責務表を要する判断であり、P6-03で扱う。
- **P6-01から継続する未実装**: Registry portと`LinkManager` portの記録wrapper（P7）、[11-quality-and-ci.md](../11-quality-and-ci.md)§7.2のE2E照合（P6以降）、`port.FileSystem`／`port.Environment`のproduction実装（P8-01）、Windowsの起動とjob割当ての隙間。
- **P5-03から継続する未決が1件**（`./`始まりのtar entryを[10-security.md](../10-security.md)§5に従って拒否しており、標準4 toolの実archiveを繋ぐP6-03で該当があれば仕様側で扱いを決める）。**P6-01で埋めた仕様の空白が1件継続**（exact指定で`installable=false`のときの`E_PLATFORM_UNSUPPORTED`）。**仕様側の未決が2件継続**（[07-registry-and-tools.md](../07-registry-and-tools.md)§5第2項のlicense file size上限が§21の表に無い、§2の「license file名はASCII kebab grammar」が§2自身の例と食い違う）。`python.toml`の`lifecycle = "unknown"`はP3-04から継続。source error専用のerror code（`E_DEFINITION_INVALID`）、version数値要素の64 bit上限、`logs/` file名規約と§11「専用lock」の解釈は未決である。
