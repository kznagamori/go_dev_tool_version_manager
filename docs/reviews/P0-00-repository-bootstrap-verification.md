# P0-00 repository初期登録の検証

## 1. 目的と対象

[11-quality-and-ci.md](../11-quality-and-ci.md)§5.6の一回限りの初期登録が、指定maintainerの手で完了しているかを検証した記録である。repository再作成とremote branch操作は指定maintainerが行うため（同§5.6末尾）、Claude Codeは実施ではなく検証と証跡記録だけを担当した。

対象repositoryは`kznagamori/go_dev_tool_version_manager`。検証時点のremote refは`main` `2cb8bc5`、`develop/work`／`claude/work`／`codex/work`がいずれも`9e69261`。検証環境はLinux container、Go 1.24.7 linux/amd64、認証identityは`kznagamori`。

本書は監査証跡であり規範仕様ではない。実装時は[docs/README.md](../README.md)が指定する番号付き仕様を正とする。

## 2. 結論

- §5.6手順1〜4のうち、**file内容とbranch topologyに関する条件はすべて機械的に確認できた**。旧repositoryのhistoryはremoteのどのrefからも到達不能である。
- **branch protectionとtag rulesetは機械的に確認できなかった**。このセッションからのGitHub REST API直接呼出しはproxyが`403`で遮断し、GitHub MCPにruleset/protection取得toolが無い。利用者へ1問確認し、「設定済み」との回答を得たため、この2項目は利用者確認を証跡とする。
- 上記の制約により、P0-00は「機械検証＋利用者確認」の組合せで完了と判定した。protection設定値そのものの機械照合はP0-01でrequired status checkを追加する際に、最初のPRが実際にgateされるかで間接的に確認できる。

## 3. §5.6 手順ごとの検証結果

| 手順 | 条件 | 結果 | 根拠 |
|---|---|---|---|
| 1 | GitHub生成のREADME・MIT LICENSE・Go `.gitignore`を持つ`main`初期commit | 一致 | `main`初期commit `2cb8bc5`のtreeは`.gitignore`、`LICENSE`、`README.md`の3 fileのみ。author `kznagamori`、message `Initial commit`、日時`2026-08-09T00:48:15+09:00` |
| 1 | LICENSEの著作権者・年が現在fileと一致 | 一致 | `main`版と`develop/work`版が完全一致。MIT、`Copyright (c) 2026 kznagamori` |
| 2 | `main`初期commitから`develop/work`を作成 | 一致 | `develop/work`のhistoryはroot `2cb8bc5`から連続。`2cb8bc5`→`a9278f2`（レポジトリ再構築）以降 |
| 3 | 現在の作業tree内容だけをdevelopへ登録 | 一致 | `develop/work`のtree topは`.gitignore`、`AGENTS.md`、`CLAUDE.md`、`LICENSE`、`README.md`、`docs` |
| 3 | 同名3 fileを生成版との差分確認のうえ現在版へ更新 | 一致 | `LICENSE`は差分なし（生成版＝現在版）。`.gitignore`はGo templateへ`.cache/`と`artifacts/`の2行追加。`README.md`は生成版から10行追加・1行削除 |
| 4 | developから`claude/work`と`codex/work`を作成 | 一致 | 両branchとも`9e69261`を指し、`develop/work`と同一commit |
| 4 | §5.4のbranch protection（PR必須、direct/force-push・削除禁止、linear history無効、bypass actor） | **利用者確認による** | 機械検証は不可（§4）。利用者回答「設定済み」 |
| 4 | `v*`のimmutable tag ruleset | **利用者確認による** | 機械検証は不可（§4）。利用者回答「設定済み」。remote tagは0件 |
| — | 旧`.git`・ref・historyを移行しない | 一致 | remoteのroot commitは`2cb8bc5`のみ。旧root `fb754ed`系列はremoteのどのrefからも到達不能 |
| 5 | required status checkの登録 | 対象外 | §5.6手順5はP0-01の範囲。CI workflowが存在しないため未実施が正しい |

### 3.1 実行したcommandと確認点

| command | 確認点 |
|---|---|
| `git ls-remote --heads origin` | remote branchが`main`、`develop/work`、`claude/work`、`codex/work`の4本（＋作業branch）であること |
| `git ls-remote --tags origin` | tagが0件であること |
| `git ls-tree --name-only origin/main` | 初期commitのtreeが生成3 fileだけであること |
| `git log origin/main --format='%H%n%an <%ae>%n%cI%n%s' -1` | 初期commitのauthor・日時・messageの確認 |
| `git diff origin/main:LICENSE origin/develop/work:LICENSE` | license内容の同一性（差分なし） |
| `git diff origin/main:.gitignore origin/develop/work:.gitignore` | Go templateからの追加行の特定 |
| `git diff --stat origin/main:README.md origin/develop/work:README.md` | 現在版への更新の有無 |
| `git rev-list --max-parents=0 --all` | root commitの列挙による旧history混入の検出 |
| `git push -u origin <branch>` | 作業branchへのpush権限（成功） |

### 3.2 localの旧history残存について

このcontainerのclone内には、旧repository由来のlocal専用branch `claude/work`（`76b4043`、root `fb754ed`）が残っている。remoteには存在せず、§5.6の「旧historyを移行しない」条件には違反しない。ただし`claude/work`という名前がlocalとremoteで別commitを指すため、feature branchは`origin/claude/work`を明示的にbaseにして作成した。

## 4. 機械検証できなかった項目と理由

`GITHUB_TOKEN`は環境に存在するが、このセッションからの`https://api.github.com`直接呼出しはagent proxyが遮断する。

```text
GET /repos/kznagamori/go_dev_tool_version_manager/rulesets
-> 403 {"message":"GitHub access is not enabled for this session.
         An org admin must connect the Claude GitHub App for this organization."}
GET /repos/kznagamori/go_dev_tool_version_manager
-> 403（同上）
```

GitHub MCP serverにはruleset・branch protection・repository merge設定を取得するtoolが無い。代替として`list_branches`を実行したところ、`main`を含む全branchが`"protected": false`を返した。GitHubのbranch一覧APIの`protected`はrulesetによる保護を反映しない場合があるため、この値を未設定の根拠とは扱わない。

再現command（GitHub App接続済みのsessionまたは`gh`利用可能な環境で実行する）:

```bash
gh api repos/kznagamori/go_dev_tool_version_manager/rulesets
gh api repos/kznagamori/go_dev_tool_version_manager/branches/main/protection
gh api repos/kznagamori/go_dev_tool_version_manager --jq '{allow_squash_merge,allow_merge_commit,allow_rebase_merge}'
```

§5.3の「squash mergeとmerge commitを許可し、rebase mergeを無効にする」repository設定も同じ理由で未確認である。P0-01でCI workflowを追加し最初のPRを作成する際に、merge方式の選択肢とrequired status checkの挙動から確認する。

## 5. 次タスクへの申し送り

- P0-01でCI matrix workflowを作成し、最初の成功check名を4 protected branchのrequired status checkへ登録する（§5.6手順5）。その時点でprotectionの実効性が観測できるため、結果を本書§4の未確認項目へ反映する。
- 検証環境のGoは1.24.7で、§1のminimum toolchain（Go 1.26系）を満たさない。CIは`actions/setup-go`で1.26系へ固定するため影響しないが、containerでのlocal build・testはP0-02でtoolchain取得方法を決めるまで実施できない。
