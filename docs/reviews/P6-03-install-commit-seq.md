# P6-03 決定記録（5/6）: commit transaction

対象タスク: `docs/13-progress.md` P6-03の5本目。規範仕様は[08-install-runtime.md](../08-install-runtime.md)§7、[04-storage-and-data.md](../04-storage-and-data.md)§4・§13・§14、[02-architecture.md](../02-architecture.md)§12、[10-security.md](../10-security.md)§6。

## 1. 着手時の確認事項（4本目の停止記録より）

2件とも**仕様から一意に決まった**ため、利用者判断を求めていない。

### 1.1 `Rename`だけで表現できるかは問題にならない

`port.FileSystem.Rename`のdoc commentは「同一filesystem上で移動する。commit操作に使う」とだけ定め、**完成先が存在する場合の挙動を定義していない**。しかし§7手順7は「**完成先がないことを確認し**、version directoryを同一volumeでatomic renameする」と、事前確認を明示的に求めている。

仕様が事前確認を求めるのは、rename先が存在する場合の挙動がOSで違うためである（POSIXは空directoryを置換しうるが、WindowsのMoveFileは失敗する）。**`Rename`にその差を吸収させる必要がない**ため、portの変更は要らない。

確認とrenameの間に別processが完成先を作る余地は残る。**その競合こそ同§が「完成先が競合して作られた場合」として扱う場面である**。renameが失敗したらもう一度完成先を見て、同一内容なら成功として扱う。

### 1.2 `ClassInstall`のlock keyは§12の順でqualifierへ並べる

`port.LockKey(class, qualifier)`がqualifierをpath componentとして検査したうえで`<class>~<qualifier>~...`へ組み立てる。[02-architecture.md](../02-architecture.md)§12の`ClassInstall`は「ToolID、version、platform順」であり、`LockKey(port.ClassInstall, []string{tool, version, platform})`となる。

**本PRではlock取得を実装していない**（§8手順3〜5は6本目）。確認事項として答えを確定させ、実装は次へ送る。

## 2. 判断

### 2.1 receiptはstagingへ書いてからrenameする

§7手順6→7の順である。完成先へ直接書くと、rename前の中断で半端なreceiptが完成先へ残る。

receiptはpayloadの**兄弟**（version directory直下）に置く。§14が保存pathを`.gdtvm-install.toml`、`payload_path=payload`固定と定めることから決まる。

### 2.2 rename後の失敗でinstallを失敗にしない

§7「**手順7のrenameが完了した時点でinstallは成功とみなす**。rename前の中断は未導入、rename後の中断は導入成功でindexだけ古い状態であり、次回起動時の再構築で解消する」。

したがって手順8（index更新）の失敗で**完成先を巻き戻さない**。巻き戻すと、成功した導入を失う。`TestCommitSucceedsInstallEvenIfIndexFails`がこれを固定する。

### 2.3 破損したreceiptを持つ完成先を黙って上書きしない

[04-storage-and-data.md](../04-storage-and-data.md)§4「内部TOML/JSONのunknown key、重複、型違い、上限超過は破損として扱い、**黙って修復・再生成しない**」。完成先はあるがreceiptを読めない場合は`E_CONFLICT`とし、既存を消さない。

### 2.4 staging payloadの再検査を`Extractor`任せにしない

§7手順1「staging payloadの全pathがroot内にあることを**再検査**する」。展開側が同じ検査を済ませているが、展開からcommitまでの間にsymlinkが差し込まれていないことをここで確かめる。**1箇所の検査に頼らない。**

判定は`FileInfo.IsSymlink`で行う（3本目と同じ理由 — junctionは`IsDir=true`で報告される）。

## 3. `SameInstall`の欠陥を1件修正した

**commit経路へ繋いだところで、3本目に入れた`SameInstall`が実際には常に不一致を返すことが分かった。**

完成先のreceiptはdiskから読んだもの、比較相手は今memoryで組み立てたものであり、両者は同じ経路を通っていない。**TOMLを往復すると空arrayは長さ0のsliceになるが、memory側は`nil`を持つ。** `reflect.DeepEqual(nil, []T{})`は`false`である。

その結果、同一内容でも常に不一致となり、**§7の「一致すれば成功」が実際には到達しない** — 利用者判断で選んだ選択肢Aが目的としていた冪等installが、実装では効かない状態だった。

3本目の検査は`conflictReceipt`同士を直接比べており、往復を挟んでいなかったため見つからなかった。空sliceと`nil`を同じに扱うよう直し、**往復を挟む`TestSameInstallSurvivesCodecRoundTrip`を足した** — これが実際の比較の形である。§14も「arrayはstorageだけ空可」として空と不在を区別していない。

## 4. 検査が固定したこと

25 subtestを追加した。

| 検査 | 対象 |
|---|---|
| `TestCommitRenamesAndUpdatesIndex` | 手順6〜8の成功経路（receiptの位置、rename、revision、index entry） |
| `TestCommitTreatsIdenticalExistingAsSuccess` | 同一内容の既存を成功として扱うこと |
| `TestCommitRejectsDifferentExisting` | 内容差で`E_CONFLICT`、診断に不一致fieldが載ること |
| `TestCommitRejectsUnreadableExisting` | 破損receiptを黙って上書きしないこと |
| `TestCommitRejectsSymlinkInPayload` | 展開後に差し込まれたsymlink/junctionを`E_PATH_UNSAFE`にし、**renameしないこと** |
| `TestCommitSucceedsInstallEvenIfIndexFails` | **index失敗で完成先を巻き戻さないこと** |
| `TestCommitUpsertsIndexEntry` | 同じtupleを二重に持たないこと（§13） |
| `TestCommitReportsFailures` / `RejectsInvalidRequest` | 失敗注入、前提違反10件 |
| `TestSameInstallSurvivesCodecRoundTrip` | **TOML往復後も同一と判定されること**（§3の欠陥の回帰） |

### 4.1 変異test

2件入れ、いずれも検査が落ちた。

| 変異 | 結果 |
|---|---|
| 空slice正規化を外す（§3で直した欠陥） | 落ちた（2 case） |
| 完成先の事前確認を外す | 落ちた（2 case） |

## 5. 検証

Linux containerで実行した（Go 1.26.6）。両OSの判定はCI matrixで行う。

| command | 結果 |
|---|---|
| `gofmt -l .` / `go build ./...` / `GOOS=windows go build ./...` | 出力なし・成功 |
| `go vet ./...` / `GOOS=windows go vet ./...` | 成功 |
| `go test ./... -race -shuffle=on -covermode=atomic` | 全package成功。`internal/install` 88.4% |
| `check_policy.py` / `check_imports.py` / `check_docs.py` / `check_licenses.py` / `check_messages.py` | すべて成功 |
| `git diff --check` | 出力なし |

## 6. 未実施・制約

- **`Committer`をまだ誰も呼んでいない。** install engineの部品（`Stager`／`ProbeRunner`／`CollectCommandTargets`／`HardenPayload`／`Committer`）が揃ったが、**それらを順に呼ぶorchestrationが無い**。6本目で§8手順3〜5（`inputs`実体再取得、lock取得と取得後の再検査、`app.Guard`適用）とともに繋ぐ。
- **lock取得を実装していない。** §8手順4と§12のlock順序は6本目である。key組立ての答えは§1.2で確定させた。
- **`W_CLEANUP_INCOMPLETE`への変換を実装していない。** §2「commit後の一時file清掃失敗は成功＋`W_CLEANUP_INCOMPLETE`」。`Committer`はindex更新失敗をerrorとして返すだけで、warningへの変換は呼出し側（6本目）の責務である。
- **receipt indexの再構築は実装しない。** §7が「次回起動時」と定めており、起動時処理（`Initialize`、P8-01）の責務である（4本目からの継続）。
- **P6-02から継続する仕様の食い違いが1件**: `internal/store`のtemplate grammarが`internal/definition`と一致しない。fail closedは保たれ正当なdefinitionからは生じない値である。6本目で扱う。
- **P6-01から継続する未実装**: Registry portと`LinkManager` portの記録wrapper（P7）、[11-quality-and-ci.md](../11-quality-and-ci.md)§7.2のE2E照合（P6以降）、`port.FileSystem`／`port.Environment`のproduction実装（P8-01。`HardenReadExecute`のproduction実装もここ）、Windowsの起動とjob割当ての隙間、起動時の孤児staging cleanup。
- **P5-03から継続する未決が1件**（`./`始まりのtar entryを[10-security.md](../10-security.md)§5に従って拒否しており、標準4 toolの実archiveを繋ぐ6本目で該当があれば仕様側で扱いを決める）。**P6-01で埋めた仕様の空白が1件継続**（exact指定で`installable=false`のときの`E_PLATFORM_UNSUPPORTED`）。**仕様側の未決が2件継続**（[07-registry-and-tools.md](../07-registry-and-tools.md)§5第2項のlicense file size上限が§21の表に無い、§2の「license file名はASCII kebab grammar」が§2自身の例と食い違う）。`python.toml`の`lifecycle = "unknown"`はP3-04から継続。source error専用のerror code（`E_DEFINITION_INVALID`）、version数値要素の64 bit上限、`logs/` file名規約と§11「専用lock」の解釈は未決である。
