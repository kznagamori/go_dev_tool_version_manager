# P3-01 決定記録（1/3）: definition基盤と§2〜§5.1

対象task: `docs/13-progress.md` P3-01（3分割の1本目）
規範仕様: [06-tool-definition.md](../06-tool-definition.md) §1〜§5.1・§13・§14、[04-storage-and-data.md](../04-storage-and-data.md) §21、[07-registry-and-tools.md](../07-registry-and-tools.md) §2・§5

## 1. 3分割の判断

P3-01の対象は[06-tool-definition.md](../06-tool-definition.md) §2〜§12の16 table・125 keyと、`registry/schemas/tool-definition-v1.json`である。

| 区分 | key数 | 主な条件付き規則 |
|---|---:|---|
| top-level / `[tool]` / `[[platforms]]` / `provider` | 28 | platform tupleの一致、`artifact_kind`別のprovider必須/禁止 |
| `version_source`（§6.1〜§6.6） | 49 | `kind` 3値ごとの許可/禁止key、`item_flatten_pointer`の1段制約と親公開日時継承、`lifecycle_map`の全件明示、`channel_pointer`のstring/boolean |
| `artifact` / `selector` / `checksum` | 17 | `source=template\|asset`、checksum 2 kindとalgorithm/hex長 |
| `storage` / `install` / `runtime` / `validation` | 31 | scopeとpurgeの組、`expect` 3値ごとの必須/禁止field、template rootの文脈差 |
| 合計 | **125** | |

P2-04（12形式・150 field超、3分割）と同規模のため、利用者判断で3 PRへ分割した。

| # | 範囲 | branch |
|---|---|---|
| 1（本PR） | 基盤（strict TOML decode、§13の診断契約、§3 identifier grammar）＋§2＋§4＋§5枠＋§5.1 | `claude/feature-p3-01-definition-skeleton` |
| 2 | §6 version source | `claude/feature-p3-01-version-source` |
| 3 | §7〜§12＋`registry/schemas/tool-definition-v1.json` | `claude/feature-p3-01-artifact-runtime` |

task IDはP3-01のままとし、台帳項目は3本目のmerge後に`[x]`とする。

## 2. 判断

### 2.1 `internal/store`のcodecを流用しない

`internal/store`は§7〜§19の永続表現、`internal/definition`はregistryが配る入力を扱う。両者が同じstrict decode関数を共有すると、片方の都合で受理範囲を変えたときにもう片方が黙って動く。[02-architecture.md](../02-architecture.md) §2も両者を別の論理領域として並べている。`internal/config`が同じ理由で自前のstrict decodeを持っており、その前例に合わせた。

重複するのはgo-tomlのdecoder設定とerror分解のおよそ30行である。

### 2.2 未実装節をrawのまま保持する

§6〜§11のtableは`RawTable`（`map[string]any`）として保持し、本PRでは**存在だけ**を検査する。

型を先に与えて中身を素通りさせると、未実装の検証を「通った」ように見せることになる。逆に存在検査を後回しにすると、必須tableの欠落を3本目まで検出できない。`[[platforms]]`の12 keyが全件必須である（§5）ことは本PRで固定し、中身の許可key・enum・上限は2本目・3本目が足す。

空tableのdecode結果はnil mapになるため、空の非nil mapへ正規化した。そのまま入れると「table自体が無い」と区別できない。

### 2.3 stable reason codeをmessage IDにする

§13は「errorはdefinition relative path、line/column、field path、stable reason codeを返す」と定めるが、reason codeの集合を列挙していない。別のcode体系を新設せず、[domain.MessageID](../02-architecture.md)をそのまま使う。

新しいenumを作ると、同じ失敗がcode表とmessage catalogの2か所で別々に管理される。P1-04でtyped error/progress/warningがmessage IDで表示文を引く仕組みを入れており、definition errorだけを別扱いにする理由が無い。codeは`definition.`で始まる15件を定数で持ち、grammar・prefix・重複をtestで固定した。

### 2.4 診断へrelative pathを載せる

[10-security.md](../10-security.md) §9.2は「user home、user名、hostname、SIDを含むabsolute path」を除去対象とする。definitionのpathはregistry rootからのrelative（`tools/node.toml`）であり、これらを含まない。roleだけでなくpathも診断へ出せる。絶対pathが混ざっていないことをtestで確かめている。

### 2.5 `domain.ToolID`へ§3のgrammarと上限を入れた

**P1-02から停止記録へ引き継いでいた「`ToolID`の長さ上限は仕様に規定が無い」は誤りだった。** [06-tool-definition.md](../06-tool-definition.md) §3が「tool ID/alias `^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`、1～64 byte」と明示している。

`domain.ToolID`は`^[a-z0-9]+(?:-[a-z0-9]+)*$`で上限無しだったため、§3に合わせて先頭を英字へ限り64 byte上限を入れた。

`internal/definition`側だけで検査する選択もあるが、registryへ登録されたdefinitionだけがtool IDを持ち込むため、§3はdomain値の不変条件そのものである。domain側を締めればCLI入力とstateから読んだ値も同じ検査を通る。既存の有効値（`go`, `node`, `python`, `dotnet-sdk`, `a1`, `a-1-b`）はすべて新grammarを満たし、他packageのtestに影響は無かった。

### 2.6 SPDX license listを同梱しない

§4の`license`と§5.1の`provider.license`は「SPDX expression」である。**検査するのは式のsyntaxだけで、SPDX license listへの登録有無は見ない。**

listをclientへ同梱するとregistryとは別の更新経路ができ、上流のlist更新で既存definitionが読めなくなる。実在確認は[07-registry-and-tools.md](../07-registry-and-tools.md) §5の項目9が、OSI承認かどうかを含めてregistry reviewで人手照合する契約になっている。

SPDX 2.3のexpression grammar（`AND`/`OR`/`WITH`、括弧、`+`接尾辞、`LicenseRef-`、`DocumentRef-...:LicenseRef-...`）を再帰下降parserで検査する。`LicenseRef-`を受けるのは、.NET SDKのWindows配布物のように独自EULAでSPDX識別子を持たないlicenseがあるためである（同§、`LicenseRef-dotnet-library`）。

同じ理由で、§5の`license_notice`は**message IDとしての妥当性だけ**を見て、宣言の要否は判定しない。「OSI承認OSS licenseでない配布物へ宣言する」の判定にlicense listが要るためで、この対応は同§5項目9とP4-01のcontract testが担う。

### 2.7 URLのquery stringを一律で拒否する

§5.1は「URLはHTTPS、credential/query secretなし」と定めるが、[10-security.md](../10-security.md) §9.2は「既知のtoken query key」の一覧を定めていない。列挙できない以上、通す判断も落とす判断も根拠を持てないため、同§13のfail closedに従ってquery全体を拒否した。

`internal/security`の`MaskURL`がmask側で同じ理由からquery値を種類によらず伏せており、その判断と対になる。標準4 toolのdefinitionはいずれもqueryを使わない。

**この判断は2本目で見直す可能性がある。** §6の`version_source.url`や§7のartifact templateがqueryを必要とするsourceを扱う場合、本PRの規則をそのまま適用してよいかを再確認する。

fragmentも拒否する。取得に影響せず、mask時に落ちるため、残っていても診断と実際の取得先がずれるだけである。

hostはASCII lowercaseへ限る。大文字やIDNのpunycode前表記を許すと、同じhostが別の文字列として§7.1のredirect許可listの比較を抜ける。

### 2.8 予約device名listを複製しない

§3は「Windows予約名」の拒否を求める。grammarは小文字だけを許すため`con`や`com1`は文法上は正しく、grammarでは表せない。[02-architecture.md](../02-architecture.md) §2が`internal/security`へpath検査を割り当てているため、P2-03で入れた`ValidateComponent`を呼ぶ。listをここへ複製すると、片方だけが更新される余地ができる。

`check_imports.py`へ`internal/definition` → `internal/domain`・`internal/security`を根拠付きで追記した。

### 2.9 tool IDとaliasを同じ一意検査に入れる

§4はaliasの一意性を明示していないが、tool IDと同じaliasを許すと解決の起点が2つになり、どちらが正規かを型で区別できなくなる。`domain.ToolID`が「aliasではなく正規IDだけを保持する」（P1-02）前提が崩れるため、自分自身のIDも衝突検査の対象へ入れた。

registry全体でのtool ID/alias衝突検査（§3）は全definitionを見る必要があるため**P4-01**の範囲である。

## 3. 検査が固定したこと

### 3.1 「全件必須」を機械的に確かめる

個別testでは§2・§4・§5・§5.1の全件必須を網羅できないため、§15の正規例からscalar keyを1行ずつ削除して全件が拒否されることを確かめる。削除対象の件数（18件）を定数で持ち、正規例が縮んで検査が空振りしたときに気付けるようにした。P2-04のreceipt/catalogで使った方式と同じである。

走査範囲は`[platforms.version_source]`より前の行だけとする。それ以降のtableは本PRでは存在検査だけを行い、中のkeyの必須性は2本目・3本目が決める。

### 3.2 主なnegative test

| 対象 | 内容 |
|---|---|
| §13-1 byte制約 | 5件。空、BOM付き、UTF-8でない、2 MiB超過、TOMLとして壊れている |
| §13-1 unknown/重複 | 7件。top-level/tool/platform/provider内のunknown key、重複key、重複table、型違い |
| §13-2 schema | 4件。schema 2、schema_id不一致、両者の欠落 |
| §3 identifier | 3 grammar × 各13件以上。空、uppercase、前後空白、先頭hyphen/数字、連続separator、ASCII以外、NUL、`.`/`..`、区切り、長さ上限の境界 |
| §3 予約名 | 6件（`con`〜`lpt1`）。拡張子付きも拒否し、`console`・`com0`・`com10`・`nula`は通す |
| §4 text | 5件。空、上限超過、前後空白 |
| §4 alias | 5件。空配列は通す、grammar違反、alias同士の重複、tool IDとの重複、16件上限 |
| §4 basename | `tools/nodejs.toml`に`id = "node"`を書いたfileを拒否 |
| §4/§5 enum | `version_scheme`と`artifact_kind` |
| §5 platform tuple | 4件。未知ID、os/arch/libcの不一致。Linux側のtupleが通ることも確認 |
| §5 platform件数 | 重複IDと3件目（§21の上限2） |
| §5 `license_notice` | message ID grammar違反 |
| §5.1 条件付きkey | 4件。officialの`adoption_reason`、third-partyの`repository`/`adoption_reason`欠落、officialの`repository`は任意 |
| §4/§5.1 URL | 10件。HTTP、scheme無し、userinfo、host無し、query、fragment、host大文字、host非ASCII、前後空白、8 KiB超過 |
| SPDX expression | 18件。演算子だけ/演算子で終わる/始まる、括弧の不整合、`WITH`の右辺欠落と演算子、idstring外の文字、非ASCII、`LicenseRef-`の本体空、`DocumentRef`のcolon欠落、小文字演算子、余分な語、上限超過 |
| §13 集約 | 4か所を同時に壊して4件が1回で出ること、100件で打ち切ること、上限ちょうどでtruncatedにならないこと |

### 3.3 検査が実装の欠陥を検出した

SPDX parserが`GPL-2.0-only WITH OR`を受理していた。`OR`がidstring grammar（`[A-Za-z0-9.-]+`）に一致するためで、`WITH`の右辺が式なのかIDなのか決まらない式が通っていた。演算子と括弧を先に弾くよう修正した。

global state検査が新規package変数5件を検出したため、根拠付きで許可表へ追加した（6回目）。

## 4. 検証

### 4.1 ローカル検証

Linux container（Go 1.26.5、Python 3.11.15）で実行した。

| command | 結果 |
|---|---|
| `gofmt -l .` | 0 file |
| `go vet ./...` | 成功 |
| `go test ./... -race -shuffle=on -covermode=atomic` | 成功。`internal/definition` 95.2% |
| `scripts/ci/check_policy.py` | 成功。production Go file 91件 |
| `scripts/ci/check_imports.py` | 成功。package 20件 / internal import 95件 |
| `scripts/ci/check_docs.py` | 成功。file 39件 |
| `scripts/ci/check_licenses.py` | 成功。module 14件 |
| `scripts/ci/check_pr_refs.py` | 成功（task-id=p3-01, slug=definition-skeleton） |
| `git diff --check` | 差分なし |

### 4.2 CI

PR #55（workflow run 31797209544）で、6 job×2 OSの **12 checkすべてがsuccess** になった。

## 5. 未実施・制約

- §6 version source（§13の検証順序5）は**2本目**、§7〜§12（同6〜10）と`registry/schemas/tool-definition-v1.json`は**3本目**の範囲である。本PRの`Platform`は該当tableを`RawTable`で保持し、存在検査だけを行う。
- registry全体のID/alias/command衝突と[07-registry-and-tools.md](../07-registry-and-tools.md) contract（§13の検証順序11）は**P4-01**の範囲である。
- `license_notice`の宣言要否と`provider.license`のOSI承認判定は、SPDX license listを同梱しない判断（§2.6）によりregistry reviewとP4-01のcontract testが担う。
- URLのquery拒否（§2.7）は`version_source.url`とartifact templateへ同じ規則を適用してよいかを**2本目の着手時に再確認**する。
- version grammarの境界test（`semver`/`go`/`python`のcomparison、channel/lifecycle）は**P3-02**の範囲である。本PRは`version_scheme`をenum 3値として受けるところまでとし、scheme依存のversion検査（§6.4 override、§6.6 static version）は2本目で扱う。
- `go tool govulncheck ./...` はこのcontainerから`vuln.go.dev`が`Forbidden`で到達できず、ローカルでは未実行である（P0-02から継続）。本PRは新規依存を追加していない。
- Windowsでの実行はCI matrixでのみ確認する。
