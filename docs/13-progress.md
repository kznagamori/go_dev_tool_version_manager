# 実装・評価進捗チェックリスト

## 1. 目的

本書は`go_dev_tool_version_manager`（`gdtvm`）のv0.1実装、標準registry、評価、公開文書、releaseを途中停止・再開できる単一進捗台帳である。**WindowsとLinuxを同時に開発し、CI matrixを唯一のgateとする**。

checkboxを満たすために仕様を弱めない。仕様変更時は番号付き仕様、fixture/test、本書を先に同期する。v0.1の完成対象はWindows amd64、Linux amd64/glibc、Go/Node.js/Python/.NET SDK、portable/user、9 commandだけ。

## 2. 進捗スナップショット

| 項目 | 現在値 |
|---|---|
| 全体状態 | `進行中` |
| 現在フェーズ | `P1` |
| 実行中タスクID | `P1-01` |
| 最後に完了したタスクID | `P0-03（port interfaceとfake）` |
| 次に開始するタスクID | `P1-01`（G-CI達成後） |
| CI状態 | `両OS 12 checkがgreen。required status check登録済み。Go検査はpackage投入で自動有効化` |
| blocker | `なし` |
| 最終更新日時 | `2026-08-10T01:40:31+09:00` |
| 更新者 | `Claude Code` |
| 作業branch | `claude/feature-p1-01-package-skeleton` |
| 使用環境 | `Linux container` |
| 最新の証跡 | [P0-03 決定記録](reviews/P0-03-port-interfaces-and-fakes.md)、CI 12/12 success、`go test -race` coverage 84.5%、`govulncheck`の`No vulnerabilities found.` |

全体状態は`未着手|進行中|停止中|blocked|完了`。実行中taskは同時に1件だけ。値なしは`なし`と記す。

## 3. 記法と更新

- `[ ]`: 未着手
- `[-]`: 進行中。作業終了前に`[ ]|[x]|[!]`へ戻す
- `[x]`: 全受入条件と証跡を満たした完了
- `[!]`: blocker。試行・解除条件を停止記録へ記載
- `[~]`: 仕様変更で不要。置換先と判断記録を記載

完了時はcommitまたは未commit差分、検証command/result、report/判断記録pathを証跡へ書く。未実施・失敗・証跡なしを完了にしない。

### 3.1 停止

1. processを安全に止め、生成途中・未commit差分を確認する。
2. 未完了taskを`[ ]`へ戻し、完了部分、残作業、blocker、再開最初の具体操作を書く。
3. snapshotのlast/next/branch/environment/evidence/timeを更新する。
4. `[-]`を残してturn/sessionを終了しない。

### 3.2 再開

1. snapshotと最新停止記録を読む。
2. branch/commit/worktree/environmentを再確認する。
3. blockerと依存gateを確認する。
4. 最後の関連成功testを再実行する。
5. next task 1件だけを`[-]`にする。

### 3.3 停止・再開記録

新しいrecordを上へ追加する。

| 日時 | 状態 | 完了済み部分 | 残作業・次の具体操作 | blocker/解除条件 | branch | 環境 | 証跡 |
|---|---|---|---|---|---|---|---|
| 2026-08-10T01:40:31+09:00 | P0-03 完了・G-CI達成 | 着手時に台帳の依存循環を検出した。P0-03「fake portを作る」の対象interfaceがP1-03「全portのinterface定義とfake」で定義される順序になっており、置き場所のpackage骨格もP1-01（依存: G-CI）だった。利用者判断で選択A（P0-03をinterface定義まで拡張し、P1-03を依存注入とglobal mutable state不存在のtestへ縮小）と6 port先行を確定し、台帳の2行を同期修正した。`internal/domain/port`へClock/FileSystem/LinkManager/HTTPClient/ProcessRunner/UserLookupの6 interfaceと`Ports`を、`internal/domain/port/fake`へ6 fakeと`Injector`（failure injection）と`Set`を実装した。配置は§1「抽象ポートはcore側が所有」とimport cycle回避の両立から決め、§2の表に行が無いことをPRで明示して確認を求めた。`policy` jobへproduction pathからのfake import禁止を追加し§7.1へ規範化した。PR #14のCIで両OS 12 checkがgreenになり、`govulncheck`が`No vulnerabilities found.`、依存license検査が13 module、`unit`が実packageに対する`go test -race`で成功した。これによりP0の全taskが完了しG-CIを満たした | P1-01で`cmd/gdtvm`と§2のpackage骨格、package comment、依存方向のstatic checkを作る。static checkには`internal/domain/port`がdomain値と標準libraryだけに依存する制約も含める | なし。`internal/domain/port`という配置が§2の表に無いため利用者確認待ち。P1-02でdomain値が入ったらport signatureをdomain型へ寄せられるか見直す | `claude/feature-p0-03-fake-ports` | Linux container / Go 1.26.5・Python 3.11.15、CIはPython 3.12 | [P0-03 決定記録](reviews/P0-03-port-interfaces-and-fakes.md)、CI 12/12 success、`go test -race -shuffle=on` coverage 84.5%、test 40件、禁止import検査のnegative、`git diff --check` |
| 2026-08-10T00:05:17+09:00 | P0-02 完了 | 利用者判断2件（license検査は自作script、coverageは計測のみで閾値なし）を確定し、[11-quality-and-ci.md](11-quality-and-ci.md)へ§1.1〜§1.5を追加した（§2以降は番号を変えていない）。module path、`go 1.26.0`＋`toolchain go1.26.5`、job別の固定command、依存license許可list（MIT/Apache-2.0/BSD-2-Clause/BSD-3-Clause/ISC）、証跡directory`docs/reviews/<TASK-ID>-<slug>.md`、証跡のsecret除去規則を規範化した。実装は`go.mod`/`go.sum`（govulncheckを`tool` directiveで固定）、`scripts/ci/check_licenses.py`、`lint`/`unit` jobのGo検査、`.gitattributes`。Go versionの正本を`go.mod`だけにし、`lint`が実行中Go versionとの一致を検査する。package 0件では`go vet`/`go test`がexit 1になるため`go list ./...`の空判定で分岐した。一時package`internal/ciprobe`をPRへ入れてCIで実測し、確認後に削除した。実測でWindowsのcheckoutがCRLF化して`gofmt`が全Go fileを未format扱いにする両OS差を発見し、`.gitattributes`の`* text=auto eol=lf`で修正した。あわせてWindowsで`-race`が付くこと、`govulncheck`が両OSで実行できることを確認した | P0-03でfake port（clock/HTTP/process/filesystem/link/user lookup）とfailure injection基盤を作る。P1-01でpackage骨格が入った時点で`go vet`/`go test`/`govulncheck`が自動的に実検査へ切り替わるため、最初のPRで3つが両OSでgreenになることを確認する | なし。`govulncheck`はこのcontainerから`vuln.go.dev`が`Forbidden`で到達できず、検証手段はCIだけである。coverage閾値は実測値が揃ってから別taskで判断する | `claude/feature-p0-02-toolchain` | Linux container / Go 1.26.5（GOTOOLCHAIN取得）・Python 3.11.15、CIはPython 3.12 | [P0-02 決定記録](reviews/P0-02-toolchain-and-commands.md)、CI 12/12 success、全13 stepの`bash -eo pipefail`実行、license検査のpositive/negative、`git diff --check` |
| 2026-08-09T03:47:50+09:00 | P0-01 完了 | PR #5が`claude/work`へmergeされ、6 job×2 OSの12 checkがgreenであることを確認した。指定maintainerが12 check名を`main`、`develop/work`、`claude/work`、`codex/work`のrequired status checkへ登録し、利用者確認を得たため§5.6手順5を満たした。あわせてpush確認用の`claude/go-dev-tool-version-manager-w9h1z1`とfeature branch 2本が削除され、remoteは`main`、`develop/work`、`claude/work`、`codex/work`の4本に戻った | P0-02でGo module/toolchain、format/vet/lint/test/coverage command、証跡directory・命名・secret除去規則を固定する。`lint` jobの`gofmt`/`go vet`と`unit` jobは`go.mod`が入った時点で実検査へ切り替わるguardになっているため、同じ変更で`actions/setup-go`のpinとvulnerability/license検査commandを追加する必要がある | なし。PR #3とPR #5がいずれもmerge commitで統合されており、§5.3が定めるfeature→agent workのsquash mergeと異なる。repository設定でsquash mergeが有効か未確認。`develop/work`はP0-00までで、`codex/work`は`9e69261`のまま。§5.5の同期は指定maintainerの作業 | `claude/feature-p0-01-required-checks` | Linux container / Go 1.24.7・Python 3.11.15 | PR #5（commit `44fc277`）、CI run 12/12 success、`git ls-remote --heads`、利用者によるrequired status check登録確認 |
| 2026-08-09T03:38:40+09:00 | P0-01 実装済み・check登録待ち | `.github/workflows/ci.yml`で6 job（`lint`/`unit`/`e2e`/`policy`/`package`/`bootstrap`）×`ubuntu-latest`/`windows-latest`のmatrixを作り、`paths` filterとOS分岐を置かず`fail-fast: false`とした。`scripts/ci/check_docs.py`（相対link、anchor、code fence、table列数）、`scripts/ci/check_pr_refs.py`（§5.2命名grammarと§5.3のsource→target）、`scripts/ci/check_policy.py`（§7.1の昇格/system変更/package manager/TLS）を実装した。中身が未実装のjobは入力の有無を判定し、入力が現れた時点で失敗するguardにしてplaceholderが残らないようにした。lint検査を通すため`docs/04-storage-and-data.md`の7 recordと`docs/reviews/W00-01-specification-audit.md`の1 recordでinline code内の`\|`未escapeを修正した。全10 stepを`bash -eo pipefail`で実行し、入力なしで全成功、入力ありで未実装7 stepが失敗することを確認した。PR ref検査はpositive 6件・negative 12件・skip 1件が期待どおり。PR #5でCI run #1を実行し12 check中10件がgreen、`lint (windows-latest)`と`policy (windows-latest)`がfailした。WindowsのPythonがstdoutを`cp1252`で開くため日本語出力が`UnicodeEncodeError`になる両OS差で、3 scriptへstdout/stderrのUTF-8固定を追加して修正した。修正後のcommit`6e1aa8a`に対するrunで**両OS 12 checkが全てsuccess**になり、§5の6 job×2 OSが最小構成でgreenになることを確認した。あわせて`py_compile`が生成した`__pycache__`を追跡から外し、`.gitignore`へPython bytecodeを追加した | 指定maintainerが12 check名を`main`、`develop/work`、`claude/work`、`codex/work`のrequired status checkへ登録して§5.3～§5.4のrulesetを完成させる（§5.6手順5）。登録が済んだ時点でP0-01を`[x]`にし、G-CIの残りをP0-02へ進める | なし。required status check登録はREST APIがこのsessionから使えないため指定maintainerの作業 | `claude/feature-p0-01-ci-matrix` | Linux container / Go 1.24.7・Python 3.11.15（CIはPython 3.12へ固定） | `.github/workflows/ci.yml`、`scripts/ci/check_docs.py`、`scripts/ci/check_pr_refs.py`、`scripts/ci/check_policy.py`、guard論理のpositive/negative実行、`PYTHONIOENCODING=cp1252`での再現・修正確認、CI run #1、`git diff --check` |
| 2026-08-09T03:11:40+09:00 | P0-00 完了 | 指定maintainerが実施した[11-quality-and-ci.md](11-quality-and-ci.md)§5.6の初期登録を検証した。`main`初期commit`2cb8bc5`が生成3 file（`README.md`/`LICENSE`/`.gitignore`）だけを持つこと、LICENSEがMIT・`Copyright (c) 2026 kznagamori`でmain版とdevelop版が同一であること、`.gitignore`がGo templateへ`.cache/`・`artifacts/`を追加した現在版であること、`README.md`が現在版であること、`develop/work`が現在の作業tree内容だけを持つこと、`claude/work`と`codex/work`が`develop/work`と同一commit`9e69261`であること、remoteのroot commitが`2cb8bc5`のみで旧root`fb754ed`系列が到達不能であること、remote tagが0件であることを機械検証した。branch protectionと`v*` immutable tag rulesetはREST APIがproxyに遮断され機械検証できず、利用者へ1問確認して「設定済み」の回答を証跡とした。作業branchへのpush権限も確認した。あわせて、agent sessionからremote branchを削除できない（ref削除が`403`）ことが判明したため、branch削除を指定maintainerの作業と明記する形で§5.2、§5.3、§5.5、`CLAUDE.md`§4.1、`AGENTS.md`§4.1を同期修正した | P0-01でCI matrix workflow（`lint`/`unit`/`e2e`/`policy`/`package`/`bootstrap`）を最小構成で作り、両OSでgreenにしてから最初の成功check名を4 protected branchのrequired status checkへ登録する（§5.6手順5）。同時にPRのsource/target/branch名policy検査を追加する。push確認用の`claude/go-dev-tool-version-manager-w9h1z1`（固有commitなし、`claude/work`と同一`9e69261`）の削除を指定maintainerへ依頼済み | なし。branch protection値と§5.3のrepository merge設定（squash/merge commit許可、rebase merge無効）はこのsessionからREST APIで読めず未照合。P0-01の最初のPRで実効性を観測して[P0-00 verification report](reviews/P0-00-repository-bootstrap-verification.md)§4へ反映する | `claude/feature-p0-00-branch-topology` | Linux container / Go 1.24.7（§1のminimum Go 1.26系未満のためlocal build・testは未実施） | [P0-00 verification report](reviews/P0-00-repository-bootstrap-verification.md)、`git ls-remote --heads/--tags`、`git ls-tree origin/main`、`git diff origin/main:<file> origin/develop/work:<file>`、`git rev-list --max-parents=0 --all` |
| 2026-08-09T02:33:50+09:00 | S00-06 完了 | 利用者のモチベーション・実装量・複雑度・CI E2E制約・ドッグフーディング観点で全16文書を再レビューし、利用者判断7件を確定・反映した。(1)e2e jobを全面fixture化しlive検証をrelease時live smoke（実artifactの4 tool一巡を追加）と§9利用者チェックへ移動、(2)G-TOOLS後にDF-01でdevel buildのドッグフーディングを開始、(3)Plan契約を縮約（`reads[]`廃止・`inputs`へ`registry_sha256`追加、`writes[]`を利用者可視変更へ限定、`rollback[]`廃止、CI §7.2を封じ込め検査へ）、(4)読取り5 commandの`--json`は維持、(5)message catalog `ja.toml`は維持、(6)user mode/bootstrapは維持、(7)download再開・`available` filter・`download.concurrency`を延期しD-23〜D-26へ記録した。root README.mdへ仕様策定中statusを追加した | P0-00でrepositoryのbranch topology（`develop/work`、`codex/work`、protection、tag ruleset）を整備する | なし。remote branch操作は指定maintainerが行う | `claude/go-dev-tool-version-manager-7zyakj` | Linux container | [S00-06 review report](reviews/S00-06-motivation-simplification-review.md)、link/anchor/fence/stale用語検査、`git diff --check` |
| 2026-08-08T18:56:21+09:00 | S00-05 完了 | 利用者判断16件をbranch topology、命名、同期・再作成、merge、protection、CI、release freeze、CalVer、annotated tag起動releaseへ同期した。`v0.1`と実versionを分離し、Go module非互換、同日`00`～`99`、失敗tag非再利用、repository再作成手順を明記した。link 181件/anchor 22件、section参照58件、fence 156 marker、Git ref 5件、feature正負6件、CalVer正負7件、CI job 6件、PR経路5件、stale表記0件、`git diff --check`が全て成功した | P0-00でGitHub repositoryを再作成し、main初期commit→develop/work→両agent workを作成して初期protection/tag rulesetを設定する | なし。remote操作、workflow、VERSION、Go sourceは未実施 | `main` | Windows 10.0.26200 x64 / PowerShell 7.6.4 / Go 1.26.5 | [S00-05 review report](reviews/S00-05-branch-version-strategy-review.md)、inline PowerShell validators、`git diff --check` |
| 2026-08-08T18:43:11+09:00 | S00-05 再開 | release中並行開発は選択Aに確定した。release PR作成からtag起動release CIの公開完了までagent work→develop/workのmergeを凍結し、feature→agent workは継続できる | S00-05を`[-]`へ戻し、[11-quality-and-ci.md](11-quality-and-ci.md)から関連文書へ確定仕様を同期する | なし | `main` | Windows 10.0.26200 x64 / PowerShell 7.6.4 / Go 1.26.5 | 利用者回答A |
| 2026-08-08T18:42:01+09:00 | S00-05 release中並行開発判断待ち | 全判断を仕様へ展開する過程で、`develop/work→main`のrelease PR中にagent workをdevelopへmergeするとrelease対象commitが変動する未定義状態を検出した。既存文書の修正には未着手で、判断recordだけを更新した | release PR作成からrelease公開完了までのdevelop merge規則を確定後、S00-05を`[-]`へ戻し、最初に[11-quality-and-ci.md](11-quality-and-ci.md)を修正する | 利用者がrelease中並行開発のA～Cを選択する | `main` | Windows 10.0.26200 x64 / PowerShell 7.6.4 / Go 1.26.5 | branch topologyとrelease手順の状態遷移対照 |
| 2026-08-08T18:40:21+09:00 | S00-05 再開 | approval数は選択Aに確定した。必須approvalは0件とし、PR、必須CI、未解決conversationなし、指定maintainerによるmergeを要求する。全てのbranch/version/release判断が確定した | S00-05を`[-]`へ変更し、最初に[11-quality-and-ci.md](11-quality-and-ci.md)へbranch lifecycle、CI gate、CalVer/release契約を反映する | なし | `main` | Windows 10.0.26200 x64 / PowerShell 7.6.4 / Go 1.26.5 | 利用者回答A、本書のS00-05判断record一式 |
| 2026-08-08T18:39:19+09:00 | S00-05 approval数判断待ち | release後の混在状態は選択Aに確定した。`develop/work`をmainへ、作業中agent workをdevelopへ、そのfeatureをagent workへ順にrebaseし、非作業中agent workは削除・再作成する。両agentが非作業中ならdevelopと両agent workを削除・再作成する | 保護branchのPRにGitHub approval reviewを何件要求するか確定する | 利用者がapproval数のA～Cを選択する | `main` | Windows 10.0.26200 x64 / PowerShell 7.6.4 / Go 1.26.5 | 利用者回答A |
| 2026-08-08T18:37:30+09:00 | S00-05 release後混在状態判断待ち | CI範囲は選択Aに確定した。feature→agent workはWindows/Linuxの`lint`、`unit`、`policy`、agent work→develop/workとdevelop/work→mainは両OSの全6 jobを必須とする。rebase後も次のmerge前に該当CIを再実行する | release時に片方のagent workだけが作業中の場合のdevelop/workと両agent workの更新順を確定する | 利用者がrelease後混在状態のA～Cを選択する | `main` | Windows 10.0.26200 x64 / PowerShell 7.6.4 / Go 1.26.5 | 利用者回答A |
| 2026-08-08T18:26:07+09:00 | S00-05 CI範囲判断待ち | repository再作成時のmain初期commitは選択Cに確定した。GitHubでREADME、LICENSE、`.gitignore`を生成し、そのcommitから`develop/work`を作成して現在のデータを登録する。同名fileは差分確認後にdevelop側へ反映し、agent workはdevelopから作る | feature→agent work、agent work→develop/work、develop/work→mainで必須にするCI job範囲を確定する | 利用者がCI範囲のA～Cを選択する | `main` | Windows 10.0.26200 x64 / PowerShell 7.6.4 / Go 1.26.5 | 利用者回答C |
| 2026-08-08T18:20:54+09:00 | S00-05 repository再作成判断待ち | 既存remote branchの移行案は撤回し、現在の作業treeを使用してrepositoryを再作成する利用者方針へ変更した。新repositoryではGitHub作成直後の`main`を起点とし、そこから`develop/work`を作成して現在の文書を登録し、その後agent workを作る | `main`と`develop/work`の共通祖先になるmain初期commitの内容を確定後、S00-05を再開して仕様文書を同期修正する | 利用者がmain初期commitのA～Cを選択する | `main` | Windows 10.0.26200 x64 / PowerShell 7.6.4 / Go 1.26.5 | 利用者指定のrepository再作成方針 |
| 2026-08-08T18:17:31+09:00 | S00-05 初期移行判断待ち | release起動方式は選択Aに確定した。main向けPRと必須CI成功後、指定maintainerがVERSIONと一致するannotated tagをpushし、tag起動CIがVERSION、tag名、JST日付、通番、main commit到達性を検査してbuild・公開する。現存する`origin/claude/work`と旧Claude feature branchはともに`main`の祖先で、remote固有commitがないことも再確認した | 旧branchの一回限りの移行方法を確定後、S00-05を再開して仕様文書を同期修正する | 利用者が初期移行のA～Cを選択する | `main` | Windows 10.0.26200 x64 / PowerShell 7.6.4 / Go 1.26.5 | 利用者回答A、`git rev-list --left-right --count`、`git log --all` |
| 2026-08-08T18:16:29+09:00 | S00-05 release起動判断待ち | CalVer境界は選択Aに確定した。日付はannotated tag作成時刻のJST日付、同日通番はremoteに過去作成されたtagを含む次の未使用`00`～`99`とし、失敗分も再利用しない。`99`の次は翌日までreleaseを停止する | tag作成主体とrelease CI triggerを確定後、branch初期作成・移行手順を確認する | 利用者がrelease起動方式のA～Cを選択する | `main` | Windows 10.0.26200 x64 / PowerShell 7.6.4 / Go 1.26.5 | 利用者回答A |
| 2026-08-08T18:15:20+09:00 | S00-05 CalVer境界判断待ち | version方式は選択Aに確定した。client version/tagは`YYYY.MM.DD.XX`/`vYYYY.MM.DD.XX`を維持し、正式配布はGitHub Releasesのarchive/bootstrapに限定する。CalVer tagによる`go install`は非対応とし、`v0.1`は実versionでなく機能scope・release段階名とする | 日付基準、同日通番の割当・上限を確定後、release起動方法を確認する | 利用者がCalVer境界のA～Cを選択する | `main` | Windows 10.0.26200 x64 / PowerShell 7.6.4 / Go 1.26.5 | 利用者回答A、Go Modules・SemVer公式仕様 |
| 2026-08-08T18:14:20+09:00 | S00-05 version方式判断待ち | feature branch命名は選択Aに確定した。`<agent>/feature-<task-id>-<slug>`とし、agentは`claude\|codex`、task IDは進捗台帳の小文字表記、slugは小文字ASCII英数字を単一hyphenで連結した1～48文字とする | CalVerとGo module非互換の扱いを確定後、同日通番・release日境界を確認する | 利用者がversion方式のA～Cを選択する | `main` | Windows 10.0.26200 x64 / PowerShell 7.6.4 / Go 1.26.5 | 利用者回答A、Git `check-ref-format`公式文書 |
| 2026-08-08T18:13:27+09:00 | S00-05 branch命名判断待ち | 保護例外主体は選択Aに確定した。`main`には例外を設けず、指定maintainerだけが事前の作業中判定後に`develop/work`とagent workの削除・再作成、およびagent workへの`--force-with-lease`を使用できる。通常の直接pushは禁止を維持する | feature branch名のexact grammarを確定後、CalVer戦略を確認する | 利用者がbranch命名のA～Cを選択する | `main` | Windows 10.0.26200 x64 / PowerShell 7.6.4 / Go 1.26.5 | 利用者回答A、GitHub公式ruleset文書 |
| 2026-08-08T18:12:00+09:00 | S00-05 保護例外判断待ち | agent workの作業中判定は選択Aに確定した。未mergeの`<agent>/feature-*`、agent workをhead/baseとするopen PR、または`develop/work`に未反映のagent work固有commitが1件でもあれば作業中とする | rebaseに必要なforce updateと非作業時の削除・再作成を、保護規則上どの主体へ許可するか確定する | 利用者が保護例外主体のA～Cを選択する | `main` | Windows 10.0.26200 x64 / PowerShell 7.6.4 / Go 1.26.5 | 利用者回答A |
| 2026-08-08T18:11:06+09:00 | S00-05 作業中判定待ち | 同期は利用者指定方式に確定した。agent workが作業中なら`develop/work`へrebaseし、非作業中なら削除して最新`develop/work`から再作成する。mainへのrelease PR後も、非作業中なら`develop/work`とagent workを削除し、最新`main`から`develop/work`、そこからagent workを再作成する | feature PRを失わない「作業中」のexact判定を確定し、その後rebase・削除に必要な保護規則の例外主体を確認する | 利用者が作業中判定のA～Cを選択する | `main` | Windows 10.0.26200 x64 / PowerShell 7.6.4 / Go 1.26.5 | 利用者指定同期方針 |
| 2026-08-08T18:06:10+09:00 | S00-05 同期方法判断待ち | protection/CI対象は選択Aに確定した。`main`、`develop/work`、`claude/work`、`codex/work`を保護し、PR、必須CI、最新base、直接push・force-push・削除禁止を要求する。merge commitを許可するためlinear historyは要求しない | `develop/work`の更新を再利用するagent workへ戻す方法を確定後、branch命名を確認する | 利用者が同期方法のA～Cを選択する | `main` | Windows 10.0.26200 x64 / PowerShell 7.6.4 / Go 1.26.5 | 利用者回答A、GitHub公式ruleset文書 |
| 2026-08-08T18:04:31+09:00 | S00-05 protection判断待ち | merge方式は選択Aに確定した。feature→agent workはsquash mergeしてfeature削除、再利用するagent work→develop/work→mainはmerge commitでcommit到達関係を維持する。長期branch間ではsquash/rebase mergeを禁止する | protection/CI対象を確定後、agent workの同期とbranch命名を確認する | 利用者がprotection/CIのA～Cを選択する | `main` | Windows 10.0.26200 x64 / PowerShell 7.6.4 / Go 1.26.5 | 利用者回答A、GitHub公式PR merge文書 |
| 2026-08-08T18:03:00+09:00 | S00-05 merge方式判断待ち | branch topologyは選択A（`feature→agent work→develop/work→main`の二段階PR）に確定した | 長期branchへ過去変更を再提示しないmerge方式を確定し、その後branch protection/CIを確認する | 利用者がmerge方式のA～Cを選択する | `main` | Windows 10.0.26200 x64 / PowerShell 7.6.4 / Go 1.26.5 | 利用者回答A、GitHub公式PR merge文書 |
| 2026-08-08T18:01:16+09:00 | S00-05 branch/version仕様判断待ち | `docs/README.md`、`11`、`13`を全文確認し、repository全体のbranch/version/release記載を監査した。現在は`main=f48df04`、`origin/claude/work=76b4043`だけが存在し、`develop/work`と`codex/work`は未作成。提案されたbranch名はGit refとして有効。一方、長期agent work branchへのsquash反復、developとの同期、二段階PRのCI、CalVer tagのGo module非互換、同日通番上限、release公開日確定が未規定と確認した | branch topologyから1問ずつ確定する。再開時は最初に選択結果を本recordへ追記し、次のmerge方式を確認する | 利用者がbranch topologyのA～Cを選択する | `main` | Windows 10.0.26200 x64 / PowerShell 7.6.4 / Go 1.26.5 | `git branch --all --no-color`、`git log --all`、Git `check-ref-format`、GitHub protected branch/PR merge、Go Modules、SemVer 2.0.0公式文書 |
| 2026-08-08T17:41:10+09:00 | S00-04 文書整合性レビュー完了 | 現行16文書と指示文書を再監査し、.NET SDKのSHA-512/親公開日時/license/storage/probe/管理外領域を他3 toolと同じ契約軸へ同期した。definition、Plan/SetupPlan/PlanArg、CLI JSON、warning、doctor、PathValue、bootstrap、registry読込みの不整合を修正し、利用者判断13件をreview reportへ記録した。相対link/anchor 169件、linked §参照54件、fence marker 150件、JSON 5件、JSONL 1件、TOML 24件、error 34件、Plan warning 8件、Result warning 5件、diagnostic 10件、path role 22件、SetupPlan field 15件が全て成功/期待値一致。stale用語0件、`git diff --check`成功 | P0-01でCI matrix workflowの骨格を作る | なし。.NET definition実体/fixtureはP10-04、global tool実測はP10-06 | `main` | Windows 10.0.26200 x64 / PowerShell 7.6.4 / Go 1.26.5 | `docs/reviews/S00-04-document-consistency-review.md`、inline PowerShell link/section/fence/JSON/TOML/enum validators、`git diff --check` |
| 2026-08-08T16:40:38+09:00 | S00-04 再開 | 利用者が追加判断で選択Aを確定し、Planへ`setup`専用objectを追加する方針とした。branch `main`、commit `f48df04`、既存差分が本taskの文書変更だけであることを再確認した | S00-04を`[-]`へ戻し、最初に`04`§16へsetup objectのexact contractを追加する | なし | `main` | Windows 10.0.26200 x64 / PowerShell 7.6.4 / Go 1.26.5 | 利用者回答A、`git status --short`、`git rev-parse --short HEAD`、`go version` |
| 2026-08-08T16:39:22+09:00 | S00-04 追加仕様判断待ち | 利用者判断12件を関連仕様へ同期し、追加横断検査で`strip_components`をv0.1使用値`0\|1`へ閉じた。Plan配列、path role、延期契約を再監査した | `setup`が表示必須とするmode/data root/distribution root/filesystem能力/shim/integration/backupをtyped Planのどこへ保持するか確定後、最初に`04`§16のPlan schemaへ反映する。その後、残る軽微な記述修正、review report、全文機械検査を行う | 利用者がPlan schemaの選択肢A～Cを確定する | `main` | Windows 10.0.26200 x64 / PowerShell 7.6.4 / Go 1.26.5 | `git diff`、`03`§3.1、`04`§16、`09`§12の対照 |
| 2026-08-08T16:07:14+09:00 | S00-04 再開 | 利用者判断が必要な12件を1問ずつ確定した。`.NET global tool`のP10-06実測維持、InstallSummary、bootstrap更新、current path、warning型、doctor status/exit、.NET公開日時、Plan reads、PathValue、Diagnostic code、channel pointerの契約を確定した | S00-04を`[-]`へ戻した。最初に`04`のPlan/JSON/enum契約を修正し、関連文書へ同期する | なし | `main` | Windows 10.0.26200 x64 / PowerShell 7.6.4 / Go 1.26.5 | 本書§3.3直前recordと利用者回答 |
| 2026-08-08T15:16:30+09:00 | S00-04 仕様判断待ち | 現行16文書を全文監査し、相対link 153件、§参照46件、code fence 154 marker、JSON 6例、TOML 24例を機械検査した。.NETのrelease metadata、license、global tool配置契約を一次資料と照合し、修正候補を抽出した。利用者判断により`.NET global tool`は選択B（P10-06の両OS実測を維持し、結果まで管理範囲を確定しない）、`InstallSummary`は選択A（receiptにないchannel/lifecycleを削除）、bootstrap更新は旧distributionのbackup/自動rollbackなしで検証済みstagingから`current`を置換し、復旧は開発利用者による旧版再取得、`SelectionSummary`は選択A（単数`command_path`を`payload_path`へ変更）、warning契約は選択A（`PlanWarningCode`と`ResultWarningCode`を分離）、`doctor.status`は選択A（`healthy\|degraded\|unhealthy`をseverityから集約）、`doctor`のunhealthyは選択A（成功JSONを返して専用exit 12）、`.NET published_at`は選択A（親`release-date`を子SDKへ継承するschema field追加）、Plan pathは選択A（独立した`reads[]`を追加）、公開pathは選択A（共通`PathValue` object）、`Diagnostic.code`は選択A（10診断項目の閉じた集合）、`channel_pointer`は選択A（string exact値とboolean `true→stable`/`false→prerelease`）とした。artifact sizeは既存の`size=0`＝unknown契約で.NETにも適用可能と確認した | 利用者判断を全て仕様へ反映後、全文機械検査、review report作成、本書の証跡/次task更新を行う | なし | `main` | Windows 10.0.26200 x64 / PowerShell 7.6.4 / Go 1.26.5 | Microsoft Learnの.NET環境変数・global tool文書、GitHub `dotnet/core`のlicense情報、Microsoft公式release metadata |
| 2026-08-08T00:30:00+09:00 | S00-03 全文書の整合監査完了 | .NET反映漏れ8件、文書間矛盾7件を修正。未定義enum 3件（`severity`/`checksum_source`/`path_role`）を`04`§17.1/§17.2で閉じた集合として定義 | P0-01でCI matrix workflowの骨格を作る | なし | `claude/go-dev-tool-version-manager-um7q20` | Linux container | `docs/04`§17.1/§17.2、本書§5 |
| 2026-08-07T23:59:00+09:00 | S00-02 .NET SDK追加のT01～T05完了 | 一次資料調査、provider選択、schema trace、判断6件確定、schema拡張5件（E1～E5）と`dotnet`定義契約を仕様へ反映 | P0-01でCI matrix workflowの骨格を作る。registry `dotnet.toml`実体はP10-04 | なし | `claude/go-dev-tool-version-manager-um7q20` | Linux container | `docs/06`§6.2/§16.4、`docs/07`§10、本書§5 |
| 2026-08-07T23:40:00+09:00 | S00-01 仕様再構成完了 | 19文書4,594行を16文書へ再構成。v0.1スコープを9 commandへ確定。CI matrixを唯一のgateへ変更。台帳を151→51タスクへ再編 | P0-01でCI matrix workflowの骨格を作る | なし | `claude/go-dev-tool-version-manager-um7q20` | Linux container | 本書と`docs/`全体のdiff |
| 2026-08-07T22:15:33+09:00 | 旧W00-01仕様再監査完了 | 旧章構成（README＋01〜18）で全仕様を監査しR01〜R32を同期 | 再構成により本台帳へ置換 | なし | `main` | Windows 10.0.26200 x64 / PowerShell 7.6.4 / Go 1.26.5 | `docs/reviews/W00-01-specification-audit.md` |

`docs/reviews/W00-01-specification-audit.md`は再構成前の章番号（01〜18）を参照する歴史記録である。現在の規範は[README.md](README.md)の文書一覧を正とする。

## 4. gate

- [x] **G-CI**: P0完了。CI matrixの全jobが最小構成でgreen
- [ ] **G-CORE**: P1～P5完了。基盤、schema、registry、IO層のcontractが両OSで合格
- [ ] **G-FLOW**: P6～P9完了。install/use/uninstall/doctorのCLI経路が両OSで合格
- [ ] **G-TOOLS**: P10完了。標準4 toolのcontractが合格し、tool固有Go分岐がない
- [ ] **G-E2E**: P11完了。両OSでE2E 15 scenarioとpackage/bootstrap jobがgreen
- [ ] **G-DONE**: P12完了。公開と公開後再検査が成功し、未確認項目が明記されている

WindowsとLinuxの間に順序gateを設けない。platform差が出る箇所は同じtaskの中で両OS分を実装し、CI matrixで同時に検証する。

G-TOOLS達成後は、G-E2E/G-DONEの完了を待たずにDF-01（§17）のドッグフーディングを開始し、P11以降と並行して不具合報告へ対応する。

## 5. S00 仕様再構成

- [x] **S00-01** 19規範文書をv0.1スコープへ再構成し16文書へ統合する。依存: なし。完了: 全相互linkが解決し、延期機能が[15-deferred.md](15-deferred.md)へ再導入gateと実装prompt付きで記録され、台帳が機能単位へ再編されている。証跡: 本書§3.3の該当record
- [x] **S00-02** 標準toolへ.NET SDKを追加するため[14-maintenance.md](14-maintenance.md)§3のT01〜T05を実施する。依存: S00-01。完了: 一次資料に基づくupstream調査とprovider選択が記録され、schema trace のgap 4件＋license表示1件に対する判断が確定し、schema拡張E1〜E5と`dotnet`のprovider/version/artifact/command/storage契約が仕様へ反映されている。T06〜T08（fixture・definition実体・実機検証）はP10-04以降で行う。証跡: 本書§3.3の該当record
- [x] **S00-03** 全16文書を横断監査し、.NET SDKの記載が他3 toolと同等であること、および文書間の矛盾・未定義enumが無いことを確認する。依存: S00-02。完了: tool列挙の反映漏れ、`--yes`承認件数、延期文書への章番号参照、廃止済み上限、`SelectionSummary`のenum衝突、未定義enum 3件を解消し、link/anchor・TOML/JSON例・件数の機械検査が全件成功している。証跡: 本書§3.3の最新record
- [x] **S00-04** W00-01/S00-03後の現行16文書を再レビューし、.NET SDKが他3 toolと同等に扱われていること、および構成変更後の文書内・文書間の矛盾や不整合がないことを確認・是正する。依存: S00-03。完了: 全文書の目視監査とlink/anchor・TOML/JSON例・用語/件数の機械検査を行い、発見事項、修正内容、検証結果をreview reportと本書§3.3へ記録している。証跡: `docs/reviews/S00-04-document-consistency-review.md`と本書§3.3の2026-08-08T17:41:10+09:00 record
- [x] **S00-05** branch/PR/version/tag/release workflowを仕様化し、既存のclient version/release契約と矛盾しないように修正する。依存: S00-04。完了: branch topology・同期・merge・protection・CI、CalVer rationale/上限/Go module非互換、main/tag/release手順が一意で、指示文書・品質仕様・公開文書仕様・進捗が同期し、link/用語/例の機械検査が成功している。証跡: [S00-05 review report](reviews/S00-05-branch-version-strategy-review.md)と本書§3.3の2026-08-08T18:56:21+09:00 record
- [x] **S00-06** 利用者のモチベーション（導入の容易さ、初心者の正確な再現、上流標準設定、最小security）と実装量・複雑度・CI E2E制約・ドッグフーディングの観点で全16文書を再レビューし、確定した簡素化を同期反映する。依存: S00-05。完了: e2eのfixture化とrelease時live smokeの実tool一巡、DF-01の新設、Plan契約の縮約、D-23〜D-26の延期記録、root README.mdのstatus表示が全対象文書へ同期され、利用者判断7件がreview reportへ記録されている。証跡: [S00-06 review report](reviews/S00-06-motivation-simplification-review.md)と本書§3.3の2026-08-09T02:33:50+09:00 record

## 6. P0 開発準備

- [x] **P0-00** [11-quality-and-ci.md](11-quality-and-ci.md)§5.6の一回限りの手順でrepositoryを再作成する。GitHub生成のREADME/MIT LICENSE/Go `.gitignore`を持つmain初期commitから`develop/work`を作り、現在の作業tree内容だけを登録し、そこから`claude/work`と`codex/work`を作る。旧`.git`/ref/historyを移行せず、同名3 fileの差分確認、初期branch protection、immutable tag ruleset、branch/commit/worktreeの証跡を記録する。依存: S00-05。実施は指定maintainer、検証と証跡記録はClaude Codeが担当した。branch protectionと`v*` tag rulesetはREST APIがこのsessionから読めないため利用者確認を証跡とし、値の照合はP0-01へ送った。証跡: [P0-00 verification report](reviews/P0-00-repository-bootstrap-verification.md)と本書§3.3の2026-08-09T03:11:40+09:00 record
- [x] **P0-01** CI matrix workflow（`lint`/`unit`/`e2e`/`policy`/`package`/`bootstrap`）を最小構成で作り、全jobを`ubuntu-latest`と`windows-latest`の両方でgreenにする。PRのsource/target/branch名policy検査も追加し、最初の成功check名を`main`、`develop/work`、両agent workのrequired status checkへ設定して[11-quality-and-ci.md](11-quality-and-ci.md)§5.3～§5.4のrulesetを完成する。依存: P0-00。完了: 6 job×2 OSの12 checkがgreenで、`policy` jobが§5.2の命名grammarと§5.3のsource→targetを拒否し、`lint` jobが文書link/anchor/fence/table列数を検査する。中身が未実装のjobは入力が現れた時点で失敗するguardを持つ。12 check名のrequired status check登録は指定maintainerが実施し利用者確認を得た。証跡: PR #5（commit `44fc277`）、CI run 12/12 success、本書§3.3の2026-08-09T03:47:50+09:00 record
- [x] **P0-02** Go module/toolchain、format/vet/lint/test/coverage command、証跡directory・命名・secret除去規則を固定する。依存: P0-01。完了: [11-quality-and-ci.md](11-quality-and-ci.md)§1.1〜§1.5がmodule path、`go`/`toolchain`、job別command、依存license許可list、証跡directory・命名、secret除去規則を一意に定め、`go.mod`/`go.sum`と`scripts/ci/check_licenses.py`と`.gitattributes`が実装され、両OSの12 checkがgreenである。証跡: [P0-02 決定記録](reviews/P0-02-toolchain-and-commands.md)と本書§3.3の2026-08-10T00:05:17+09:00 record
- [x] **P0-03** [02-architecture.md](02-architecture.md)§4.1の抽象portのうち、[11-quality-and-ci.md](11-quality-and-ci.md)§6が決定的検査の前提とするclock/HTTP/process/filesystem/link/user lookupの6件について、core側が所有するinterfaceを定義し、fakeとfailure injection基盤を作る。残る8 port（Registry/Archive/Hash/Lock/Environment/Random/ProgressSink/Logger）のinterfaceとfakeは、最初に必要とするtaskで追加する。依存: P0-02。完了: 6 interfaceと`Ports`、6 fakeと`Injector`／`Set`が`internal/domain/port`配下にあり、production pathからのfake importを`policy` jobが拒否し、両OSの12 checkがgreenである。証跡: [P0-03 決定記録](reviews/P0-03-port-interfaces-and-fakes.md)と本書§3.3の2026-08-10T01:40:31+09:00 record

## 7. P1 基盤

- [-] **P1-01** `cmd/gdtvm`と[02-architecture.md](02-architecture.md)§2のpackage骨格、package comment、依存方向のstatic checkを作る。依存: G-CI。証跡: 未記録
- [ ] **P1-02** ToolID/Version/Platform/Mode/Scope/Digest/Path/InstallKey/Selection等のdomain valueと3 version schemeを実装・testする。依存: P1-01。証跡: 未記録
- [ ] **P1-03** portの依存注入（`NewServices`とPorts組立て）と、package global mutable stateが存在しないことをtestする。interface定義とfakeはP0-03で6件を作成済みのため、ここでは行わない。依存: P1-01。証跡: 未記録
- [ ] **P1-04** typed error/message ID/exit code/secret masking/invocation・operation ID/cancel/progress/structured loggerを実装・testする。依存: P1-02,P1-03。証跡: 未記録

## 8. P2 config・path・state

- [ ] **P2-01** portable/user root決定（Windows Known Folder、Linux OS user lookup）、`--home`、config locatorを両OSで実装・testする。依存: P1。証跡: 未記録
- [ ] **P2-02** global/project TOML schema、default、unknown/type/limit、Git境界探索をstrict実装・testする。依存: P2-01。証跡: 未記録
- [ ] **P2-03** root layout/containment/owner/reparse/unsafe filesystemを実装・negative testする。依存: P2-01。証跡: 未記録
- [ ] **P2-04** state/setup/backup/selection/receipt/index/catalog/Plan/CLI JSONのcodecを[04-storage-and-data.md](04-storage-and-data.md)どおり実装・testする。依存: P1-02,P2-03。証跡: 未記録
- [ ] **P2-05** atomic write/flush/backup/revision/conflict/破損復旧とlock順、timeout/cancel、process間競合をfailure injection・parallel testする。依存: P2-04。証跡: 未記録

## 9. P3 definition schema

- [ ] **P3-01** schema 1の全field/conditional key/unknown/type/enum/limit validatorとJSON schemaを実装する。依存: P2-02。証跡: 未記録
- [ ] **P3-02** semver/go/python grammar、exact一致、comparison、channel/lifecycleを境界testする。依存: P3-01。証跡: 未記録
- [ ] **P3-03** `json`/`json-index`/`static` version source、index 2段取得と部分catalog禁止、`item_flatten_pointer`の1段展開、親公開日時の継承、`channel_pointer`のstring/boolean、`document_lifecycle_pointer`と`lifecycle_map`（未定義値のsource error）、pointer/token/asset、lifecycle override/evidence、artifact template/selector、checksum 2 kindとdigest algorithm（`sha256`/`sha512`）をfake upstream testする。依存: P3-01,P1-03。証跡: 未記録
- [ ] **P3-04** typed storage、install parameter（`strip_components` 0と1）、runtime command/env、probe（専用temp cwd）、`license_notice`を実装し、[06-tool-definition.md](06-tool-definition.md)§15〜§16の4 tool分をpositive fixture、全conditional違反をnegative fixtureにする。依存: P3-01～P3-03。証跡: 未記録

## 10. P4 registry

- [ ] **P4-01** [07-registry-and-tools.md](07-registry-and-tools.md)§2のexact tree、registry strict parser、file digest検証、command別load範囲を実装・testする。依存: P3-01。証跡: 未記録
- [ ] **P4-02** 4 definition/schema/message/licenseのsource validatorを作る。依存: P3,P4-01。証跡: 未記録

## 11. P5 HTTP・archive・process

- [ ] **P5-01** Go標準proxy/OS trust、HTTPS、timeout/retry/redirect/body上限/secret maskを実装・testする。依存: P1-03,P2-02。証跡: 未記録
- [ ] **P5-02** `.part` streamで内部/cache SHA-256とprovider指定SHA-256/SHA-512を1 pass計算し、progress、partial破棄、cache identity、offline判定を実装・testする。依存: P5-01。証跡: 未記録
- [ ] **P5-03** zip/tar.gzのentry事前検査（両OSのcollision/traversal/link/bomb規則）、same-volume staging、permission正規化、atomic rename、cleanupをfailure injection testする。依存: P1-03,P2-05。証跡: 未記録
- [ ] **P5-04** ProcessRunner（両OS）のabsolute target/argv/env/cwd/stdio/timeout/cancel/tree終了/output上限と、Plan外probe/write/download拒否および書込み範囲記録wrapperを実装・testする。依存: P5-01,P2-04。証跡: 未記録

## 12. P6 catalog・install・selection・runtime

- [ ] **P6-01** catalog refresh/cache/platform availability/exact/latest/not-found/EOLを実装・testする。依存: P3,P5-01。証跡: 未記録
- [ ] **P6-02** typed Plan、重要要約と全詳細、`inputs`/`PathValue`/`PlanArg`、operation排他の`SetupPlan`、download/extract/probe/storageと利用者可視`writes`、書込み封じ込め、任意helper process拒否、`PlanWarningCode`/approval category、stale判定を実装・testする。依存: P2-04,P6-01。証跡: 未記録
- [ ] **P6-03** Resolve→Plan→Approve→Execute→Commit、progress/cancel、中断後tmp cleanup、archive install/probe/receipt/index/idempotence、`install`非選択を実装・failure injection testする。依存: P5,P6-02。証跡: 未記録
- [ ] **P6-04** user/project use/current、Windows junction・Linux symlink transaction、native shim resolver/index/project precedence/env/fixed args/recursion/exitを実装・testする。依存: P6-03,P7-01,P7-02。証跡: 未記録
- [ ] **P6-05** uninstall/reference/trash/shared retain/purgeを実装・failure testする。依存: P6-04。証跡: 未記録

## 13. P7 platform・setup

- [ ] **P7-01** Windows NTFS junctionとLinux relative symlinkのcreate/verify/replace/unlink、root外/reparse/absolute targetのnegative testを実装する。依存: P2-03。証跡: 未記録
- [ ] **P7-02** shim実体（Windows hardlinkまたは内蔵resolver、Linux relative symlink）、argv0/module/root identity、console/signal/exit透過、long path、glibc判定とmusl/arm64 fail closedを実装・testする。依存: P1-03,P2-03。証跡: 未記録
- [ ] **P7-03** setupの`SetupPlan`（旧新root、filesystem/link、integration、backup）、transaction、冪等性、`--remove`を両OSで実装する。Windowsは`user-path|none`、Linuxは`shell-profile|none`。依存: P2,P7-02。証跡: 未記録
- [ ] **P7-04** Windows HKCU Pathのraw/type/24,576上限/backup/re-read/rollback/WM通知/remove ownershipと、Linux bash/zsh/fish profileのmarker/escape/conflict/backup/removeをtestする。依存: P7-03。証跡: 未記録

## 14. P8 Application Service・CLI

- [ ] **P8-01** Initializeと5 read operationを[02-architecture.md](02-architecture.md)§7のtyped request/resultで実装・testする。依存: P2～P7。証跡: 未記録
- [ ] **P8-02** Setup/Install/Use/UninstallのPlan/Execute mappingを実装する。依存: P6,P7。証跡: 未記録
- [ ] **P8-03** RuntimeResolverとProgressSink/CancelTokenのbackpressure/coalesce/commit境界、thread safety/lock順/request immutable/no global stateをrace testする。依存: P6-04,P8-02。証跡: 未記録
- [ ] **P8-04** CLI global option/位置/排他/case/alias/exact入力のtable testと、9 commandを薄いadapterとして全実装しService mapping testする。依存: P8-01～P8-03。証跡: 未記録
- [ ] **P8-05** human Planの重要要約→詳細→再要約、table/width/no-color、TTY progress bar、非TTY節目、読取り5 commandのJSON envelope、`ResultWarningCode`、`PathValue`、help、exit/child codeをgolden/schema testする。依存: P8-04。証跡: 未記録

## 15. P9 doctor・report

- [ ] **P9-01** [08-install-runtime.md](08-install-runtime.md)§13の10診断項目と`Diagnostic.code`、`healthy|degraded|unhealthy`集約、unhealthy時exit 12を実装・testする。依存: P8-01。証跡: 未記録
- [ ] **P9-02** `doctor --report`のMarkdown生成、[10-security.md](10-security.md)§9のmask規則、snapshot test、secretを含むfixtureでの漏れnegative testを実装する。依存: P9-01。証跡: 未記録

## 16. P10 標準4 tool

各toolはofficial fixture/static pin、strict definition、catalog新旧version、digest/archive、required command、storage、2 version切替、offline、uninstallを両OSで検証する。

- [ ] **P10-01** `go.toml`: 公式JSON/SHA-256、go scheme、channel/lifecycle根拠、go/gofmt、GOROOT/GOENV/GOPATH/cache、共有GOBIN、GOTOOLCHAIN=local。依存: G-CORE。証跡: 未記録
- [ ] **P10-02** `node.toml`: 公式index/SHASUMS、channel/lifecycle根拠、node/npm/npx固定target、tool共有config/cache、version別global package、Linuxは`.tar.gz`。依存: G-CORE。証跡: 未記録
- [ ] **P10-03** `python.toml`: Astral static build/digest/license/channel/lifecycle/third-party Plan、python/pip、stdlib/venv、version別user package、両platformのversion集合一致。依存: G-CORE。証跡: 未記録
- [ ] **P10-04** `dotnet.toml`: `json-index`の2段取得、`item_flatten_pointer`によるfeature band展開と親`release-date`継承、`support-phase`の`lifecycle_map`写像、SHA-512 digest、Windows `license_notice`と`W_RESTRICTIVE_LICENSE`承認、`strip_components=0`、`dotnet --version`/`--list-sdks` probe、version別`DOTNET_CLI_HOME`とtool共有NuGet cache。依存: G-CORE。証跡: 未記録
- [ ] **P10-05** registry exact 4 tool/schema/message/licenseのcontract testと、tool固有Go分岐不存在をsynthetic definitionで検証する。依存: P10-01～P10-04。証跡: 未記録
- [ ] **P10-06** `dotnet tool install -g`の実配置先を両OSで測定し、`DOTNET_CLI_HOME`で管理領域へ隔離できるかを確定する。隔離できる場合はstorage宣言とcontract testへ反映し、できない場合は管理外として[12-public-docs.md](12-public-docs.md)の必須記載項目へ確定する。推測でstorageを宣言しない。依存: P10-04。証跡: 未記録

## 17. DF ドッグフーディング開始

- [ ] **DF-01** G-TOOLS達成後、CI `package` jobのdevel build成果物（両OS archive）を利用者へ提供し、ドッグフーディングを開始する。利用者は[11-quality-and-ci.md](11-quality-and-ci.md)§9のチェックリストと実作業で使用し、不具合は`gdtvm doctor --report`と[14-maintenance.md](14-maintenance.md)§5のテンプレートで報告する。報告は[14-maintenance.md](14-maintenance.md)§5.2の初動とB01で分類し、必要なtaskを本書へ追加する。devel buildはCI artifactとしてだけ提供し、GitHub Releaseを作らない。依存: G-TOOLS。証跡: 未記録

## 18. P11 E2E・package・bootstrap

- [ ] **P11-01** E2E基盤を両OSで作る。書込み範囲記録wrapper（封じ込め検査）、ローカルHTTPS疑似upstream・合成archive・擬似tool binaryのfixture基盤、clean rootの生成と破棄を含む。networkへ依存しない。依存: G-TOOLS。証跡: 未記録
- [ ] **P11-02** [11-quality-and-ci.md](11-quality-and-ci.md)§8のscenario 1～8（setup冪等、4 toolの一巡、install非選択、not-found案内、latest/prerelease/EOL/third-party/制限的license Plan、storage分離、project優先）を自動化する。依存: P11-01。証跡: 未記録
- [ ] **P11-03** scenario 9～15（offline/cache/cancel/lock、failure injection、悪性fixture、PATH integration、`setup --remove`、doctor/report、書込み範囲検査）を自動化する。依存: P11-02。証跡: 未記録
- [ ] **P11-04** `package` jobで2 archiveを生成し構造/permission/binary versionを検査し、`bootstrap` jobでfixture releaseに対する`install.ps1`/`install.sh`のpositive/negative、同版no-op、異版のbackupなし置換、任意`gdtvm.toml`のbyte完全引継ぎを検査する。依存: P11-01。証跡: 未記録

## 19. P12 文書・release

- [ ] **P12-01** root README/USER_GUIDEを[12-public-docs.md](12-public-docs.md)どおり作り、実装済みstatus/9 command/4 tool/storage/security/更新方法を同期する。依存: G-E2E。証跡: 未記録
- [ ] **P12-02** link/anchor/example/help一致/schema/公開文書、secret/個人pathを検査する。依存: P12-01。証跡: 未記録
- [ ] **P12-03** 5 assetを公開して全件を再download/checksum/archive/script bytes検証し、[11-quality-and-ci.md](11-quality-and-ci.md)§9の利用者確認チェックリストと未確認項目をrelease noteへ記載する。公開assetを上書きしない。依存: P12-02。証跡: 未記録
- [ ] **P12-04** G-DONEと最終snapshotを判定する。依存: P12-01～P12-03。証跡: 未記録

## 20. 章別coverage

- [ ] **COV-01** `01-requirements`: P0,P8,P11,P12
- [ ] **COV-02** `02-architecture`: P1,P8
- [ ] **COV-03** `03-cli`: P8,P11
- [ ] **COV-04** `04-storage-and-data`: P2,P6,P7
- [ ] **COV-05** `05-configuration`: P2,P10
- [ ] **COV-06** `06-tool-definition`: P3,P10
- [ ] **COV-07** `07-registry-and-tools`: P4,P10,P12
- [ ] **COV-08** `08-install-runtime`: P5,P6,P9,P11
- [ ] **COV-09** `09-platform`: P7
- [ ] **COV-10** `10-security`: P4～P7,P9,P11
- [ ] **COV-11** `11-quality-and-ci`: P0,P11,P12
- [ ] **COV-12** `12-public-docs`: P12-01,P12-02
- [ ] **COV-13** `13-progress`: 全停止/再開/gate
- [ ] **COV-14** `14-maintenance`: P0-02と将来追加/不具合作業
- [ ] **COV-15** `15-deferred`: 延期機能を実装する場合の入口

## 21. 最終未完了一覧

必須項目を時間不足で延期してG-DONEにしない。[11-quality-and-ci.md](11-quality-and-ci.md)§9の利用者確認は必須項目ではないが、実施状況を必ず記載する。

| Task | 理由 | 利用者影響 | 回避策 | 承認者 | 期限/追跡 |
|---|---|---|---|---|---|
| なし | なし | なし | なし | 未設定 | なし |
