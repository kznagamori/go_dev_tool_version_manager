# P7-03 決定記録（2/2）: setup transactionと再起動要否

対象タスク: `docs/13-progress.md` P7-03の2本目（最終）。規範仕様は[09-platform.md](../09-platform.md)§6・§7・§8、[04-storage-and-data.md](../04-storage-and-data.md)§16・§16.1・§16.2、[15-deferred.md](../15-deferred.md) D-08。

## 1. 着手時の確認事項（1本目の停止記録より）

2件とも**仕様から一意に決まった**ため、利用者判断を求めていない。

### 1.1 「今回作成物」はin-memoryのundo listで持つ

§7「途中失敗は**今回作成物だけ**逆順rollbackし、既存tool/stateを削除しない」。作成物をどこへ記録するかが論点だった。

**[15-deferred.md](../15-deferred.md) D-08がoperation journal（`state/operations/*.jsonl`）を延期している。** 同項のv0.1での代替は「staging＋atomic renameとindex再構築で整合性を保てる」「operation tmpのdirectory単位削除」であり、journalを書く選択肢はv0.1に無い。

したがって巻き戻しは**同一process内のundo list**で行う。processごと落ちて巻き戻しが走らなかった場合の回復は、§7の**冪等な再実行**が担う（「setup済みrootへの再実行は差分だけを適用し、既に一致する項目をno-opとして報告する」「portable rootの移動後、link/shim/integrationを作り直す手段はこの再実行である」）。

§11のsetup stateは10 keyで閉じており作成物listを持てないが、**持つ必要が無い**というのがこの答えである。

### 1.2 `restart_required`はintegrationを実際に変更したときだけtrue

§16は`restart_required`を「trueと`W_RESTART_REQUIRED` exactly 1件を同値にする」と定めるが、**どの変更がtrueを導くかは書いていない**。次の3点から一意に決まる。

1. §16.2が`W_RESTART_REQUIRED`を「既存terminal/GUI processへ変更が反映されない」と定める。
2. §6「gdtvm shim directoryはuser PATH/profileの先頭へ1回だけ追加する」——利用者から見える経路はPATHだけである。
3. §8「VS Code等は起動時環境を保持する。setup/use後は新terminalまたはReload Windowが必要な場合を表示する」——反映されないのは起動時に環境を読み込むprocessである。

shimとtool本体はPATH経由で解決され、既存processも次回起動時に新しい実体を見る。**PATHそのものが変わらなければ既存processに不足は生じない。**

したがって`path_integration=none`ではfalse、冪等な再実行で何も変えなかった場合もfalseである。

## 2. 判断

### 2.1 書込み順を表で固定し、実行時に組み立てない

§7の6段階は**順序そのものが仕様**である。`StepOrder`として並びで持ち、`RunTransaction`が順序違反を`ErrStepOrder`で拒否する。実行時に組み立てると、段階を足したときに順序の意図が失われる。

**段階を飛ばすことは許し、並べ替えは許さない。** `path_integration=none`ではbackup／変更／検証の3段階が無い。

### 2.2 巻き戻しの対象を「この実行で変更した段階」に限る

`Apply`が`changed=false`を返した段階は何も作っていない。巻き戻すと**既に正しかった実体を消す**（§7「既存tool/stateを削除しない」）。

**失敗した段階自身は巻き戻す。** 途中まで作っている可能性があり、残すと次回の実行が既存entryで止まる。

### 2.3 巻き戻しは適用の逆順で行う

先に作ったものが後のものの前提になっているため、前から消すと後のものが宙に浮く。

### 2.4 元の失敗を巻き戻し失敗で上書きしない

利用者がまず知りたいのは「なぜsetupが止まったか」である。巻き戻し中の失敗は`RollbackErrors`へ集めて別に報告する。

**1件失敗しても残りの巻き戻しを続ける。** 途中で止めると、後続の作成物が残ったままになる。

### 2.5 `W_SHELL_MODIFICATION`と`W_RESTART_REQUIRED`を対で出す

§16.1は`W_SHELL_MODIFICATION`を「user PATHまたはshell profileの変更・除去」として**承認が必要**、§16.2は`W_RESTART_REQUIRED`を情報提供として**承認を要さない**と定める。

**2つは同じ条件で立つが、役割が違う。** 片方だけを出すと、利用者は承認を求められずにPATHを変えられるか、変わったことを知らされないままになる。

承認要否の表は`internal/store`が持つ。呼出し側が7件を並べ直すと、表を変えたときに片方だけが古いままになる。

## 3. 検査が固定したこと

`internal/shell`で26 caseを追加した。

| 検査 | 対象 |
|---|---|
| `TestStepOrderMatchesSpec` | §7の6段階の並びそのもの |
| `TestRunTransactionAppliesInOrder` | 適用順が§7と一致すること |
| `TestRunTransactionRollsBackInReverseOrder` | **巻き戻しが適用の逆順であること** |
| `TestRunTransactionUndoesOnlyThisRunsChanges` | **no-opの段階を巻き戻さないこと**（§7「今回作成物だけ」） |
| `TestRunTransactionUndoesFailedStepItself` | 途中まで作って失敗した段階を巻き戻すこと |
| `TestRunTransactionKeepsOriginalErrorOverRollbackFailure` | **元の失敗を上書きしないこと** |
| `TestRunTransactionContinuesRollbackAfterFailure` | 巻き戻しを1件目の失敗で止めないこと |
| `TestRunTransactionAllowsSkippedSteps` | `path_integration=none`で3段階を飛ばせること |
| `TestRunTransactionRejectsOutOfOrderSteps` | 逆順・重複・未知・空の4 case |
| `TestRunTransactionReportsIdempotentRerun` | §7の冪等な再実行がno-opとして報告されること |
| `TestRestartRequiredFollowsIntegrationChange` | **integration変更のときだけtrue**、shimだけ／段階なしでfalse |
| `TestIntegrationWarningsPairApprovalAndNotice` | 2 warningが対で立つこと、承認要否が§16.1の表と一致すること |

### 3.1 変異test

7件入れ、いずれも検査が落ちた。生き残りは無い（1本目と合わせて15件）。

| 変異 | 結果 |
|---|---|
| 巻き戻しを適用順で行う | 落ちた |
| 変更していない段階も巻き戻す | 落ちた |
| 巻き戻し失敗で元の失敗を上書きする | 落ちた |
| 巻き戻しを1件目の失敗で止める | 落ちた |
| 段階の順序検査を外す | 落ちた |
| shim変更でも再起動を要求する | 落ちた |
| 承認warningを出さない | 落ちた |

## 4. 検証

Linux containerで実行した（Go 1.26.6）。両OSの判定はCI matrixで行う。

| command | 結果 |
|---|---|
| `gofmt -l .` / `go build ./...` / `GOOS=windows go build ./...` | 出力なし・成功 |
| `go vet ./...` / `GOOS=windows go vet ./...` | 成功 |
| `GOOS=windows go test -c` | 成功 |
| `go test ./... -race -shuffle=on -covermode=atomic` | 全package成功。`internal/shell` 90.8% |
| `check_policy.py` / `check_imports.py` / `check_docs.py` / `check_licenses.py` / `check_messages.py` | すべて成功 |
| `git diff --check` | 出力なし |

`allowedGlobals`へ3件を登録した。`internal/shell`のimport表は1本目から変えていない。

## 5. 未実施・制約

- **6段階の中身を実装していない。** 本PRが固定したのは**順序・巻き戻し・冪等性の意味**であり、各段階が実際に何を書くかは呼出し側が`Action`として渡す。root/state初期化とsetup state commitは`port.FileSystem`のproduction実装（P8-01）を、shim生成はP7-02の`shim.Deployer`を、integrationの3段階は**P7-04**（Windows HKCU Path、Linux profile marker）を要する。
- **`SetupPlan`を組み立てていない。** §16の15 fieldのうち本taskが決めたのは`filesystem_capabilities`／`current_link_strategy`／`shim_strategy`（1本目）と`restart_required`の導出規則（本PR）である。残る11 field（mode、旧新root、shim path、integration方式と対象、backup）は、rootとconfigを持つApplication Service側（**P8-02**「Setup/Install/Use/UninstallのPlan/Execute mapping」）が埋める。
- **`--remove`の実体を実装していない。** §7「`setup --remove`はsetup stateで所有を証明できるPATH entryまたはmarkerだけを除去する」の**所有証明**は§9の`integration_identity`（`before_sha256`／`after_sha256`）を読む処理であり、integrationの中身と同じくP7-04の範囲である。本PRのtransactionは`setup`と`setup-remove`のどちらにも使える形にしてある。
- **journalを書かない。** D-08の延期に従う。processごと落ちた場合の回復は§7の冪等な再実行であり、**その再実行が実際に差分だけを適用することはP7-04の統合後にE2Eで確かめる**（[11-quality-and-ci.md](../11-quality-and-ci.md)§8）。
- **P7-03(1/2)から継続**: probeを実filesystemで走らせていない（fake portでの検査。能力を過大に報告する側へは倒れない）。`file-identity`のprobeは`port.FileInfo`がinode/file indexを持たないため`RealPath`の区別と安定性で判定する。
- **P7-02から継続**: §10手順2〜6と`cmd/gdtvm`の分岐はP8-03／P8-04の範囲。`internal/shim`の`sameFile`はsize・mode・mtimeで判定する。
- **P7-01から継続**: junction経路のsyscallは`windows-latest`だけが動かす（PR #145で確認済み）。`port.LinkCapabilities`は失敗理由を運べない。
- **P6-03から継続する未実装**: 合成側の`InstallEngine` adapter、`app.Guard`を噛ませた経路のE2E照合（§7.2）、receipt indexの再構築、`port.FileSystem`／`port.Environment`のproduction実装（いずれもP8-01）。
- **P6-02から継続する食い違いが1件**: `internal/store`のtemplate grammarが`internal/definition`と一致しない。fail closedは保たれ、正当なdefinitionからは生じない値である。§2の責務表を要する判断であり未着手。
- **P5-03から継続する未決が1件**（`./`始まりのtar entryを[10-security.md](../10-security.md)§5に従って拒否している）。**P6-01で埋めた仕様の空白が1件継続**（exact指定で`installable=false`のときの`E_PLATFORM_UNSUPPORTED`）。**仕様側の未決が2件継続**（[07-registry-and-tools.md](../07-registry-and-tools.md)§5第2項のlicense file size上限が§21の表に無い、§2の「license file名はASCII kebab grammar」が§2自身の例と食い違う）。`python.toml`の`lifecycle = "unknown"`はP3-04から継続。source error専用のerror code、version数値要素の64 bit上限、`logs/` file名規約と§11「専用lock」の解釈は未決である。
