# P6-03 決定記録（3/5）: permission正規化と`E_CONFLICT`判定

対象タスク: `docs/13-progress.md` P6-03の3本目。規範仕様は[08-install-runtime.md](../08-install-runtime.md)§6・§7、[04-storage-and-data.md](../04-storage-and-data.md)§14、[02-architecture.md](../02-architecture.md)§1・§2・§4・§4.1、[10-security.md](../10-security.md)§5・§6。

## 1. 着手時の確認事項（2本目の停止記録より）

### 1.1 `port.FileSystem.Chmod`では両OSを表現できない

[08-install-runtime.md](../08-install-runtime.md)§7手順5「payloadを通常利用でread/execute onlyへ正規化する。**Windowsは現在userのwrite ACEを除き**、Linuxはdirectory 0555、executable 0555、その他0444を基本とする」。

`Chmod`のdoc commentは「Windows実装はfile属性へ写像できる範囲だけを扱う」と断っており、**ACE除去はPOSIX mode bitへ写像できない**。modeを渡す形のままだと、Windows adapterは表現できない要求を受け取ることになる。

[02-architecture.md](../02-architecture.md)§4.1はFileSystem portの操作に「permission」を挙げているため、portへ操作を足すこと自体は仕様の枠内である。**達成すべき結果**をkindで渡す`HardenReadExecute(path, kind)`を追加し、OS固有の実現方法はadapterへ委ねた。

| 案 | 採否 | 理由 |
|---|---|---|
| `Chmod`にmodeを渡す | 不採用 | Windowsのwrite ACE除去を表現できない |
| ACLをportへ公開する | 不採用 | §1「DomainとApplication Serviceから具体的OS APIを参照することを禁止する」。Windows security modelをcoreへ持ち込む |
| **結果をkindで渡す** | **採用** | 仕様が定めるのは結果（read/execute only）である。実現方法はadapterの責務 |

**treeの走査はportへ入れない。** §4「効果がすべて既存portの背後へ閉じているorchestrationはportにしない」。走査までportへ入れると、どのentryをどの種別で正規化したのかをfakeで確かめられなくなる。`HardenPayload`が`Walk`で走査し、entryごとの正規化だけをportへ委ねる。

### 1.2 `E_CONFLICT`の比較fieldは利用者判断で決めた

§7「完成先が競合して作られた場合、両receiptと`command_targets`が**完全一致**すれば後発stagingを破棄して成功、違えば`E_CONFLICT`」。

receiptは`install_id`（install毎のrandom 128 bit ID）と`installed_at`（時刻）を必須fieldに持つ。**文字どおり全fieldを比べると独立した2つのinstallでは絶対に一致せず、「一致すれば成功」という条項が到達不能なdead codeになる。** 仕様はどのfieldを除くかを書いていない。

A（install固有の識別子と時刻だけ除く）・B（`client_version`/`client_commit`も除く）・C（文字どおり全field比較）を提示し、**A**を採った。

| field | 比較 | 理由 |
|---|---|---|
| `install_id` | 除く | install毎のrandom値。同一内容でも必ず異なる |
| `installed_at` | 除く | 導入完了時刻。同上 |
| `probes[].finished_at` | 除く | probe終了時刻。同上 |
| それ以外すべて | **含む** | `client_version`／`client_commit`を含む |

除くのは**同一内容でも定義上必ず異なる値**だけであり、判断の余地を残さない。`client_version`と`client_commit`は同じclientを2回動かせば同じ値になるため除かない——異なるclient版が書いたreceiptは競合として表面化させる。

**この除外を[08-install-runtime.md](../08-install-runtime.md)§7へ明記した。** 仕様に無い挙動を実装だけで持たない。

## 2. 判断

### 2.1 payload root自身も正規化する

payload directoryが書込み可能なままだと、中身がread onlyでもentryの追加・削除ができる。

### 2.2 payload内のsymlink/reparse pointを拒否する

§6が展開時にこれらを拒否しており、payload内に存在しないはずである。現れた場合は展開後に差し込まれたことを意味し、permissionを正規化して先へ進むより失敗させるほうが安全である。

**判定は`FileInfo.IsSymlink`で行う。** `IsDir`だけを見ると、**Windowsのjunctionはdirectoryとして報告されるため普通のdirectoryとして正規化してしまい**、payload外を指すlinkがpayload内に残る。この経路を`TestHardenPayloadRejectsSymlink`のjunction caseが固定する。

### 2.3 比較は除外fieldをzero値へ落とす形で書く

fieldを1つずつ拾う形にすると、receiptへfieldが増えたときに比較から漏れる。**漏れた側は常に「一致」と判定される**ため、違う導入を同一と誤認する。zero値へ落とす形なら、増えたfieldは自動的に比較対象へ入る。件数は`TestConflictReasonCoversEveryField`が固定する。

## 3. 検査が固定したこと

| 検査 | 対象 |
|---|---|
| `TestHardenPayloadNormalizesEveryEntry` | 種別ごとのmode（directory/executable 0555、その他0444）、**payload root自身も対象**、正規化したentryの集合 |
| `TestHardenPayloadRejectsSymlink` | symlinkと**junction（reparse point）**の拒否 |
| `TestHardenPayloadReportsFailure` / `RejectsInvalidInput` | 失敗注入、role違い・未設定 |
| `TestPermissionKindCountMatchesSpec` | 種別数とzero値の無効性 |
| `TestSameInstallIgnoresPerInstallIdentity` | 除外3件で同一と判定されること |
| `TestSameInstallDetectsContentDifference` | 内容差15件の検出と診断field名（`client_version`/`client_commit`を含む） |
| `TestConflictReasonCoversEveryField` | 比較表がreceiptのfieldを網羅すること |
| `TestSameInstallHandlesNilProbes` | probe有無の扱い |

### 3.1 変異test

5件入れた。**1件目が生き残り、検査を強化した。**

`TestHardenPayloadRejectsSymlink`は当初symlinkだけを見ており、`IsSymlink`検査を外してもsymlinkが「通常fileでないentry」の分岐で拒否されるため通っていた。**junctionは`IsDir=true`で報告される**ため、その分岐に当たらず普通のdirectoryとして正規化される——これが実際の穴である。junction caseを足して固定した。

| 変異 | 初回 | 検査追加後 |
|---|---|---|
| payload root自身を正規化しない | 落ちた | — |
| `IsSymlink`検査を外す | **通った** | 落ちる（junction case） |
| `client_version`/`client_commit`も除外する（選択肢B） | 落ちた | — |
| 除外をやめる（選択肢C） | 落ちた | — |

## 4. 検証

Linux containerで実行した（Go 1.26.6）。両OSの判定はCI matrixで行う。

| command | 結果 |
|---|---|
| `gofmt -l .` / `go build ./...` / `GOOS=windows go build ./...` | 出力なし・成功 |
| `go vet ./...` / `GOOS=windows go vet ./...` | 成功 |
| `go test ./... -race -shuffle=on -covermode=atomic` | 全package成功。`internal/security` 97.2%・`internal/install` 89.9% |
| `check_policy.py` / `check_imports.py` / `check_docs.py` / `check_licenses.py` / `check_messages.py` | すべて成功 |
| `git diff --check` | 出力なし |

## 5. 未実施・制約

- **`HardenPayload`と`SameInstall`をまだ誰も呼んでいない。** 呼出し側は4本目のcommit transactionである。
- **§7手順6〜8（receipt書込み、atomic rename、receipt index更新）は未実装。** §7手順1〜3（staging payloadのroot内再検査、probe実行）と§8手順3〜5（`inputs`実体再取得、lock、`app.Guard`適用）も4本目である。**probe tempの実際の作成・削除も4本目**（2本目からの継続）。
- **`port.FileSystem.HardenReadExecute`のproduction実装は無い。** fakeとGuard wrapperだけである。`port.FileSystem`のproduction実装自体がP8-01であり、Windowsのwrite ACE除去はそこで実装する。**本PRはportの契約と呼出し側を固定しただけで、Windowsでの実挙動は未検証である。**
- **P6-02から継続する仕様の食い違いが1件**: `internal/store`のtemplate grammarが`internal/definition`と一致しない。fail closedは保たれ正当なdefinitionからは生じない値である。4本目で扱う。
- **P6-01から継続する未実装**: Registry portと`LinkManager` portの記録wrapper（P7）、[11-quality-and-ci.md](../11-quality-and-ci.md)§7.2のE2E照合（P6以降）、`port.Environment`のproduction実装（P8-01）、Windowsの起動とjob割当ての隙間、起動時の孤児staging cleanup。
- **P5-03から継続する未決が1件**（`./`始まりのtar entryを[10-security.md](../10-security.md)§5に従って拒否しており、標準4 toolの実archiveを繋ぐ4本目で該当があれば仕様側で扱いを決める）。**P6-01で埋めた仕様の空白が1件継続**（exact指定で`installable=false`のときの`E_PLATFORM_UNSUPPORTED`）。**仕様側の未決が2件継続**（[07-registry-and-tools.md](../07-registry-and-tools.md)§5第2項のlicense file size上限が§21の表に無い、§2の「license file名はASCII kebab grammar」が§2自身の例と食い違う）。`python.toml`の`lifecycle = "unknown"`はP3-04から継続。source error専用のerror code（`E_DEFINITION_INVALID`）、version数値要素の64 bit上限、`logs/` file名規約と§11「専用lock」の解釈は未決である。
