# P3-01 決定記録（2/3）: §6 version source

対象task: `docs/13-progress.md` P3-01（3分割の2本目）
規範仕様: [06-tool-definition.md](../06-tool-definition.md) §6.1〜§6.6・§13、[04-storage-and-data.md](../04-storage-and-data.md) §15・§21、[10-security.md](../10-security.md) §9.2・§13

## 1. 着手時に見つけた仕様の矛盾

### 1.1 `json`と`lifecycle_pointer`（利用者判断で解決）

§6.1の表は`kind="json"`へ`lifecycle_map`を禁じる。一方で同§6.1の本文は「`lifecycle_map`は`document_lifecycle_pointer`**または`lifecycle_pointer`**と組で使う」とし、§6.3は「`lifecycle_pointer`が読んだ値を`lifecycle_map`で写像した結果。**mapに無い値はsource error**」と定める。

つまり`json`が`lifecycle_pointer`を許して`lifecycle_map`を禁じると、**その組合せは必ずsource errorになる**定義がschema検証を通ってしまう。写像先が1件も無いためである。

利用者判断により**`json`は両方を禁止**し、lifecycle pointerを`json-index`専用にした。仕様を同じ変更で修正している。

| 修正箇所 | 内容 |
|---|---|
| §6.1の表 | `json`の禁止listへ`lifecycle_pointer`を追記 |
| §6.1の`lifecycle_map`説明 | 「どちらか一方を宣言したら必須」を明記 |
| §6.3の優先順位2 | 「どちらのpointerも`json-index`でだけ使え、`lifecycle_map`と組でなければ宣言できない」と理由を追記。3へ「`json` sourceのlifecycleは1と3だけで決まる」を追記 |

なお`lifecycle_pointer`は標準4 tool（Node.js／Go／Python／.NET SDK）のいずれも使っていない。

### 1.2 URLのquery stringは1本目の判断のままにできない

1本目は参照URLのqueryを一律拒否し、決定記録へ「2本目の着手時に`version_source.url`へ同じ規則を適用してよいか再確認する」と残していた。**確認した結果、適用できないことが分かった。**

§16.2のGoが`https://go.dev/dl/?mode=json&include=all`を正規例に持つ。queryがAPI契約の一部であり、拒否するとGoのdefinitionが書けない。

用途で分けた。判断の根拠が違うため、1つの規則にまとめない。

| kind | 対象 | query | 根拠 |
|---|---|---|---|
| `urlReference` | `tool.homepage`, `provider.homepage`, `provider.repository`, `evidence` | 拒否 | §5.1「credential/query secretなし」。[10-security.md](../10-security.md) §9.2が「既知のtoken query key」の一覧を定めないため、§13のfail closedで全体を拒否する |
| `urlEndpoint` | `version_source.url`, static assetの`url`/`release_url` | 許可 | §16.2のGoが正規例に持つ |

userinfoとfragmentはkindによらず拒否する。userinfoは[10-security.md](../10-security.md) §11.1が明示的に禁じ、fragmentは取得に影響せずmask時に落ちるため、残っていても診断と実際の取得先がずれるだけである。

Goのquery付きURLがendpointで通り参照で落ちることをtestで固定した。

## 2. 判断

### 2.1 kindごとの禁止keyを表で持つ

§6.1の表を`forbiddenSourceKeys`の集合として持つ。条件分岐で書くと、kindが増減したときにどのkeyが漏れたかを読み取れない。1件でも違反があれば以降の個別契約を検査しない。kindの前提が崩れた状態で見ても診断が増えるだけで原因が分かりにくくなるためである。

`static`の禁止keyには**`version_regex`を含めた**。§6.1の文面は「他pointer/url/index/cache fieldを禁止する」であり`version_regex`はそのどれでもないが、同§が「`static_versions` arrayと`max_items`**だけ**を使用し」と書いており、static versionは正規完全versionを直接書くためregexの適用対象が無い。§6.4もoverrideを明示的に禁じている。

### 2.2 必須keyを契約から導く

§6.1は許可keyだけを列挙し、必須keyを明示していない。実際、Node.jsは`channel_pointer`を、Goは`published_at_pointer`を持たない。契約から導ける必須keyを次のとおり固定した。

| kind | 必須key |
|---|---|
| `json` | `kind`, `url`, `items_pointer`, `version_pointer`, `version_regex`, `max_items`, `cache_ttl` |
| `json-index` | 上記＋`index_items_pointer`, `index_document_pointer`, `max_documents` |
| `static` | `kind`, `max_items`, `static_versions` |

**`cache_ttl`の必須性は[04-storage-and-data.md](../04-storage-and-data.md) §15から導いた。** 同§が「static sourceは`expires_at=null`を許す」と定める。裏を返せばnetwork sourceのcatalogは必ず期限を持つため、`cache_ttl`は`json`と`json-index`で必須である。標準4 toolの正規例もすべて宣言している。

### 2.3 pointerはgrammarだけを検査する

§6.1の「pointerはすべてRFC 6901」に対し、**解決の意味づけを検査しない。** どのnodeを指すか、配列かどうかは上流文書の形に依存し、definitionだけでは決まらない。取得経路はP3-03の範囲である。

この分離により、Node.jsの`items_pointer = "/"`をそのまま受ける。RFC 6901では`/`は「空文字keyのmember」であり文書全体ではないが、syntaxとしては正当である。実際にどこを指すかの解釈はP3-03が扱う。

### 2.4 `cache_ttl`の範囲を導入した

仕様は範囲を定めていない。0以下を許すとrefreshのたびに上流を叩き、極端に長い値はEOL情報の更新を止める。[05-configuration.md](../05-configuration.md) §3.4のnetwork durationが「Go duration grammarの正値」であることに合わせ、1分〜30日とした。標準4 toolはいずれも`24h`である。

**この範囲は仕様に無く、本PRで導入した規約である。**

### 2.5 pointerとregexの長さ上限を導入した

仕様は個別の上限を定めていない。上限が無いと、definitionから解析費用を膨らませられる。pointerは[04-storage-and-data.md](../04-storage-and-data.md) §21のpath component上限と同じ255 byte、regexは1024 byteとした。標準4 toolのregexはいずれも200 byte未満である。

**これも本PRで導入した規約である。**

### 2.6 version schemeを`[tool]`からplatformへ渡す

§6.4の`lifecycle_overrides[].version`と§6.6の`static_versions[].version`は「正規完全version」である。schemeは`[tool].version_scheme`が持つため、`[tool]`を先に読んでから`buildPlatforms`へ渡す。

1本目の決定記録では「scheme依存のversion検査は2本目で扱う」としており、そのとおり実装した。schemeが決まっていない場合（`[tool]`側の診断が既に出ている場合）はversion検査で追加の診断を出さない。同じ原因で2件のerrorを出すと、どちらを直せばよいかが読み取れない。

### 2.7 asset側のos/arch/libcをplatform entryと突き合わせない

static assetは`os`/`arch`/`libc`を持つが、platform entryとの一致を検査しない。§7.1のselectorが`os`/`arch`/`libc`で絞り込む契約であり、asset listに他platform向けのentryが含まれてよいためである。selectorとの整合はP3-01の3本目が扱う。

### 2.8 `unknown`へのlifecycle overrideを拒否する

§6.4は`status`を`supported|eol`の2値へ限る。`unknown`は「判断していない」であり、根拠（`evidence`と`assessed_at`）を添えて上書きする対象にならない。§6.6のstatic versionは`unknown`を取れるが、こちらは「不明と判断した調査根拠」を残す契約であり別物である。

## 3. 検査が固定したこと

### 3.1 4 toolの正規例をすべて通す

§16の4 tool分のversion sourceをそのままfixtureにし、`json`（Node.js／Go）、`json-index`（.NET SDK）、`static`（Python）が通ることを確かめる。1つのkindだけで通る実装になっていないことの担保である。

### 3.2 kindごとの禁止keyを1組ずつ

`forbiddenSourceKeys`の表を走査し、scalar keyの禁止組合せ22件を機械的に検査する。table形式の4件（`lifecycle_map`, `asset_fields`, `metadata_fields`, `lifecycle_overrides`）と`static_versions`は行の差し替えで表現できないため別testに置いた。

### 3.3 主なnegative test

| 対象 | 内容 |
|---|---|
| §6.1 kind別禁止key | scalar 22組＋table 4組＋`static_versions` 2組 |
| §6.1 必須key | kind別に7／10／3件を1件ずつ落として拒否 |
| RFC 6901 pointer | 6件。`/`で始まらない、`~`で終わる、`~2`、`~x`、上限超過、UTF-8でない。空・`/`・`//`・escape・非ASCIIは通す |
| §6.3 `version_regex` | 6件。named captureなし／名前違い／2件／RE2不正／空／上限超過 |
| §6.1 `cache_ttl` | 7件。0、負、下限未満、上限超過、単位なし、空白入り、未知単位 |
| §6.1 上限 | `max_items` 0/負/10,001、`max_documents` 0/負/33 |
| §6.1 lifecycle組契約 | 6件。pointerだけ、mapだけ、両pointer同時、enum外、空table。item側pointer＋mapは通す |
| §6.1 flatten組契約 | 3件。親pointerのみ、親と子の同時指定。flattenだけは通す |
| §6.2 `required_tokens` | 7件。片方だけ2件、空配列、重複、空文字、非ASCII、空白入り。両方欠落は通す |
| §6.5 `asset_fields` | 3件＋13値のparse。集合外key、pointerでない値、空table |
| §6.5 `metadata_fields` | 4件。grammar外key、hyphen入りkey、pointerでない値、空table |
| §6.4 override | 11件。4 keyの欠落、`unknown`、enum外、部分版/range/leading v、HTTP evidence、非UTC。重複versionも拒否 |
| §6.6 static version | 10件の欠落・enum・version・時刻＋channel/lifecycle全6組合せの受理 |
| §6.6 static asset | 17件。size 0/負、algorithm enum外、hex長不一致、大文字hex、短いhex、prefix付き、os/arch/libc enum外、ID負/leading zero/非数値、HTTP URL、区切り入りname、非UTC/形式外の時刻 |
| §6.6 asset全件必須 | 13 keyを1件ずつ落として拒否（件数を`AssetFieldCount`と突き合わせ） |
| §6.6 platform間一致 | 3件。同じ集合は通し、値違いと件数違いを拒否 |
| URL kind | Goのquery付きURLがendpointで通り参照で落ちる。userinfo/fragmentは両方で落ちる |

### 3.4 global state検査が5件を検出

`lowerHexRe`、`decimalIDRe`、`digestHexLength`、`assetFieldOrder`、`sourceKeyOrder`を根拠付きで許可表へ追加した（7回目）。

## 4. 検証

### 4.1 ローカル検証

Linux container（Go 1.26.5、Python 3.11.15）で実行した。

| command | 結果 |
|---|---|
| `gofmt -l .` | 0 file |
| `go vet ./...` | 成功 |
| `go test ./... -race -shuffle=on -covermode=atomic` | 成功。`internal/definition` 94.8%、test 321件 |
| `scripts/ci/check_policy.py` | 成功。production Go file 96件 |
| `scripts/ci/check_imports.py` | 成功。package 20件 / internal import 99件 |
| `scripts/ci/check_docs.py` | 成功。file 40件 |
| `scripts/ci/check_licenses.py` | 成功。module 14件 |
| `scripts/ci/check_pr_refs.py` | 成功（task-id=p3-01, slug=version-source） |
| `git diff --check` | 差分なし |

### 4.2 CI

PR #58で、6 job×2 OSの **12 checkすべてがsuccess** になった。

## 5. 未実施・制約

- §7 artifact／§8 storage／§9 install／§10 runtime／§11 validation／§12 templateと`registry/schemas/tool-definition-v1.json`は**3本目**の範囲である。本PRの`Platform`は該当tableを`RawTable`で保持し、存在検査だけを行う。
- version sourceの**取得と評価**（index 2段取得、部分catalog禁止、`item_flatten_pointer`の1段展開、親公開日時の継承、`channel_pointer`のstring/boolean写像、pointerの解決）は**P3-03**の範囲である。本PRはdefinitionのschema検証までとし、上流文書の形に依存する判定を持たない。
- version grammarの境界test（comparison、`22.9.0`と`22.18.0`の順序）は**P3-02**の範囲である。本PRは`domain.ParseVersion`をそのまま使う。
- `cache_ttl`の範囲（§2.4）、pointerとregexの長さ上限（§2.5）は**仕様に無く本PRで導入した規約**である。仕様へ昇格させるか変更するかは利用者判断である。
- asset側のos/arch/libcと§7.1 selectorの整合は**3本目**、registry全体のID/alias/command衝突は**P4-01**の範囲である。
- `go tool govulncheck ./...` はこのcontainerから`vuln.go.dev`が`Forbidden`で到達できず、ローカルでは未実行である（P0-02から継続）。本PRは新規依存を追加していない。
- Windowsでの実行はCI matrixでのみ確認する。
