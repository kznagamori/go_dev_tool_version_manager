# P0-03 port interfaceとfakeの決定記録

## 1. 目的と対象

[13-progress.md](../13-progress.md) P0-03の実施記録である。着手時に台帳のtask依存へ循環を見つけたため、まず依存を是正し、そのうえで6 portのinterface、fake、failure injection基盤を実装した。

規範は[02-architecture.md](../02-architecture.md)§1・§4・§4.1と[11-quality-and-ci.md](../11-quality-and-ci.md)§6・§7.1・§8とする。本書は判断と実測の証跡であり規範仕様ではない。

環境はLinux container（Go 1.26.5、Python 3.11.15）とCI matrix（`ubuntu-latest`／`windows-latest`）。

## 2. 着手前に見つけた依存循環

| # | 矛盾 | 影響 |
|---|---|---|
| 1 | P0-03「fake portを作る」に対し、対象interfaceの定義はP1-03「全portのinterface定義とfake」 | fakeは対象interfaceが無いと書けないため、記載順のままでは着手不能 |
| 2 | 置き場所のpackage骨格はP1-01（依存: G-CI＝P0完了） | P0-03時点でGo packageが0件 |
| 3 | 「fake」がP0-03とP1-03の両方にある | 責務境界が一意でない |
| 4 | §4.1は14 port、§6とP0-03は6 portだけを名指し | 残る8 portの担当taskが未定 |

[README.md](../README.md)が指定する他の番号付き仕様にも、この責務分担を解決する記述は無かった。`CLAUDE.md`§2に従い実装を開始せず、利用者へ確認した。

## 3. 利用者判断

| # | 項目 | 採用 | 主な理由 | 他の選択肢 |
|---|---|---|---|---|
| 1 | 循環の解消 | **A: P0-03をinterface定義まで拡張し、P1-03を依存注入とglobal mutable state不存在のtestへ縮小** | 循環が消え、interfaceとfakeが同一taskになり責務が一意になる。P0/P1のphase構成と§4のG-CI定義を変えずに済む | B: P0-03をP1-03へ統合しG-CIをP0-02到達へ変更、C: P0-03をinterface非依存のtest基盤だけに限定 |
| 2 | port範囲 | **6 port先行** | §6が決定的検査の前提として名指しする6件に絞る。未使用のfakeを「将来のため」に先行導入しない（`CLAUDE.md`§7） | 14 portを一括 |

台帳のP0-03とP1-03の2行を同じ変更で修正した。

## 4. 配置の判断

port interfaceは`internal/domain/port`、fakeは`internal/domain/port/fake`へ置いた。

§1が「抽象ポートはcore側が所有し、Infrastructureはそれを実装する」と定めるため、core側でなければならない。加えてGoのimport cycleを避ける必要がある。

| 候補 | 判定 |
|---|---|
| `internal/app` | 不可。`internal/install`等の内側packageがinterfaceのためにappをimportし、appがそれらをimportし返してcycleになる |
| `internal/domain`直下 | 可能だが、§2が`internal/domain`の責務をToolID/Version/Platform/Plan/Selection/Receipt/Errorと定めており、port抽象と混ざる |
| `internal/domain/port` | 採用。core側のleafでcycleを生まない。§2の表の見出しが「論理領域」であり、`internal/domain`配下のsub-packageは同じ領域に属すると読んだ |

§2の表に`internal/domain/port`という行は無い。この読み方が意図と異なる場合は移動が必要になるため、PR本文で明示して確認を求めた。

## 5. 設計判断

- **Clockはwall clockと単調時間を別型にした**。`Monotonic`を独立した型にすることで、記録用時刻と経過時間計測の取り違えをcompile時に防ぐ。NTP補正やsummer timeでwall clockが巻き戻ってもtimeout判定が負にならないことをtestで確認する。
- **FileSystemの書込みは`AtomicWrite`だけにした**。部分書込みを観測できるAPIを置かないことで、[10-security.md](../10-security.md)が要求する「中断しても壊れた状態を残さない」を型で強制する。
- **`LinkManager.RemoveLink`はlinkだけを外す**。link先の実体削除はFileSystem側の操作と分け、`current`の張り替えでtool本体を消す事故を構造的に防ぐ。
- **ProcessRunnerとHTTPClientは未登録の対象をerrorにする**。仕様が禁止する任意helper processの起動や未想定hostへのアクセスを、stubの書き忘れごと検出する。fakeが黙って成功を返すと、禁止事項を破ったtestが通ってしまう。
- **Injectorは操作名ごとにskip/times付きで失敗を積む**。「3回目のwriteでdisk full」を実disk状態に依存せず再現できる。`Pending()`で未消化の注入を検出し、意図した経路が実行されないまま通るtestを防ぐ。
- **6 portが1つのInjectorを共有する**。download失敗の後にstagingのcleanupが走ったか、といったport横断の順序を単一の記録で検査できる。

## 6. `policy` jobへの追加

production pathからfake packageをimportすることを禁止し、[11-quality-and-ci.md](../11-quality-and-ci.md)§7.1へ規範化した。決定的testのための細工がruntime経路へ載ることを静的に拒否する。fake package自身のfileと`_test.go`は対象外とする。

## 7. 検証結果

| 検証 | 結果 |
|---|---|
| `gofmt -l .` | 差分なし |
| `go vet ./...` | 成功 |
| `go test ./... -race -shuffle=on` | 成功。coverage **84.5%**（fake package 85.1%） |
| test件数 | 40件 |
| 禁止import検査 negative | production importを拒否。fake package自身と`_test.go`は除外 |
| 全13 run stepを`bash -eo pipefail`で実行 | `govulncheck`以外すべて成功 |
| **CI（両OS 12 check）** | **全て success** |
| CI `govulncheck` | `No vulnerabilities found.`（`lint (ubuntu-latest)`のlog） |
| CI 依存license検査 | 13 module判定（Apache-2.0 2件／BSD-3-Clause 10件／MIT 1件） |
| CI `unit` | 両OSで success。実packageに対する`go test -race`が初めて走った |

主なtestの観点は、clock巻き戻し後も単調時間が進むこと、`AtomicWrite`失敗時にfileが旧内容のまま残り`Writes`へ記録されないこと、link能力不足時の作成拒否、`RemoveLink`がlink先を消さないこと、redirect上限とbody上限、`PassthroughStdio`時に内容をcaptureしないこと、cancel済みcontextの拒否、共有Injectorでのport横断順序である。

## 8. 次タスクへの申し送り

- P0-03完了によりP0の全taskが終わり、§4のG-CI（P0完了。CI matrixの全jobが最小構成でgreen）が満たされる。次はP1-01（`cmd/gdtvm`と§2のpackage骨格、依存方向のstatic check）。
- P1-01の依存方向static checkは、`internal/domain/port`がdomain値と標準libraryだけに依存する制約も対象に含める。
- P1-02でdomain値（ToolID、Version、Platform、Path等）が入ったら、現在std型で受けているport signatureをdomain型へ寄せられるか見直す。今回はdomain値が未実装のため`string`と標準library型で定義した。
- 残る8 port（Registry、Archive、Hash、Lock、Environment、Random、ProgressSink、Logger）は最初に必要とするtaskで追加し、同じ変更で§4.1と台帳を更新する。
