# P7-03 決定記録（1/2）: filesystem能力のprobeとlink方式の決定

対象タスク: `docs/13-progress.md` P7-03の1本目。規範仕様は[09-platform.md](../09-platform.md)§2.3・§3.1・§3.3・§5.1・§7・§9、[04-storage-and-data.md](../04-storage-and-data.md)§16・§17.1、[02-architecture.md](../02-architecture.md)§2・§4・§4.1。

## 1. 着手時の確認事項

2件とも**仕様から一意に決まった**ため、利用者判断を求めていない。

### 1.1 capability probeへportを足さない

§17.1のfilesystem capabilityは7値である。うちlink 3種はP7-01の`LinkManager.Capabilities`が返すが、残る4値を返す口が無い。portへ操作を足すかが論点だった。

**§4.1は「能力検査」を`LinkManager`にだけ挙げ、`FileSystem`の操作一覧（stat、read、random access read、atomic write、stream write、mkdir、rename、remove、walk、permission、realpath）に含めていない。** 残る4値は既存操作だけで確かめられる。

| capability | probe | 使うport操作 |
|---|---|---|
| `atomic-replace` | 既存fileを長さの違う内容へ置換し、読み戻す | `AtomicWrite`／`ReadFile` |
| `directory-rename` | **中身のある**directoryをrenameし、中身が付いてくるか見る | `MkdirAll`／`AtomicWrite`／`Rename`／`Stat` |
| `file-identity` | 別fileのcanonical pathが異なり、同じfileが安定して解決されるか見る | `RealPath` |
| `owner-enforcement` | 対象directoryの所有者が現在userか見る | `UserLookup.Current`／`OwnerOf` |

これは§4「効果がすべて既存portの背後へ閉じているorchestrationはportにしない」に当たる。archive展開と同じ形であり、**portにすると検査自体を差し替えられ、testで確かめられなくなる**。

### 1.2 `shim_strategy`はclientへ実際に張れるかで決まる

§3.3「**同一volumeで安全にrootを導出できるとき**はclientへのhardlinkを優先し」。この条件は「shim directoryからclient pathへhardlinkを張れるか」そのものである。

**volume名を比べて推測しない。** 同じvolumeでもmount point、ACL、filesystem機能で結果が変わる。shim directoryからclient pathへ実際に張って確かめ、消す。

`fallback-resolver`はP7-02着手時の利用者判断でP11-04へ回しているため、張れなければ§16「必須capabilityを確認できない場合はPlanを作らず`E_PLATFORM_UNSUPPORTED`にする」に従う。

## 2. 分割

P7-03は§7の書込み6段階と逆順rollback、§16の`SetupPlan` 15 fieldに及ぶ。1本に収まらないため**2 PRへ分割**する。

| 本 | 内容 |
|---|---|
| 1（本PR） | §17.1のfilesystem capability probeとlink方式の決定 |
| 2 | setup transaction（書込み6段階、逆順rollback）、冪等性、`--remove` |

1本目を先にするのは、2本目のtransactionがcapabilityと方式の確定を前提にするためである（§16「必須capabilityを確認できない場合は**Planを作らず**」）。

## 3. 判断

### 3.1 filesystem種別名から推測しない

§3.1「必須能力が欠ける場合は**setup probe**で理由付き拒否する」。ReFS/FAT/network shareで何が欠けるかは種別名からは決まらず、権限やmount optionでも変わる。P7-01の`LinkManager.Capabilities`と同じ方針である。

### 3.2 「能力が無い」と「確かめられなかった」を混ぜない

前者はfilesystemを変えれば解決し、後者は権限やdiskの問題である。**利用者が取る行動が違う。**

`UserLookup`が失敗した場合は`ErrCapabilityProbe`を返し、「owner-enforcementが無い」とは報告しない。shim directoryを作れない場合は`E_FILESYSTEM`（`Retryable`）とし、`E_PLATFORM_UNSUPPORTED`（`Retryable`でない）と分ける。

### 3.3 所有者を確かめられないことを「一致した」と見なさない

FAT等では`OwnerOf`が空を返す。所有を根拠にできない状態で`owner-enforcement`を報告すると、**他userが書けるrootをsetupが受け入れる**（§2.3「他user所有、world-writable parentを拒否する」）。

### 3.4 `directory-rename`を空のdirectoryで確かめない

§7手順7のcommitがversion directoryのatomic renameに依存する。空だけ通るfilesystemがあるため、中身を入れてrenameし、中身が付いてきたことまで確かめる。

### 3.5 `atomic-replace`を長さの違う内容で確かめる

同じ長さだと、部分書込みでも成功して見えることがある。§4のatomic writeは「中断時にpathが旧内容のまま残るか、まったく存在しないかのどちらか」を要求しており、置換が丸ごと反映されることが前提能力である。

### 3.6 実測できた能力をそのまま`SetupPlan`へ載せない

§16「**hardlinkを使う場合だけ**capabilityへ`hardlink`を含める」。必須集合＋実際に使う方式だけを載せる。

そのまま載せると、`filesystem_capabilities`が「setupが依存する能力」ではなく「たまたま使えた能力」の一覧になる。Planは利用者が承認する対象であり、依存していないものを並べると何を確認すればよいか分からなくなる。

### 3.7 probeの後片付けに`RemoveAll`を使わない

junctionをdirectoryとして辿り、target側の内容を消しうる（§3.2「junction targetを再帰削除しない」）。`Walk`で拾って深い順に`Remove`する。列挙してから消すことで、途中失敗で残った実体も拾える。

### 3.8 足りないcapabilityを名指しする

§9「errorには対象logical path/value名、実行しなかった変更、**利用者が選べる安全な代替**を含める」。「対象外」だけでは利用者が何を直せばよいか分からない。

## 4. 検査が固定したこと

`internal/shell`で31 caseを追加した。

| 検査 | 対象 |
|---|---|
| `TestProbeReportsAllCapabilitiesOnCapableFilesystem` | 7値が揃うこと、**ASCII byte順**であること |
| `TestProbeLeavesNothingBehind` | probeの残骸が無いこと、実行ごとに結果が変わらないこと |
| `TestProbeReportsMissingLinkCapability` | 作れないlink種別を能力として報告しないこと |
| `TestProbeDoesNotAssumeOwnershipWithoutEvidence` | **所有者が空のときに「一致した」と見なさないこと** |
| `TestProbeReportsForeignOwnerAsMissing` | 他user所有を拒否すること |
| `TestProbeDistinguishesFailureFromMissingCapability` | **probe失敗と能力不足を混ぜないこと** |
| `TestCheckRequiredNamesMissingCapabilities` | 必須集合の判定4 case、**不足を名指しすること**、`Retryable=false` |
| `TestDecideUsesPlatformFixedStrategies` | §16のplatform別固定方式（両OS） |
| `TestDecideLeavesNoProbeBehind` | shim方式probeの残骸が無いこと |
| `TestDecideRejectsWhenLinkCannotBeCreated` | **張れないときに`E_PLATFORM_UNSUPPORTED`**（両OS） |
| `TestDecideReportsFilesystemErrorSeparately` | 書込み失敗を`E_FILESYSTEM`（`Retryable`）として分けること |
| `TestSelectCapabilitiesIncludesHardlinkOnlyWhenUsed` | §16の`hardlink`条件付き、使わない能力を載せないこと、ASCII byte順 |
| `TestSelectCapabilitiesRejectsMissingRequired` | 必須不足でPlanを作らないこと |
| `TestSelectCapabilitiesRejectsContradictoryHardlink` | 方式と能力が矛盾する組合せを止めること |
| `TestFallbackResolverIsNotSelected` | **どのplatformでも`fallback-resolver`を選ばないこと**（P11-04まで） |

### 4.1 変異test

8件入れ、いずれも検査が落ちた。生き残りは無い。

| 変異 | 結果 |
|---|---|
| 所有者が空でも`owner-enforcement`とする | 落ちた |
| 他user所有でも通す | 落ちた |
| probeの後片付けをしない | 落ちた |
| 必須capability検査を外す | 落ちた |
| shim方式のprobeを省く | 落ちた |
| 使わない`hardlink`もcapabilityへ載せる | 落ちた |
| 実測できた能力をそのまま載せる | 落ちた |
| `directory-rename`を空dirで確かめる | 落ちた |

## 5. 検証

Linux containerで実行した（Go 1.26.6）。両OSの判定はCI matrixで行う。

| command | 結果 |
|---|---|
| `gofmt -l .` / `go build ./...` / `GOOS=windows go build ./...` | 出力なし・成功 |
| `go vet ./...` / `GOOS=windows go vet ./...` | 成功 |
| `go test ./... -race -shuffle=on -covermode=atomic` | 全package成功。`internal/shell` 87.8% |
| `check_policy.py` / `check_imports.py` / `check_docs.py` / `check_licenses.py` / `check_messages.py` | すべて成功 |
| `git diff --check` | 出力なし |

`scripts/ci/check_imports.py`へ`internal/shell`のimport 4件を仕様根拠付きで追加し、`allowedGlobals`へ4件を登録した。

## 6. 未実施・制約

- **setup transactionを実装していない。** §7の書込み6段階（root/state初期化、shim生成、integration backup、integration変更、再読検証、setup state commit）、逆順rollback、冪等性、`--remove`は**2本目**の範囲である。
- **`SetupPlan`を組み立てていない。** 本PRが決めるのは§16の15 fieldのうち`filesystem_capabilities`／`current_link_strategy`／`shim_strategy`の3つである。残る12 field（mode、旧新root、shim path、integration方式と対象、backup、restart要否）は2本目で扱う。
- **integration（`user-path`／`shell-profile`）の内部を扱っていない。** Windows HKCU Pathのraw/type/上限/通知とLinux profileのmarker/escape/conflictは**P7-04**の範囲である。
- **probeを実filesystemで走らせていない。** fake portでの検査であり、ReFS/FAT/network shareで実際に何が欠けるかは確かめていない。**能力を過大に報告する側へは倒れない** —— probeは実際に作って読み戻せた場合だけtrueにする。実機確認は[11-quality-and-ci.md](../11-quality-and-ci.md)§9の利用者確認チェックリストとして扱う。
- **`file-identity`のprobeがinode/file indexを見ていない。** [port.FileInfo](../../internal/domain/port/filesystem.go)がそれらを持たないため、`RealPath`の区別と安定性で判定する。case foldingや8.3名の畳み込みは捕まえられるが、hardlinkで結ばれた別名を「同じfile」と判定する能力までは確かめていない。
- **P7-02から継続**: §10手順2〜6と`cmd/gdtvm`の分岐はP8-03／P8-04の範囲。`internal/shim`の`sameFile`は`port.FileInfo`の制約からsize・mode・mtimeで判定する。
- **P7-01から継続**: junction経路のsyscallは`windows-latest`だけが動かす（PR #145で確認済み）。`port.LinkCapabilities`は失敗理由を運べない。
- **P6-03から継続する未実装**: 合成側の`InstallEngine` adapter、`app.Guard`を噛ませた経路のE2E照合（§7.2）、receipt indexの再構築、`port.FileSystem`／`port.Environment`のproduction実装（いずれもP8-01）。
- **P6-02から継続する食い違いが1件**: `internal/store`のtemplate grammarが`internal/definition`と一致しない。fail closedは保たれ、正当なdefinitionからは生じない値である。§2の責務表を要する判断であり未着手。
- **P5-03から継続する未決が1件**（`./`始まりのtar entryを[10-security.md](../10-security.md)§5に従って拒否している）。**P6-01で埋めた仕様の空白が1件継続**（exact指定で`installable=false`のときの`E_PLATFORM_UNSUPPORTED`）。**仕様側の未決が2件継続**（[07-registry-and-tools.md](../07-registry-and-tools.md)§5第2項のlicense file size上限が§21の表に無い、§2の「license file名はASCII kebab grammar」が§2自身の例と食い違う）。`python.toml`の`lifecycle = "unknown"`はP3-04から継続。source error専用のerror code、version数値要素の64 bit上限、`logs/` file名規約と§11「専用lock」の解釈は未決である。
