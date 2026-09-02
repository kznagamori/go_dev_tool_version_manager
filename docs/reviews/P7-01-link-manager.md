# P7-01 決定記録: junction／symlinkのproduction adapter

対象タスク: `docs/13-progress.md` P7-01。規範仕様は[09-platform.md](../09-platform.md)§3.1〜§3.3・§5.1・§9、[02-architecture.md](../02-architecture.md)§1・§2・§4.1。

## 1. P6-04ではなくP7-01へ進んだ理由

P6-03完了時の停止記録は次をP6-04としていた。**依存gateの確認漏れである。**

| task | 依存 | 状態 |
|---|---|---|
| P6-04 | P6-03, **P7-01**, **P7-02** | P7-01・P7-02が未着手 |
| P7-01 | P2-03 | 完了 |

CLAUDE.md §5「依存未完了タスクを、実装しやすさだけを理由に先行させない」に従い、依存が満たされているP7-01から着手した。

## 2. 着手時の確認事項

2件とも**仕様から一意に決まった**ため、利用者判断を求めていない。

### 2.1 `replace`はportへ操作を足さずに既存操作の列で表す

P7-01の項目は「create/verify/**replace**/unlink」と書くが、[port.LinkManager](../../internal/domain/port/link.go)に`Replace`は無い。§3.2が定める列は次である。

```text
temporary junctionを作成 → targetを再読 → selection stateをcommit → 旧junctionをunlink → temporary名をrename
```

renameは`FileSystem` portの操作であり、**この列は既存の操作だけで書ける**。§5.1のLinuxも同じ形である。したがってportへ操作を足さず、列が両OSで成立すること（とくに旧targetの中身が残ること）をtestで固定した。順序と巻き戻しはP6-04のselection transactionが持つ。

### 2.2 `Capabilities`は実際に作って消すprobeにする

§3.1は「必須能力が欠ける場合は**setup probe**で理由付き拒否する」と定める。filesystem種別名から推測する実装にしなかったのは、**Windows標準userがsymlinkを作れるかがfilesystemではなくprivilegeとDeveloper Modeで決まる**ためである。NTFSかどうかを見ても答えが出ない。

## 3. 判断

### 3.1 junctionは`FSCTL_SET_REPARSE_POINT`を直接呼ぶ

**Go標準libraryにjunctionを作るAPIが無い。** `os.Symlink`はsymbolic linkを作り、§3.2が求めるdirectory junctionにはならない。両者はWindows上で別のreparse tagを持ち、標準userがsymlinkを作るには特権かDeveloper Modeが要るのに対しjunctionは要らない。`x/sys/windows`はreparse bufferの構造体を非公開にしているため、wire形式を自前で組み立てる。

### 3.2 reparse bufferの組立てと解釈をplatform非依存のfileへ置く

`link_reparse.go`はbuild tagを持たない。**Windowsでしかbuildしない場所へ置くと、境界検査の退行がWindows jobまで見つからない。** ここはOS APIを呼ばない純粋なbyte変換であり、Linux側のtestでも同じ検査が走る。

自前の定数がOS定義とずれる危険は`link_reparse_windows_test.go`が止める。ずれると**Linux側のtestは通ったままWindowsで別種のreparse pointを作る**ため、一致をtestで固定した。

### 3.3 `SubstituteName`を読み、`PrintName`を読まない

`PrintName`は表示用で空でもよく、実際に解決へ使われるのは`SubstituteName`である。junctionの`\??\`前置は保存形式であって利用者が指定したtargetの一部ではないため取り除く。

`C:\`のようなvolume rootからは末尾separatorを削らない。削ると`C:`となり、drive相対pathという別の意味になる。

### 3.4 absoluteでないpathと正規形でないpathを直さずに拒否する

absoluteでないpathは暗黙のcurrent directoryに依存する（§4「暗黙working directory…を使わない」）。正規形でないpathを黙って畳むと、途中componentがlinkの場合に畳んだ結果が実体と食い違う。**どちらも直さずに拒否する。**

### 3.5 `RemoveLink`はlinkでないpathを拒否する

「link先の実体を消してはならない」（portのdoc comment）を守るには、消す前に対象がlinkであることを確かめる必要がある。`os.Remove`だけを使い`os.RemoveAll`を使わないのは、junctionがdirectory属性を持ち、再帰削除がtarget側へ入りうるためである（§3.2「junction targetを再帰削除しない」）。

### 3.6 `Kind`はreparse pointを先に見て、次にlink countを見る

junctionはdirectory属性も持つため、directoryかどうかで先に振り分けるとjunctionを通常のdirectoryとして扱う。

**未知のreparse tagは`LinkNone`ではなくerrorにする。** `LinkNone`を返すと呼出し側が通常のdirectoryと区別できず、置換してよいものとして扱ってしまう（§3.2「別root reparse pointなら自動置換せず`doctor`診断とする」）。

link countが2以上のregular fileを`LinkHardlink`とするのは、§3.3の公開command shimがその形であり、setupの冪等性が「既にhardlinkである」と分かることに依存するためである。

### 3.7 hardlink targetがregular fileであることを`Lstat`で確かめる

§3.3はWindowsのshimを「clientへのhardlink」と定める。link先がさらにlinkだと、shimが起動するexecutableがそのlinkの状態で変わる。

### 3.8 Linuxで`CreateJunction`をsymlinkへ代替しない

呼出し側がjunctionを要求したのは、そのplatformの規則がjunctionだからである。黙って別種のlinkを作ると、Windows前提の検査がLinuxで別の実体を見ることになる。

## 4. 検査が固定したこと

両OS共通で28、Linux専用で3、Windows専用で5、reparse解析で13のcaseを入れた。

| 検査 | 対象 |
|---|---|
| `TestLinkManagerCreatesRelativeSymlink` | §5.1のrelative symlink。**保存値がrelativeであること**と実際に辿れること |
| `TestLinkManagerCreatesAbsoluteSymlink` | `ReadLink`が保存値をそのまま返すこと（§5.1の「absolute targetを拒否する」検査を呼出し側が行えるため） |
| `TestLinkManagerRemoveLinkKeepsTarget` | **link先の実体を消さないこと** |
| `TestLinkManagerRemoveJunctionKeepsTarget` | 同上のjunction版（§3.2「junction targetを再帰削除しない」） |
| `TestLinkManagerRefusesToRemoveNonLink` | 通常のdirectory/fileを消さないこと |
| `TestLinkManagerHardlink` | §3.3のshim形。作成→`Kind`→除去で元のfileが残ること |
| `TestLinkManagerRejectsHardlinkToNonRegularFile` | directory／symlinkへのhardlinkを拒否すること |
| `TestLinkManagerRejectsUnsafePaths` | 14 callで相対path・非正規pathを拒否し、**拒否がfilesystemへ何も残さないこと** |
| `TestLinkManagerCapabilitiesLeavesNothingBehind` | probeの残骸が無いこと、2回目が同じ結果になること |
| `TestLinkManagerReplaceSequence` | §3.2・§5.1の置換列（両OS） |
| `TestLinkManagerReplacesJunction` | 同上のjunction版 |
| `TestLinkManagerRejectsJunctionToFile` | 失敗時に空のdirectoryを残さないこと |
| `TestLinkManagerJunctionExistsRejected` | 既存entryへ上書きしないこと、失敗しても既存linkが壊れないこと |
| `TestLinkManagerRejectsJunctionOnLinux` | symlinkで代替しないこと |
| `TestLinkManagerKindIgnoresSymlinkTargetKind` | `Kind`がpath自体を見ること。壊れたsymlinkも判定・除去できること |
| `TestParseReparsePointRejectsMalformedBuffer` | **範囲外を読まないこと**（6 case） |
| `TestMountPointReparseDataRoundTrip` | 組立てた値がそのまま戻ること（4 case） |
| `TestParseReparsePointReadsSymlinkLayout` | symlinkだけが持つ`Flags`分のoffset差 |
| `TestReparseConstantsMatchSystemHeaders` | 自前定数とOS定義の一致（Windows専用） |

### 4.1 変異test

6件入れ、いずれも検査が落ちた。生き残りは無い。

| 変異 | 結果 |
|---|---|
| `RemoveLink`のlink種別guardを外す | 落ちた |
| hardlink targetのregular file検査を外す | 落ちた |
| pathの正規形検査を外す | 落ちた |
| reparse bufferの範囲検査を外す | 落ちた |
| probeが作ったlinkを消さない | 落ちた |
| `CreateSymlink`がrelative指定を無視する | 落ちた |

### 4.2 test自身の欠陥を1件直した

非正規pathのcaseを`filepath.Join(root, "a", "..", "b")`で作っていたが、**`filepath.Join`は畳むため生成されるのは正規pathである。** 検査は「拒否されなかった」で落ち、testが意図した入力を作れていないことが分かった。separatorを直接繋いで`..`を残す形へ直した。

## 5. 検証

Linux containerで実行した（Go 1.26.6）。両OSの判定はCI matrixで行う。

| command | 結果 |
|---|---|
| `gofmt -l .` / `go build ./...` / `GOOS=windows go build ./...` | 出力なし・成功 |
| `go vet ./...` / `GOOS=windows go vet ./...` | 成功 |
| `GOOS=windows go test -c` | 成功（Windows専用testのcompileを確認） |
| `go test ./... -race -shuffle=on -covermode=atomic` | 全package成功。`internal/platform` 93.7% |
| `check_policy.py` / `check_imports.py` / `check_docs.py` / `check_licenses.py` / `check_messages.py` | すべて成功 |
| `git diff --check` | 出力なし |

`internal/platform`のimport表は変えていない。`internal/app/globalstate_test.go`の`allowedGlobals`へsentinel error 5件を根拠付きで登録した。

## 6. 未実施・制約

- **junction経路をローカルで実行できていない。** Linux containerでの実行であり、`CreateJunction`／`readReparsePoint`／`fileLinkCount`のsyscallは`windows-latest` jobが初めて動かした。**PR #145の`unit (windows-latest)`が成功しており、`DeviceIoControl`によるjunctionの作成・読取り・除去・置換はCI上で確認済みである**（ローカル再現手段は無い）。
- **`Capabilities`が失敗理由を運べない。** [port.LinkCapabilities](../../internal/domain/port/link.go)は真偽値だけを持ち、「権限が無い」と「filesystemが対応しない」を区別できない。§3.1の「理由付き拒否」はsetup（P7-03）が自分で検査して報告する。**portへ理由fieldを足していない** —— 使う側が無いうちに足すとCLAUDE.md §7の「未使用のfieldを将来のために残さない」に反する。
- **§3.1の残る必須能力を検査していない。** atomic replace、file identity、ACL、long path、Windows予約名・末尾dot/space・ADS・Unicode case-foldの検査はP7-03（setup probe）とP2-03（`internal/security`）の範囲であり、P7-01はlink 3種の作成可否だけを扱う。
- **`Kind`のhardlink判定はlink countだけを見る。** どの名前が「元」かは判定しない —— filesystemにその区別が無いためである。§3.3が求めるshim実体の同一性検査（SHA-256/version/owner）はP7-02が行う。
- **P6-03から継続する未実装**: 合成側の`InstallEngine` adapter（P8-01）、`app.Guard`を噛ませた経路のE2E照合（[11-quality-and-ci.md](../11-quality-and-ci.md)§7.2）、receipt indexの再構築（P8-01）、`port.FileSystem`／`port.Environment`のproduction実装（P8-01）。
- **P6-02から継続する食い違いが1件**: `internal/store`のtemplate grammarが`internal/definition`と一致しない（`{{storage.9x}}`をdefinitionは拒否・storeは通す）。fail closedは保たれ正当なdefinitionからは生じない値である。§2の責務表を要する判断であり未着手。
- **P5-03から継続する未決が1件**（`./`始まりのtar entryを[10-security.md](../10-security.md)§5に従って拒否している）。**P6-01で埋めた仕様の空白が1件継続**（exact指定で`installable=false`のときの`E_PLATFORM_UNSUPPORTED`）。**仕様側の未決が2件継続**（[07-registry-and-tools.md](../07-registry-and-tools.md)§5第2項のlicense file size上限が§21の表に無い、§2の「license file名はASCII kebab grammar」が§2自身の例と食い違う）。`python.toml`の`lifecycle = "unknown"`はP3-04から継続。source error専用のerror code（`E_DEFINITION_INVALID`）、version数値要素の64 bit上限、`logs/` file名規約と§11「専用lock」の解釈は未決である。
