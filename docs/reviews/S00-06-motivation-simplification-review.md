# S00-06 モチベーション・簡素化レビュー

## 1. 目的と対象

利用者が提示した4つのモチベーション（WindowsとLinuxで簡単に導入できること、初心者が完全versionを間違えず再現できること、中級者が上流標準設定を使えること、企業向けsecurityは最低限にすること）と、実装量・複雑度・CI E2E制約・ドッグフーディング容易性・不具合調査手順の観点で、S00-05後の現行16文書を再レビューし、確定した簡素化を反映した記録である。

対象は[docs/README.md](../README.md)と[01](../01-requirements.md)〜[15](../15-deferred.md)、root `README.md`、`AGENTS.md`/`CLAUDE.md`、reviews 3件。baselineはcommit `a9278f2`、開始時worktreeはclean。環境はLinux container。

本書は監査証跡であり規範仕様ではない。実装時は[docs/README.md](../README.md)が指定する番号付き仕様を正とする。

## 2. 結論

- 4つのモチベーションは[docs/README.md](../README.md)§1の製品優先順位と一致しており、逸脱はない。
- 不具合調査手順と修正promptは[14-maintenance.md](../14-maintenance.md)（B01〜B05、§5報告テンプレート、§10〜§12 prompt）に整備済みで、追加は不要と確認した。
- CI E2Eは「PR testはfixture正」（[07](../07-registry-and-tools.md)§12）と「e2eはnetwork有効」（[11](../11-quality-and-ci.md)§5）のねじれがあり、利用者判断でe2e jobを全面fixture化した。実upstream・実toolの検証はrelease時live smoke（実artifactの4 tool一巡を追加）と利用者確認チェックリストへ移した。
- 実装量・複雑度の残る最大要因だったPlanの読書き完全列挙を縮約し、download再開・`available` filter・並行download設定を延期した（D-23〜D-26）。
- 「まずユーザが使用できる状態にする」ため、G-TOOLS達成後にdevel buildでドッグフーディングを開始するDF-01を新設した。

## 3. モチベーションとの整合

| モチベーション | 現行仕様の対応 | 評価 |
|---|---|---|
| 1. Windows/Linuxで簡単に導入 | 9 command、`setup`のPATH統合、bootstrap script、両OS同時開発、CI matrix | 一致。user mode/bootstrapは維持（利用者判断） |
| 2. 初心者が完全versionを正確に再現 | catalog正規文字列のbyte完全一致、部分版/range/近似の拒否、明示`--latest`、Plan要約、標準4操作 | 一致 |
| 3. 中級者は上流標準設定 | `go env -w`/`npm config`/`pip config`/`dotnet nuget`等をそのまま使い、gdtvmは保存先をtyped storageへredirectするだけ | 一致 |
| 4. 企業向けsecurityは最低限 | 署名/SBOM/audit log/中央承認なし。個人利用でも実害の出る境界（checksum、封じ込め、archive検査、mask）だけ必須 | 概ね一致。過剰だったPlan完全列挙を今回縮約 |

## 4. 利用者判断

7件を1問ずつ確認した。

| # | 項目 | 採用 | 主な理由 | 他の選択肢 |
|---|---|---|---|---|
| 1 | E2E方式 | **A: e2e jobを全面fixture化**。実upstream/実toolはrelease時live smokeと[11](../11-quality-and-ci.md)§9利用者チェックへ | CIで確実に完走・決定的。「CIで可能なものだけ」を満たす | B: Linuxのみlive（両OS原則が崩れる）、C: 現状維持（flaky） |
| 2 | 使用可能時点 | **A: G-TOOLS後にDF-01でドッグフーディング開始** | E2E完成を待たずに利用開始し、修正循環を早く回す | B: E2E前にrelease（品質リスク）、C: P12まで配布なし |
| 3 | Plan契約 | **A: 縮約**。`reads[]`廃止（`inputs`へ`registry_sha256`を追加して一本化）、`writes[]`は利用者可視変更のみ、`rollback[]`廃止、CI §7.2は封じ込め検査 | 最重量の機械契約を削減。承認に必要な表示情報は維持 | B: 完全列挙維持、C: 表示専用まで縮約 |
| 4 | 読取り`--json` | **A: 維持** | script/CI利用者（[01](../01-requirements.md)§1）とE2E照合に有用。削減効果が小さい | B: doctor/versionのみ、C: 全延期 |
| 5 | message catalog | **B: `ja.toml`同梱を維持** | 文言とcodeの分離、多言語化への距離を優先 | A: Go埋込み化 |
| 6 | user mode/bootstrap | **A: 維持** | モチベーション1の中核導線。DF-01採用で開始時期を遅らせない | B: portableのみ、C: bootstrapだけ延期 |
| 7 | 小規模簡素化 | **a,c,d採用**: download再開延期（D-24）、`available` filter延期（D-25)、`download.concurrency`削除（D-26）。b（`--offline`延期）は不採用 | 実装・test表面積の削減。offlineは対象利用者要件のため維持 | — |

## 5. 反映した変更

| 文書 | 変更 |
|---|---|
| [docs/README.md](../README.md) | §2へDF-01の開始方針、§5.6のPlan詳細表現、§6完了判定2をfixture E2E＋live smokeへ |
| [02-architecture.md](../02-architecture.md) | HTTPClientからrange再開を削除、`ListAvailable` request縮小、Execute検査を`inputs`＋封じ込めへ、download逐次化 |
| [03-cli.md](../03-cli.md) | `available`のfilter option削除（D-25） |
| [04-storage-and-data.md](../04-storage-and-data.md) | partial破棄（D-24）、Plan構造例から`reads[]`/`rollback[]`削除、`inputs`へ`registry_sha256`追加、`writes[]`を利用者可視変更へ限定、enum表からrollback行と`hardlink` action削除、path role目的を封じ込め判定へ |
| [05-configuration.md](../05-configuration.md) | `[download]`から`concurrency`/`resume`を削除（D-24/D-26） |
| [07-registry-and-tools.md](../07-registry-and-tools.md) | §12をPR CI＝fixture、release live smoke＝実artifactの4 tool一巡へ拡張 |
| [08-install-runtime.md](../08-install-runtime.md) | 逐次download、Plan詳細表示の変更、Range再開削除 |
| [10-security.md](../10-security.md) | cache再利用をcomplete fileのみへ、fail closed対象を封じ込め表現へ |
| [11-quality-and-ci.md](../11-quality-and-ci.md) | e2e jobのfixture化、live接続をrelease workflowへ限定、§5.1へDF開始、§6/§7.2のPlan検査変更、scenario 9からRange再開削除、§14完了条件 |
| [13-progress.md](../13-progress.md) | snapshot更新、S00-06登録、§17 DF-01新設と後続section再番号、P5-02/P6-02/P11-01/P11-03の記述同期 |
| [15-deferred.md](../15-deferred.md) | D-23（Plan完全列挙）、D-24（download再開）、D-25（available filter）、D-26（並行download）を追加 |
| root `README.md` | [12-public-docs.md](../12-public-docs.md)§3.1に従い「仕様策定中・利用不可」statusを明示 |

変更しないと確認した項目: 読取り5 commandの`--json`、message catalog `ja.toml`、user mode/bootstrap、`--offline`、branch/PR/CalVer workflow（S00-05確定事項）、部分version拒否と近似候補なし（README §5.1確定事項）、doctor/report/maintenance手順。

## 6. 検証

| 検証 | 結果 |
|---|---|
| `git diff --check` | PASS |
| Markdown相対link/anchor解決 | 全件成功（検証scriptで確認） |
| code fence balance | 全文書で偶数 |
| stale用語検索（`reads[]`、`rollback[]`、Range再開、`resume`、`concurrency`、`--channel`/`--lifecycle`、network有効e2e） | 規範文書で残存0件（延期記録D-23〜D-26と歴史記録を除く） |
| enum/件数整合（`writes[].action` 6値、Plan warning 8、Result warning 5、diagnostic 10、path role 22、SetupPlan field 15、exit 13値/34 error code） | 期待値一致 |

文書のみの変更であり、Go source、registry TOML実体、CI workflowは存在しないため`go test`とCI matrixは対象外。P0以降で実施する。

## 7. 未実施・次作業

- P0-00: repository branch topology（`develop/work`、`codex/work`、protection、tag ruleset）の整備。remote確認時点で`claude/work`だけが存在する。
- 本セッションはClaude Code Web指定branch（`claude/go-dev-tool-version-manager-7zyakj`）で作業した。`claude/feature-<task-id>-<slug>`規約はP0-00のbranch整備後から適用する。
- fixture基盤（疑似upstream・合成archive・擬似tool binary）の具体設計はP11-01で行う。
