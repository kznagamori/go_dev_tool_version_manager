# P5-03 決定記録（1/2）: archive entry事前検査

対象タスク: `docs/13-progress.md` P5-03の1本目。規範仕様は[10-security.md](../10-security.md)§5、[04-storage-and-data.md](../04-storage-and-data.md)§21、[02-architecture.md](../02-architecture.md)§2・§7・§14・§17、[11-quality-and-ci.md](../11-quality-and-ci.md)§1。

## 1. 着手時の確認事項（P5-02の停止記録より）

2件とも解決した。

### 1.1 §21の上限は判定箇所が2つに分かれる

| 上限 | 値 | 判定箇所 |
|---|---|---|
| archive entry | 200,000 | 展開前の全entry検査 |
| 単一file / 総展開 | 4 GiB / 20 GiB | 宣言sizeで**事前**、実展開bytesで**展開中**の両方 |
| 圧縮比（entry/全体） | 1,000 | 事前（宣言値）＋展開中（実測） |

宣言sizeだけでは足りない。zip bombは宣言を偽れるため、展開中の実bytesでも打ち切る必要がある。本PRは事前側だけを実装した。

### 1.2 `port.FileSystem`は展開に足りる

`WriteStream`（P5-02で追加）でfile内容、`MkdirAll`／`Chmod`／`Rename`／`RemoveAll`／`Stat`／`RealPath`で残りを賄える。**新しいport methodは不要**である。

## 2. 利用者判断: archive展開をportにせず、以後は同種の判断を自分で行う

§4.1・§7が`ArchiveExtractor`をportとして挙げていたが、archiveの外部作用はすべて`port.FileSystem`の背後へ閉じており、zip/tar解析は標準libraryの純計算である。P5-03の主眼は安全検査そのもので、portで差し替えられるようにするとその検査をtestで確かめられなくなる。

利用者判断で**portにせず**、§4.1の表から`ArchiveExtractor`行と§7の列挙から`Archive`を外した。あわせて**一般則**を§7へ明記した。

> **portにするのは外部作用そのものだけとする。** 次の2つはportにしない。
>
> - 外部作用を持たない純計算（digest計算）
> - 効果がすべて既存portの背後へ閉じているorchestration（archive展開）

同種の判断（Hash、Archive）が3回続いたため、以後は確認を取らず実装と仕様同期を進める合意も得た。

## 3. 依存module追加: `golang.org/x/text`

§5がarchive entryの「invalid/**non-NFC** Unicode」拒否を求めるが、**Go標準libraryはUnicode正規化を持たない**。NFC判定表を自前で持つとUnicode版の追随が必要になり、正規化の誤りはpath衝突検査をすり抜ける。

`golang.org/x/text` v0.41.0（BSD-3-Clause、module依存0件）を採用し、[11-quality-and-ci.md](../11-quality-and-ci.md)§1の依存module表へ§17が求める記録（SPDX、transitive dependency、採用理由、置換可能性）を追加した。`check_licenses.py`の許可listを通る。

## 4. 判断

### 4.1 1件でも違反があれば展開しない

安全なentryだけを選んで展開すると、archiveが意図した構成と違うものが出来上がり、probeが何を検査しているか分からなくなる。

### 4.2 case衝突をpath prefix単位で見る

**実装中にtestが検出した欠陥である。** 当初は完全pathだけを比較しており、`bin/go`と`BIN/other`が通っていた。file名は違うが、Windowsでは同じdirectoryを指すため展開後の構成が一意にならない。各pathの全prefixを登録し、同じ正規化keyに違う表記が現れたら衝突として拒否するよう直した。

Linuxでも衝突させない。同じarchiveを両OSで同じ構成へ展開するためである。

### 4.3 全体の圧縮比は同じ母集団で比べる

**これもtestが検出した欠陥である。** 展開後sizeは全fileから合計する一方、圧縮後sizeは持つentryだけから合計していたため、tarのように個別圧縮後sizeを持たないentryがあると比が実体と無関係に膨らみ、4 GiBちょうどの正当なarchiveまで拒否していた。圧縮比を出せるentryだけの合計どうしで比べるよう直した。

### 4.4 予約名listを複製せず`internal/security`を使う

当初は`windowsReservedNames`を`internal/install`へ複製していたが、§2が「path検査」を`internal/security`の責務としており、`internal/definition`も同じ理由で複製を避けている。`security.ValidateComponent(component, windows)`へ委ね、空/`.`/`..`/NUL/区切り/長さ/UTF-8と、Windows時のADS・予約名・末尾空白/dotを任せた。

これに伴い、**ADS・末尾空白/dot・予約名はWindows hostでだけ拒否する**。Linuxでは通常のfile名として有効であり、一律に拒否すると正当なarchiveを扱えない——`internal/security`が既に持つ判断へ揃えた。testもWindows専用として分離した。

### 4.5 加算overflowをfail closedで扱う

総展開sizeの加算は`total > ArchiveTotalMaxBytes-entry.Size`で判定する。`total + entry.Size`を先に計算すると、宣言sizeが極端なarchiveでoverflowして上限を下回る。

## 5. 検査が固定したこと

| 検査 | 対象 |
|---|---|
| `TestInspectEntriesAcceptsStandardLayout` | Go相当のlayoutが両OSで通り、strip後のpathとTotalBytesが合うこと |
| `TestInspectEntriesStripComponentsZero` | .NET相当のtop-level directoryなしlayout |
| `TestInspectEntriesRejectsUnsafeName` | absolute／drive／UNC／`..`／`.`／空component／backslash／空name／root（12件） |
| `TestInspectEntriesRejectsWindowsOnlyNames` | ADS・末尾空白/dotがWindowsでだけ拒否されること |
| `TestInspectEntriesRejectsControlAndNonNFC` | NUL／制御文字／DEL／改行／format制御文字／NFD、およびNFC非ASCIIが通ること |
| `TestInspectEntriesRejectsLinksAndSpecialFiles` | symlink／hardlink／特殊file／未知種別 |
| `TestInspectEntriesRejectsDuplicateAndCaseCollision` | 完全重複／file名case違い／**directory case違い**を両OSで |
| `TestInspectEntriesRejectsWindowsReservedNames` | 予約名8種（拡張子付き含む）、Linuxでは通ること |
| `TestInspectEntriesEnforcesLimits` | entry数、単一file上限（超過と上限ちょうど）、総展開、entry圧縮比、圧縮後size不明時 |
| `TestInspectEntriesRejectsInvalidRequest` | entry 0件、stripが負/2、size負、圧縮後size負、strip後に空 |

## 6. 検証

すべてLinux containerで実行した（Go 1.26.6）。

| command | 結果 |
|---|---|
| `gofmt -l .` / `go build ./...` / `go vet ./...` | 出力なし・成功 |
| `go test ./... -race -shuffle=on -covermode=atomic` | 全package成功。`internal/install` 91.8% |
| `check_policy.py` / `check_imports.py` / `check_docs.py` / `check_licenses.py` / `check_messages.py` | すべて成功（`check_licenses`はmodule 15件） |
| `check_pr_refs.py` / `git diff --check` | 成功・出力なし |

## 7. 未実施・制約

- **2本目の範囲が残っている。** 実際の展開（zip/tar.gz読取り、same-volume staging、permission正規化、atomic rename、cleanup）とfailure injection testは`claude/feature-p5-03-archive-extract-apply`で実装する。**展開中の実bytes打ち切り**（宣言sizeを偽るzip bomb対策）もそちらである。
- **`InspectEntries`をまだ誰も使っていない。** 呼出し側は2本目の展開処理である。同じtaskの2本目で必ず使う前提での分割であることを本記録と進捗台帳へ明示した。
- **zip/tarのentryを`Entry`へ写す処理は書いていない。** formatごとの差（zipは個別の圧縮後sizeを持つ、tarは持たない）を吸収する層が2本目に要る。
- **symlink raceの再検査は未実装。** §5の「検査と実書込みの間にもparent identity/containmentを確認」は展開側の責務で、2本目の範囲である。
- `InspectEntriesResult`は検査後に同じ走査をもう一度行う。entry数の上限が200,000で、走査は文字列検査だけのため許容した。展開側が結果を使う形が固まった時点で、1回の走査で済ませるか判断する。
