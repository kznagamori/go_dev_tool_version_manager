# S00-05 branch・version戦略レビュー

## 1. 対象と結論

現行repositoryの規範文書、作業指示、Git branch/ref、client version、tag、release手順を横断確認した。利用者判断を経て、branch topology、同期・再作成、merge方式、protection、段階別CI、CalVer、tag起動releaseを[11-quality-and-ci.md](../11-quality-and-ci.md)へ一意に定義し、仕様索引、公開文書仕様、Codex/Claude Code指示、進捗台帳へ同期した。

現在のremote branchを移行する案は採用しない。GitHub生成のREADME、MIT LICENSE、Go `.gitignore`を持つ新しいmain初期commitから`develop/work`を作り、現在の作業treeのfile内容だけを登録する。旧repositoryの`.git`、commit、branch、tag、refは移行しない。実際のrepository再作成はP0-00で指定maintainerが行う。

## 2. 監査時の事実

- 監査時branchは`main=f48df04`。`origin/claude/work=76b4043`と旧Claude feature branchはmainの祖先で、remote固有commitはなかった。
- `develop/work`、`codex/work`、tag、root `VERSION`、GitHub Actions workflowは存在しなかった。
- 提示された`develop/work`、`claude/work`、`codex/work`、agent feature名はGit refとして表現可能だった。
- 既存仕様には`YYYY.mm.DD.XX`の概略はあったが、`v0.1`との関係、tag日時、同日上限、失敗番号の再利用、Go module非互換、tag作成主体、release中のdevelop更新が未定義だった。
- 二段階PRに対するmerge方式、必須CI、同期、作業中判定、branch protection例外も未定義だった。

## 3. 発見事項と修正

| ID | 発見事項 | 影響 | 確定した修正 |
|---|---|---|---|
| B01 | 長期agent workを毎回squash/rebase mergeすると、既取込みcommitの再提示や履歴不一致が起きる | 次回PRの差分・競合が不安定 | feature→agent workだけsquashし、agent work→develop→mainはmerge commit |
| B02 | develop更新をagent workへ戻す規則がない | 古いbase、遅い競合検出 | 作業中はrebase、非作業中は削除してdevelopから再作成 |
| B03 | 「作業中」の判定が主観的 | open PR/featureを破壊する可能性 | 未mergefeature、関連open PR、develop未反映commitの論理和で機械判定 |
| B04 | protectionのforce/delete禁止とrebase・再作成が衝突 | 指定運用を実行不能 | mainは例外なし。develop/agent workだけ指定maintainerがlifecycle操作でbypassし、rebase反映は`--force-with-lease` |
| B05 | 二段階PRのCI範囲がない | 重複実行または検査不足 | feature段階は両OSの`lint/unit/policy`、develop以降は両OSの全6 job |
| B06 | release PR中もdevelopが動く余地がある | release対象commitとVERSIONが変動 | release PR作成から公開完了までagent work→developを凍結。feature→agent workは継続可 |
| V01 | `v0.1`とCalVerの関係が曖昧 | tag名・CLI versionの誤解 | `v0.1`は機能scope名、実version/tagは`YYYY.MM.DD.XX`/`vYYYY.MM.DD.XX` |
| V02 | CalVerをSemVerのように扱う余地がある | Go module導入が失敗 | CalVerは非SemVer。正式配布をGitHub Releasesのarchive/bootstrapに限定し、CalVer指定`go install`は非対応 |
| V03 | release日の確定点と同日上限がない | VERSION/tag不一致、通番競合 | annotated tagger timestampのJST日付、最大通番＋1、`00`～`99`、失敗tag再利用禁止 |
| V04 | tagとrelease CIの責任境界がない | 二重tag、再実行時の状態不明 | main CI後に指定maintainerがannotated tagをpushし、tag workflowが検査・build・公開 |
| R01 | 旧repositoryが不整合なbranch状態 | 初期移行規則が複雑化 | 旧historyを捨て、新main初期commit→developへ現在fileを登録する一回限りの手順に変更 |

## 4. 利用者判断

| 項目 | 確定内容 |
|---|---|
| topology | `feature→agent work→develop/work→main` |
| merge | featureはsquash＋削除、長期branch間はmerge commit |
| protection | main/develop/両agent workを保護。PR、最新base、CI、conversation解決必須、linear historyなし |
| 同期 | 作業中ならrebase、非作業中なら削除・再作成 |
| 作業中判定 | 未mergefeature、関連open PR、develop未反映commitのいずれか |
| lifecycle例外 | 指定maintainerだけ。mainは例外なし、rebaseは`--force-with-lease` |
| feature名 | `<agent>/feature-<task-id>-<slug>`、slugは小文字ASCII kebab-case 1～48文字 |
| version | CalVer維持。`v0.1`はscope名、CalVer `go install`は非対応 |
| 日付・通番 | annotated tagのJST日付、同日最大＋1、`00`～`99`、番号再利用なし |
| release起動 | maintainerのannotated tag pushでCI起動 |
| repository再作成 | mainをGitHub生成README/MIT LICENSE/Go `.gitignore`で初期化し、developへ現在fileを登録 |
| CI | featureは`lint/unit/policy`、develop/mainは全6 job。すべてWindows/Linux |
| release後混在 | develop→作業中agent work→featureの順にrebase、非作業中agent workは再作成 |
| approval | required approval 0件。指定maintainer merge |
| release freeze | release PR作成から公開完了までagent work→developを停止 |

## 5. version方式の評価

CalVerは、registry更新もclient releaseになる本製品で「番号の大きさ」より「いつのreleaseか」を優先する目的に合う。一方、4個の数値要素と各要素のleading zeroを持つ`vYYYY.MM.DD.XX`はSemVerではない。Go moduleのrelease tagはcanonical semantic versionである必要があるため、clientの正式導入をGitHub Release assetへ限定した。

将来Go moduleとして公開導入する要件が生じた場合は、CalVer tagへSemVer tagを追加するだけでは正本が二重になる。CLI表示、module path、tag対応表、release automation、互換性方針を別taskで先に仕様化する。

## 6. 参照した一次資料

- [Git `check-ref-format`](https://git-scm.com/docs/git-check-ref-format.html)
- [GitHub pull request merge方式](https://docs.github.com/en/pull-requests/reference/pull-request-merges)
- [GitHub pull request mergeの挙動](https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/incorporating-changes-from-a-pull-request/about-pull-request-merges?apiVersion=2022-11-28)
- [GitHub rulesetで利用できる規則](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/available-rules-for-rulesets)
- [Go Modules Reference](https://go.dev/ref/mod)
- [Semantic Versioning 2.0.0](https://semver.org/)

## 7. 検証

| 検証 | 結果 |
|---|---|
| `git diff --check` | PASS |
| Markdown相対link/anchor | link 181件、anchor 22件、failure 0 |
| linked `§`参照 | 58件、failure 0 |
| code fence balance | marker 156件、failure 0 |
| Git ref・feature命名 | Git ref 5件、feature正例2件・負例4件、failure 0 |
| CalVer positive/negative | 正例2件・負例5件、failure 0 |
| branch/CI/version同期contract | CI job 6件、許可PR経路5件、同期検査10件、stale CalVer表記0件 |

## 8. 未実施・次作業

- remote repositoryの再作成、branch作成・削除、ruleset設定、tag作成は実施していない。P0-00で行う。
- workflowが未実装のためrequired checkはまだ設定できない。P0-01の初回成功後、最初のmerge前に実在check名をrulesetへ追加する。
- root `VERSION`、release workflow、binary version injectionは未実装であり、P0/P11/P12の該当taskで実装・検証する。
- 本taskは文書仕様のレビューであり、GitHub上の権限・ruleset動作確認はrepository再作成後に行う。
- Go moduleとsourceはまだ存在しないため、`go test`は本taskでは実行していない。
