# 品質・CI・リリース仕様

## 1. 開発基準

- 実装言語はGo。minimum toolchainはGo 1.26系、CI/releaseは採用minorの最新security patchへ固定する。
- Windows/Linux clientは`CGO_ENABLED=0`。production pathでpanicを通常error処理に使わない。
- `gofmt`, `go vet`, unit/integration、race（対応host）、vulnerability/license検査をCIで実行する。
- dependencyは`go.mod`/`go.sum`へ固定し、tool/OSの暗黙既定値を仕様代わりにしない。
- package global mutable stateを置かず、external effectはport/fakeで決定的にtestする。

各packageに責務と依存方向のpackage comment、全export宣言にGo conventionのdocumentation commentを書く。domain invariant、security境界、transaction/rollback、lock順、platform固有処理、非自明なalgorithmは「何をするか」に加え「なぜ必要か」を説明する。自明な逐語comment、comment-out旧code、追跡先と完了条件のない`TODO|FIXME`を残さない。

tool固有挙動はdefinition/fixture/仕様へ置き、Go commentで補完しない。挙動変更時はcomment、test、仕様を同じ変更で更新する。

### 1.1 Go moduleとtoolchain

| 項目 | 値 |
|---|---|
| module path | `github.com/kznagamori/go_dev_tool_version_manager` |
| `go.mod`の`go` | `1.26.0`（minimum toolchain） |
| `go.mod`の`toolchain` | 採用minorの最新security patch。現在は`go1.26.5` |

**Go versionの正本は`go.mod`だけ**とし、workflowへ数値を書かない。CIは`actions/setup-go`の`go-version-file: go.mod`で読み、`lint` jobが`go.mod`の`toolchain`行と実行中のGo versionの一致を検査する。二箇所に数値を持つと片方だけ更新された状態が静かに成立するためである。

security patchが出た場合は`toolchain`行だけを更新する。minorを上げる場合は`go`行と本節を同じ変更で更新する。

### 1.2 固定command

| job | command | 対象が無いときの扱い |
|---|---|---|
| `lint` | `gofmt -l .` | 常時実行。0 fileでも成功する |
| `lint` | `go vet ./...` | package 0件では`go vet`自体がexit 1になるため、`go list ./...`が空なら未実行を報告して成功する |
| `lint` | `go tool govulncheck ./...` | 同上 |
| `lint` | `scripts/ci/check_licenses.py` | 常時実行。`go.mod`が無ければ未実行を報告して成功する |
| `lint` | `scripts/ci/check_docs.py` | 常時実行 |
| `unit` | `go test ./... -race -shuffle=on -covermode=atomic -coverprofile=coverage.out` | 同上。`go list ./...`が空なら未実行を報告して成功する |
| `policy` | `scripts/ci/check_policy.py`, `scripts/ci/check_pr_refs.py` | 常時実行 |

`-race`はC toolchainを要求するため、`go env CC`が実行可能な場合だけ付ける（§5の「対応host」）。OS名で分岐しない。付けない場合は`-covermode=count`とし、その旨を報告する。`windows-latest`にはC toolchainがあり`-race`が付くことをCIで実測済みである。

repository rootへ`.gitattributes`を置き、text fileを`* text=auto eol=lf`でLF固定にする。Windows runnerのcheckoutは`core.autocrlf`でCRLFへ変換するが、`gofmt`の出力はLF固定のため、CRLFで取り出すと`gofmt -l .`が全Go fileを未formatと報告してWindowsだけ落ちる。改行をplatform差として扱わず、repository側で1つに固定する。

coverageは計測して総合値をjob summaryへ出すだけとし、閾値でCIを失敗させない。package実装前に根拠のない数値を固定しないためである。閾値の導入は実測値が揃ってから別taskで判断する。

外部toolは`go.mod`の`tool` directiveで固定し、`go.sum`のchecksum検証を通す。`go run <module>@<version>`のように実行時解決へ委ねない。

### 1.3 依存licenseの許可list

依存moduleに許可するlicenseは次のpermissiveだけとする。

```text
MIT  Apache-2.0  BSD-2-Clause  BSD-3-Clause  ISC
```

`scripts/ci/check_licenses.py`が`go mod download -json all`の全moduleを走査し、license file不在、判定不能、許可list外をいずれも失敗として扱う。module graph全体を対象にするのは、build対象packageだけへ絞ると後からimportが増えたとき検査範囲が静かに広がるためである。

copyleftを含むmoduleを追加する場合は、`go.mod`へ入れる前に本節の許可listと[10-security.md](10-security.md)のlicense表示契約を同じ変更で更新する。許可listを迂回するoptionや例外指定を実装しない。

### 1.4 証跡directoryと命名

監査、レビュー、検証のreportは`docs/reviews/<TASK-ID>-<slug>.md`へ置く。

- `<TASK-ID>`は[13-progress.md](13-progress.md)に実在するIDのexact値（大文字のまま）とする。
- `<slug>`は内容を表す小文字ASCII英数字のkebab-caseとする。
- 台帳の証跡欄から相対linkで参照し、reportからは`../`で番号付き仕様を参照する。
- reportは監査証跡であり規範仕様ではない。実装時は[README.md](README.md)が指定する番号付き仕様を正とする。

CIが生成するcoverage profile、archive、log等はrepositoryへcommitしない。`.gitignore`で除外し、必要ならworkflow artifactとして残す。

### 1.5 証跡のsecret除去

証跡、commit message、PR本文、logへ次を書かない。

- secret、token、credential、API key
- 個人のhome path、user名を含むabsolute path
- 内部限定URL、社内hostname

expected/actual digest、公開URL、公開version文字列は秘密ではないため記録する（[10-security.md](10-security.md)§9.2と同じ扱い）。検証commandは環境変数の値を貼らず、変数名のまま再現できる形で書く。実行環境はOS、architecture、shell、Go/Python versionの粒度で記録し、host名を書かない。

## 2. client version

正規client versionはCalVerの`YYYY.MM.DD.XX`。`v0.1`は初期完成範囲を表すrelease段階名であり、client version、tag、Go module versionではない。

registry metadataの更新もclient releaseを必要とするため、SemVerのminor/patchが頻繁に増える方式にはしない。年月日を使うことで更新回数が多くても番号の大きさを意味づける必要がなく、利用中clientが最新または十分新しいかをversionから判断しやすくする。

- 日付はannotated tagのtagger timestampを`Asia/Tokyo`へ変換したGregorian calendarの日付とする。tagを作る日付と`VERSION`が異なる場合はtagを作らず、release PRで`VERSION`を更新してmain CIまで再実行する。
- `XX`は同じ日付についてremote repositoryに存在するtagの最大通番＋1とし、tagがなければ`00`、最大は`99`とする。失敗したtagも削除・再利用せず、`99`の次は翌日までreleaseを停止する。
- grammarは`^[0-9]{4}[.](0[1-9]|1[0-2])[.](0[1-9]|[12][0-9]|3[01])[.][0-9]{2}$`と実在日付検査。
- 比較は4個の10進整数tuple。SemVerではなく、SemVerへ変換せずprerelease/build suffixを付けない。
- 正本はroot `/VERSION`のASCII 1行＋LF。tagはannotated `v<version>`、GitHub Release titleも同じ。`VERSION`は`develop/work→main`のrelease PRへ含める。
- development buildだけ`client_version="devel"`, `client_release=false`, 40 lowercase hex commit、dirty boolを持てる。release候補にしない。

正式配布経路はGitHub Releasesのarchiveとbootstrap scriptだけとする。CalVer tagはGo moduleがrelease versionとして認識するSemVer tagではないため、`go install <module>@vYYYY.MM.DD.XX`を導入方法として提供しない。registryだけのversion/tag/releaseも作らない。

## 3. build targetとarchive

release targetは次の2件だけ。

| GOOS | GOARCH | CGO | archive |
|---|---|---:|---|
| windows | amd64 | 0 | `gdtvm_<version>_windows_amd64.zip` |
| linux | amd64 | 0 | `gdtvm_<version>_linux_amd64.tar.gz` |

archive root直下の許可entryは次だけ。任意`gdtvm.toml`は配布物へ含めず、組込みdefaultと[05-configuration.md](05-configuration.md)のsampleを使う。

```text
gdtvm[.exe]
registry/
README.md
USER_GUIDE.md
LICENSE
```

`registry/`内部は[07-registry-and-tools.md](07-registry-and-tools.md)のexact tree。余分な親directory、symlink/hardlink/reparse/device/ADS、absolute/`..`、duplicate/case collisionを禁止する。Linux clientは0755、regular dataは0644、directoryは0755。Windows ZIPは通常fileだけ。

**byte単位で再現可能なbuildはv0.1の要件としない**（[15-deferred.md](15-deferred.md) D-02）。archiveの完全性は`checksums.txt`のSHA-256と公開後の再download検査で担保する。

multi-call clientはCLI/native shim resolverを兼ねる。root導出にlinkを使えない場合の小型resolverは同じsource/releaseからbuildしてclient resourceへ内蔵し、独立assetやnetwork helperにしない。

## 4. build metadata

release binaryへ次を埋め込み、`gdtvm version`で返す。

- client version/release bool/commit/dirty=false
- UTC build time
- Go toolchain、GOOS/GOARCH/CGO
- definition/registry/state schema対応version
- official GitHub repository owner/name

binary、VERSION、tag、archive filename、release title、registry `client_min|max`の不一致をCIで拒否する。release binaryがruntimeにVERSION fileやnetworkからversionを読む構成にしない。

## 5. CI matrix — 唯一のgate

v0.1の合格判定はCI matrixだけで行う。実機での手動E2Eをrelease blockerにしない。

| job | runner | 内容 |
|---|---|---|
| `lint` | `ubuntu-latest`, `windows-latest` | `gofmt -l`, `go vet`, vulnerability scan, license scan, 文書link/example検査 |
| `unit` | `ubuntu-latest`, `windows-latest` | unit/contract/security fixture test、`-race`（対応host） |
| `e2e` | `ubuntu-latest`, `windows-latest` | §8のscenarioをfixture基盤でnetworkなしで実行 |
| `policy` | `ubuntu-latest`, `windows-latest` | §7の禁止API/書込み範囲検査 |
| `package` | `ubuntu-latest`, `windows-latest` | 2 archive生成、構造/permission/binary version検査 |
| `bootstrap` | `ubuntu-latest`, `windows-latest` | fixture releaseに対する`install.ps1`/`install.sh`の静的・実行検査 |

- Windows固有codeを変更したPRもLinux jobを、Linux固有codeを変更したPRもWindows jobを必ず実行する。片方のOSだけを回すworkflow分岐を作らない。
- 両OSのjobは同じtest sourceを共有し、platform差はbuild tagまたはruntime分岐で表現する。「Linuxで再実行する」ためだけのtestを別に書かない。
- PR CIの全jobはnetworkへ依存せず決定的に実行する。実upstreamへのlive接続は§13手順7のlive smoke（[07-registry-and-tools.md](07-registry-and-tools.md)§12）だけが行い、network起因のlive smoke失敗はretryできるがchecksum/schema/security失敗はretryしない。

### 5.1 段階的な開発順序

WindowsとLinuxを同時に開発する。実装は機能単位で進め、OSごとの段階gateを設けない。platform差が出る箇所（link、PATH integration、user lookup、path規則、signal）は同じ機能のtaskの中で両OS分を実装し、CI matrixで同時に検証する。

G-TOOLS（[13-progress.md](13-progress.md)）達成後は、`package` jobが生成するdevel build成果物を利用者へ提供し、P11以降と並行してドッグフーディング（§9のチェックリストと実作業での使用）を開始する（[13-progress.md](13-progress.md) DF-01）。devel buildはCI artifactとしてだけ提供し、GitHub Releaseを作らず、release候補にもしない。

### 5.2 branch topologyと命名

通常の変更は次の向きだけで統合する。`main`は公開済みまたは公開準備済みのrelease履歴、`develop/work`は全agentの統合先、`claude/work`と`codex/work`はagent別の再作成可能な作業統合branchである。

```text
claude/feature-<task-id>-<slug> ─> claude/work ─┐
                                                ├─> develop/work ─> main ─> vYYYY.MM.DD.XX
codex/feature-<task-id>-<slug> ───> codex/work ──┘
```

- `develop/work`は`main`から作る。`claude/work`と`codex/work`は`develop/work`から作る。
- Claude Codeの機能変更は`claude/work`から`claude/feature-<task-id>-<slug>`を、Codexの機能変更は`codex/work`から`codex/feature-<task-id>-<slug>`を作って行う。異なるagentのwork branchをbaseにしない。
- `<task-id>`は[13-progress.md](13-progress.md)にある対象task IDをASCII lowercaseへ変換したexact値とする。`<slug>`は1～48文字の`[a-z0-9]+(?:-[a-z0-9]+)*`で、内容を表すagent命名とする。例は`codex/feature-p6-02-install-plan`。
- feature branchは1 taskだけを扱い、squash merge後に指定maintainerが削除する。agentはremote branchを作成・更新するだけで削除せず、不要になったbranch名を報告する。agent work、develop、mainは履歴を継続利用する期間にfeature変更を直接commitしない。
- registry変更も同じbranchを使い、registry専用branchやregistry単体releaseを作らない。

### 5.3 PR、merge、CI gate

| source | target | merge方式 | merge前の必須CI |
|---|---|---|---|
| `claude/feature-*` | `claude/work` | squash merge後に指定maintainerがsource削除 | 両OSの`lint`, `unit`, `policy` |
| `codex/feature-*` | `codex/work` | squash merge後に指定maintainerがsource削除 | 両OSの`lint`, `unit`, `policy` |
| `claude/work` | `develop/work` | merge commit | 両OSの全6 job |
| `codex/work` | `develop/work` | merge commit | 両OSの全6 job |
| `develop/work` | `main` | merge commit | 両OSの全6 job |

全PRはtargetの最新commitを取り込み、required status checkが最新head commitで成功し、未解決conversationがない場合だけ指定maintainerがmergeする。merge後のsource branch削除も同じ指定maintainerが行う。agentはPR作成と更新までを担当し、merge操作とremote branch削除を実行しない。required approving review数は0件とする。Claude Code/Codexがrepository ownerと同じGitHub identityを使う単独開発でも、自己approval不能によって停止させないためである。

`policy` jobはPR eventのhead/baseを検査し、§5.2の命名grammarと上表にないsource→targetを拒否する。release workflowはPRを経由しないtag eventだけを別entry pointとして扱う。

repository設定ではsquash mergeとmerge commitを許可し、rebase mergeを無効にする。長期branch間をsquash/rebase mergeすると、次回PRで既取込みcommitが再提示されたり到達関係が失われたりするため、agent work→develop→mainはmerge commitだけを使う。feature branchの履歴だけをsquashする。

### 5.4 branch protection

`main`、`develop/work`、`claude/work`、`codex/work`をrulesetまたはbranch protectionのexact対象にし、次を要求する。

- PR経由、§5.3のrequired status check、merge前の最新base取込み、conversation解決。
- 通常のdirect push、force-push、branch削除を禁止する。
- merge commitを許可するためlinear historyを要求しない。
- required approving reviewは0件とし、merge操作とbranch lifecycle操作は指定maintainerだけが行う。

`main`にはforce-push・削除のbypass actorを設けない。指定maintainerだけは§5.5の同期・再作成に限り、`develop/work`とagent workのrulesetをbypassできる。rebase結果の反映は必ず`--force-with-lease`を使い、remote headが事前確認値から変わっていれば失敗させる。この例外で通常変更をdirect pushしない。

tag rulesetは`v*`の作成を指定maintainerだけに制限し、updateと削除にはbypassを設けない。§2のgrammarに合わないtagはrelease workflowで拒否するが、一度作成したtagはrelease失敗時も保持する。

### 5.5 同期、作業中判定、再作成

agent workは、次のいずれか1件でも真なら「作業中」とする。

1. 未mergeのremote `<agent>/feature-*`が存在する。
2. agent workをheadまたはbaseとするopen PRが存在する。
3. agent workに`develop/work`から到達不能なcommitが存在する。

放置feature branchがある場合も作業中になる。不要であることを確認して指定maintainerがPRをcloseしbranchを削除するまで、非作業中として扱わない。

`develop/work`更新後は、次のfeatureを開始する前に対象agent workを同期する。非作業中なら指定maintainerがagent workを削除し、最新`develop/work`から同名branchを再作成する。作業中ならagent workを最新`develop/work`へrebaseし、続いて未mergeの各feature branchを更新後agent workへrebaseする。書換えはremote headを再取得してから`--force-with-lease`で反映し、次のPR merge前に§5.3の該当CIを再実行する。

release成功後は次の順で更新する。

- 両agent workが非作業中なら、`develop/work`、`claude/work`、`codex/work`を削除し、release tagが指す最新`main` commitからdevelop、developから両agent workを再作成する。
- いずれかが作業中なら、developを最新mainへrebaseし、作業中agent workをdevelopへ、その未merge featureをagent workへ順にrebaseする。非作業中のagent workは削除して更新済みdevelopから再作成する。

### 5.6 repository再作成時の初期登録

一回限りの初期登録は次の順とする。

1. GitHubでREADME、MIT LICENSE、Go用`.gitignore`を生成して`main`の初期commitを作る。LICENSEの著作権者・年は現在fileと一致させる。
2. そのmain commitから`develop/work`を作る。
3. 現在のrepository dataをdevelopへ登録する。同名のREADME、LICENSE、`.gitignore`は生成版との差分とlicense内容を確認してから現在版へ更新する。
4. developから`claude/work`と`codex/work`を作り、PR必須、direct/force push・削除禁止、linear history無効、bypass actorの§5.4保護を設定する。
5. P0-01のworkflowを初回実行してcheck名を作り、最初のPRをmergeする前に§5.3のrequired status checkを4 protected branchへ追加する。CI未構成を理由に架空のcheck名や常時成功checkを設定しない。

旧repositoryの`.git`、commit、branch、tag、refを新repositoryへ移行せず、現在の作業treeのfile内容だけを使用する。repository再作成とremote branch操作は指定maintainerが行い、通常の実装taskへ混ぜない。

## 6. unit/contract test

最低限、次をfake clock/HTTP/process/filesystem/link/user lookupで決定的に検査する。

- exact version、latest stable、prerelease/EOL、not-found guidance
- config/project/definition/registry/state/receipt/catalog/Planのstrict positive/negative
- unknown/duplicate/type/enum/size/count/path上限
- version source 3 kind（`json`/`json-index`/`static`）、`json-index`の複数子文書連結と1文書失敗時のfail closed、`item_flatten_pointer`の1段展開、`item_parent_published_at_pointer`の親日時継承、`channel_pointer`のstring/boolean、`lifecycle_map`未定義値のsource error
- channel/lifecycle/evidence、artifact 0/1/2 selection、checksum 2 kind、digest algorithm（`sha256`/`sha512`）とhex長一致、mismatch
- archive traversal、case/予約名/ADS/link/bomb、containment race、`strip_components` 0と1
- typed storage scope、environment merge、command target/fixed args
- Plan `inputs`のstale検査、`SetupPlan`のoperation排他・旧新root・filesystem/link/integration/backup、download/extract/probe/storageの列挙、利用者可視`writes`（integration対象・project file・current link）、書込み封じ込め、任意helper processの拒否、`PlanWarningCode` 8件（うち明示承認7件）と`ResultWarningCode` 5件、approval category、`license_notice`未承認の拒否、cancel、lock順、concurrent install/use
- 機械契約enumの網羅（`04`§17.1）、`Diagnostic.code` 10件とstatus集約、`PathValue`/`path_role` 22値、Plan argvの`PlanArg` literal/path排他、未定義値の拒否
- staging/commit、failure injection、中断後のtmp cleanup
- project precedence、user selection
- shim basename/root/receipt解決、recursion、exit/signal透過
- shell marker、Windows user PATH type/length/rollback/remove ownership
- secret masking、`doctor --report`のmask漏れ
- CLI全commandとApplication Service/JSON/exit code mapping

schema/正規例の全TOML/JSON code fenceをparser testへ取り込み、文書例がschemaとずれないようにする。

## 7. 禁止API・書込み範囲の静的/動的検査

GitHub Actionsの`windows-latest` runnerはAdministratorsグループに属するため、**「標準userでしか動かない」ことをOS権限で証明できない**。代わりに次を検査する。

### 7.1 静的検査（`policy` job）

次のsymbol/文字列がproduction pathに存在しないことをsource走査で検査する。

- 昇格: `runas` verbによる`ShellExecute`、`sudo`/`gsudo`/`pkexec`の起動
- system変更: `HKEY_LOCAL_MACHINE`、`setx`、`reg.exe`、system `PATH`の書込み
- package manager: `winget`/`choco`/`apt`/`dnf`/`yum`/`pacman`の起動
- TLS: `InsecureSkipVerify`、独自CA bundle読込み
- test資材のimport: fake portのpackageをproduction pathからimportすること

Registry portはHKCUのみを受け付ける型とし、hive引数を取らないことをcompile時に保証する。

fake portのimport禁止は、決定的testのための細工がruntime経路へ載ることを防ぐ。fake package自身のfileと`_test.go`は対象外とする。

### 7.2 動的検査（`e2e` job）

E2E実行時はFileSystem、Registry、Process portを記録用wrapperで包み、次を検査する。

- 全write/move/delete先が、data root、distribution root、setup stateが宣言したintegration target（HKCU Path値または対象shell profile 1 file）、project fileのいずれかに含まれる（封じ込め検査）。判定は[04-storage-and-data.md](04-storage-and-data.md)§17.2の`path_role`で行う。管理root外への書込みは、Planの`writes[]`が宣言したintegration対象とproject fileだけであることを確認する
- 変更operationで起動した全probe processがPlan `probes[]`のexecutable/argv/cwd/write pathと一致し、任意helper/backend processがない
- 記録に昇格・system変更・package manager起動が現れない

port経由でしか外部作用が起きない構造（[02-architecture.md](02-architecture.md)）のため、この記録は実質的な全書込みの証跡になる。OS権限に依存しないので、runnerがadminであっても検査として有効である。

## 8. E2E scenario（CIで実行するもの）

`e2e` jobは両OSで次を自動実行する。scenarioはP11-01のfixture基盤（ローカルHTTPS疑似upstream、合成archive、擬似tool binary）で実行し、networkへ依存しない。標準4 toolと同じ形のtest定義を使い、標準definition実体の正しさは§6のcontract test（記録済みfixture）と§13手順7のlive smokeで検証する。

1. clean portable setupとclean user setup。setup後に**新しい子processを起動**してshimが解決されることを確認する。
2. `setup`の冪等性。2回目の実行がno-opとして報告される。
3. Go/Node.js/Python/.NET SDKの`available`/`install`/`use`/`current`/`installed`/`uninstall`。
4. `install`単独でselectionが変わらず、`--use`だけが変えること。
5. exact not-found時に`available`を案内し、近似候補を出さないこと。
6. `--latest`、prerelease、EOL、third-party Python、.NET WindowsのrestrictiveなlicenseのPlan要約と詳細、progress出力。明示承認なしでは`E_APPROVAL_REQUIRED`になること。
7. Go共有GOBIN、Node version別global prefix、Python venv/`--user`、.NET version別`DOTNET_CLI_HOME`とtool共有NuGet cacheの分離。
8. project完全versionがuser selectionより優先されること。
9. `--offline`、cache再利用、cancel、lock競合。
10. download失敗、probe失敗、disk full、commit直前中断のfailure injectionと、`tmp/operations/`の後始末。
11. 悪性archive/registry/receipt fixtureの拒否と、log/reportのmasking。
12. PATH integration: Windows HKCU Pathの型/長さ/rollback/remove、Linux shell profileのmarker追加/no-op/remove。
13. `setup --remove`がintegrationだけを除去し、tool/state/cacheを保持すること。
14. `doctor`と`doctor --report`が、正常時と破損fixture時の両方で期待どおり報告すること。
15. §7.2の書込み範囲検査を全scenarioで有効にすること。

管理者権限でしか成功しないtestは不合格とする。

## 9. 利用者確認チェックリスト（非blocker）

次はCIで自動化できない。release blockerにはせず、**確認結果と未確認項目をrelease noteと進捗台帳へ明記する**。

| # | 項目 | 確認方法 |
|---:|---|---|
| 1 | GUIアプリ（VS Code等）が新規process経由でshimを解決できるか | `user-path` setup後にVS Codeを起動し直し、統合terminalとlanguage serverで`node --version`等を確認 |
| 2 | 新しいterminalを開いたときのPATH反映と初回起動の体感 | 新規terminalを開いて`gdtvm current`とtool commandを実行 |
| 3 | Windows標準user（非Administrators）アカウントでの動作 | 標準userアカウントを作成し、bootstrapからquick startまでを一巡 |
| 4 | 実開発作業でのドッグフーディング | `go install`, `npm install -g`, `python -m venv`, `pip install --user`, `dotnet new`/`dotnet build`/`dotnet tool install -g`を実作業で使う |
| 5 | 長時間動作中のeditor/serviceへの影響 | `use`でversionを切り替え、既存processが影響を受けないことと案内文言を確認 |

不具合に遭遇した場合は`gdtvm doctor --report`を実行し、[14-maintenance.md](14-maintenance.md)§5の報告テンプレートに従って報告する。

## 10. logging/表示

日本語message catalogはkey/parameter集合をschemaとtestで検査する。human表示、単一JSON、no-color、非TTY、terminal幅狭小、Unicode非対応でも完全version/provider/checksum/warningを見落とさないsnapshot testを持つ。colorだけで状態を伝えない。

structured logはrotation/保持、secret masking、operation correlation、disk fullをtestする。専用audit logはrelease条件にしない。

`doctor --report`の出力はsnapshot testを持ち、fixtureに埋めたsecretが出力へ現れないことをnegative testで検査する。

## 11. release asset

GitHub Release asset許可集合は次のexactly 5件とする。

1. Windows amd64 archive
2. Linux amd64 archive
3. `checksums.txt`
4. `install.ps1`
5. `install.sh`

`checksums.txt`は4 download file（2 archive＋2 script）をASCII filename byte順で列挙し、各行を`<64 lowercase SHA-256><ASCII space 2個><basename><LF>`とする。BOM、CRLF、path、duplicate、extra/missingを拒否する。

SBOM、provenance、artifact attestation、署名はv0.1で生成・公開・必須化しない。将来追加する場合はasset許可集合、client検証、保証範囲を[15-deferred.md](15-deferred.md)の手順で先に仕様化する。

## 12. bootstrap script

### 12.1 共通

scriptはversion未指定時に公式repositoryの最新stable client、指定時に完全client versionだけを選ぶ。OS/arch、release/tag/asset identity、canonical checksums、archive SHA-256、archive entry構造を確認し、user mode distributionへstaging後renameする。既存active `distribution/current`が同じ完全versionなら検証後no-op、異なる完全versionなら検証済みstagingからdirectory単位で置換する。旧distributionのbackupと自動rollbackは行わず、置換または続くsetupが失敗した場合は完全client versionを指定した再実行方法を表示する。tool payload、state、storageを移動・削除しない。任意のdistribution隣接`gdtvm.toml`はowner、regular file、sizeを検査し、raw bytesのまま新stagingへ引き継ぐ。commit失敗時は設定copyを所有するoperation tempに残し、復旧pathを表示する。

GitHub API/release downloadのhostは`api.github.com`, `github.com`, `release-assets.githubusercontent.com`のexact集合だけを許し、302ごとに再検査する。

公開interfaceは次のexact形。version/path integration/shell以外のrepository、download URL、checksum URL、install root、TLS optionを受けない。script内に公式GitHub owner/repositoryとasset grammarを固定する。

```text
install.ps1 [-Version <YYYY.MM.DD.XX>]
            [-NonInteractive -PathIntegration <user-path|none>]

install.sh [--version <YYYY.MM.DD.XX>]
           [--non-interactive --path-integration <shell-profile|none>
            [--shell <bash|zsh|fish>]]
```

`-NonInteractive|--non-interactive`はpath integration必須。Linuxの`shell-profile`時だけshellも指定でき、`none`時はshell禁止。Windowsに`shell-profile`はない。非対話invocation自体をsetup変更への明示承認とみなし、検証済みclientへ`--non-interactive --yes --mode user`と選択値を渡す。通常invocationはclientの対話Plan/既定Noを維持する。

secret/credentialを引数・logへ出さず、TLS無効化、root/admin昇格、system PATH変更、package installをしない。必要なdownload/hash/extract機能がなければ変更前に不足commandとmanual手順を表示する。一時directoryを必ず固有作成し、所有物だけを清掃する。成功時は全て清掃し、current除去後のcommit失敗時だけ利用者`gdtvm.toml`の復旧copyを残してabsolute pathを表示する。その他の失敗では全て清掃する。

install成功後、同じarchiveのclientをabsolute pathで`setup --mode user`として対話起動する。非対話時は上記以外の暗黙fallbackをしない。

### 12.2 `install.ps1`

Windows PowerShell 5.1+の標準機能だけを使い、`.NET Environment.GetFolderPath(SpecialFolder.LocalApplicationData)`でLocalAppDataを得る。空値を`LOCALAPPDATA`へfallbackせず停止する。`Invoke-WebRequest`/`.NET HttpClient`のTLS検証を無効化しない。SHA-256、ZIP entry、target version/archを検査し、`<data-root>/distribution/current`へcommitする。user PATH変更はscriptで直接行わず、検証済みclientのsetupへ委ねる。

### 12.3 `install.sh`

POSIX shとして実装し、`id`, `getent`, HTTPS download (`curl|wget`の検証済み1件), SHA-256 (`sha256sum|shasum -a 256`), tar/gzipの存在をprobeする。実UIDのpasswd homeを使い`HOME|XDG_*`をroot決定に使わない。

archive entryは抽出前にASCII許可grammar、absolute/`..`/link/device/duplicate、top-level集合を検査し、owner/permissionを引き継がずstagingへ展開する。機能差があるtarへ危険なoption fallbackをしない。

### 12.4 CI検証

`bootstrap` jobは次を行う。

1. PowerShell/shellのsyntax・static検査。
2. fixture releaseを配置したlocal HTTPS serverに対する実行。
3. 導入後のarchive構造、binary version、setup state、shim解決の確認。
4. checksum不一致、asset欠落、archive構造違反、host allowlist違反の各negative case。
5. 同一version再実行のno-op、異なる完全versionへの置換、失敗時に旧distribution backupを作らず再取得手順を表示すること。任意`gdtvm.toml`のbyte完全引継ぎと、commit失敗時の設定copy復旧pathも検査する。

### 12.5 公開手順

READMEはscriptをdownloadして公開元/内容を確認後に実行する例を主とし、pipe実行だけを唯一の導線にしない。手動archive方法は`checksums.txt`照合、構造確認、展開、portable setupを記載する。

## 13. release手順

1. `develop/work`の全6 job、進捗証跡、release対象、clean worktree、remote tag一覧を確認する。annotated tag作成予定日のJST日付と既存最大通番からversionを決め、root `VERSION`を更新する。
2. `develop/work→main`のrelease PRを作る。この時点からrelease公開完了までagent work→developのmergeを凍結する。feature→agent workは§5.3のgateを満たす範囲で継続できる。
3. release PRの全6 jobが最新head commitでgreen、conversation未解決0件であることを確認し、指定maintainerがmerge commitでmainへmergeする。続くmain pushの全6 jobもgreenにする。
4. annotated tagを作る当日のJST日付、同日最大通番＋1、`VERSION`、tag候補が一致することを再確認する。日付または通番が変わった場合はtagを作らず、developでVERSIONを更新して手順2から繰り返す。
5. 指定maintainerがmainのrelease merge commitを指すannotated tag `v<version>`を作成してpushする。tagger timestampをrelease日付の正本とし、tag pushだけをrelease workflowのtriggerにする。
6. release workflowは、annotated tag、tagger timestampのJST実在日付、同日通番、tagが指すmain commit、root VERSION、未変更のrelease sourceを最初に検査する。不一致ならassetを作成・公開しない。
7. pinned Go/action/dependencyで2 targetをbuildし、registry strict validation、文書link/example、license、標準4 toolのlive metadata smokeを実行する。
8. 5 assetとrelease noteをstagingし、filename、size、digest、構造、binary versionを照合する。§9の利用者確認チェックリストの実施状況と未確認項目もrelease noteへ記載する。
9. GitHub Release titleをtagと同じ`v<version>`として5 assetを一度だけ公開する。tag、release、公開assetを変更・削除・上書きしない。
10. 公開URLから全5 assetを再downloadし、checksums、archive、binary、registry、script bytesを再検査する。
11. §5.5に従ってdevelopとagent workを更新し、release freezeを解除する。report path、command、result、未実施項目、次taskを[13-progress.md](13-progress.md)へ記録する。

どれかが失敗したreleaseを完了扱いにしない。tag作成前の失敗は修正commitとrelease PRのCI再実行で扱う。tag作成後の失敗はtagを残し、修正commitと同日の次通番（`99`なら翌日）で新しいreleaseを行う。公開済みassetを差し替えない。release PR作成後のfreezeは、release成功またはtag作成前のrelease中止を指定maintainerが記録するまで維持する。

## 14. 完了条件

- `lint`/`unit`/`policy`/`package`/`bootstrap`の全jobがgreen。
- `e2e` jobが両OSでgreen（§8の15 scenario）。
- 4 tool×2 platformのexact install/use/command/uninstallが、fixture E2Eとrelease workflowのlive smoke（[07-registry-and-tools.md](07-registry-and-tools.md)§12）で成功。
- 公開後の5 asset再検査が成功。
- repository README/USER_GUIDEが実装済み範囲と一致。
- §9のチェックリストが提示され、実施済み/未実施が明記されている。
- 進捗台帳に全command、結果、report path、次taskが記録済み。

byte再現build、性能目標、追加platform、延期commandはv0.1の完了条件ではない。
