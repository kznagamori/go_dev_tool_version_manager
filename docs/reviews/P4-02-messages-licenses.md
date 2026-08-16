# P4-02 決定記録（1/2）: message catalogとlicense text

対象タスク: `docs/13-progress.md` P4-02の1本目。規範仕様は[04-storage-and-data.md](../04-storage-and-data.md)§20（message catalog）・§7（message ID／parameter key grammar）・§21（上限）、[07-registry-and-tools.md](../07-registry-and-tools.md)§2（exact tree）・§5（source validation第2・7項）、[10-security.md](../10-security.md)§9.2（mask対象）。

## 1. 分割の理由

P4-01の停止記録が残した確認事項（§2のexact treeへ不足する2 fileの作成と`CheckTree`完全一致への切替え）を着手時に実測した。

| 構成要素 | 実測 |
|---|---|
| §5 source validation | 10項目。tree・digest・両platform version集合はP4-01と既存contract testで一部済み |
| message ID | production GoとregistryのTOMLが出しうるIDが**85件** |
| license text | upstream `raw.githubusercontent.com`が到達可能。MPL-2.0本文373行 |

規模がP3-01・P3-03・P3-04と同等のため、利用者判断で**2 PRへ分割**した。1本目がdata file 2件と§20 parser、2本目が§5の残る検査項目である。§5第1項（tree一致）と第7項（license text存在）は対象fileが無いと検査自体を書けないため、data先行・validator後追いが依存順に沿う。

## 2. 判断

### 2.1 message IDの件数を推測せず、sourceから数えた

着手時の粗い走査は199件を返したが、これは`artifact.digest`や`summary.provider_kind`のような**codec診断のfield path**を含んでいた。`internal/store`や`internal/definition`はfield pathを同じdotted形式で書くため、文字列の形だけでは区別できない。

message IDになる経路を辿って数え直し、**85件**を確定した。

| 経路 | 件数 |
|---|---:|
| `internal/config`のtyped error helper（`configError`ほか7種と`requireSchema`） | 48 |
| `internal/definition`の§13 stable reason code＋`definition.invalid` | 28 |
| `internal/catalog`の`sourceError`／`messageID` | 4 |
| `domain.MessageIDInternal`／`progress.MessageIDCancelled` | 2 |
| `internal/registry`の`registry.invalid` | 1 |
| registry TOMLの`license_notice`／`adoption_reason` | 2 |

§13のreason codeをcatalogへ載せたのは、`Diagnostic.Reason`が`domain.MessageID`型で、実装のコメントが「message IDをそのまま使う。別のcode体系を作ると、同じ失敗がcode表とmessage catalogの2か所で別々に管理されることになる」と明記しているためである。

### 2.2 網羅検査をGo testではなくCI scriptへ置いた

message IDはpackageをまたいで散らばる。`internal/registry`から`internal/config`や`internal/definition`をimportして集めることは[02-architecture.md](../02-architecture.md)§1の依存方向が許さない。

そこで`scripts/ci/check_messages.py`をsource全体の走査として`policy` jobへ足した。検査は**両方向**である。

1. production GoとregistryのTOMLが出しうるIDが、すべてcatalogにある。欠けていると、その失敗が起きたときに表示するtextが無い。
2. catalogのkeyが、すべてどこかから参照されている。CLAUDE.md §7の「未使用のenum値、kind、fieldを『将来のため』に残さない」をcatalogへも適用した。

Go側には件数の定数（`MessageCount = 85`）と分類集合・並びの検査だけを置き、網羅そのものはscriptに任せた。件数定数は、IDを増減させたときにcatalogの更新漏れへ気付くためのものである。

scriptは実際に3種の失敗（key削除、未参照key追加、grammar違反key）を検出することを手元で確認した。

### 2.3 TOMLのdotted keyを平坦化した

TOMLの`error.internal = "x"`は**dotted key**であり、decodeすると`error`の下に`internal`を持つtableになる。§20のmessage IDはdotted keyそのものなので、decode後に`a.b`へ平坦化して戻す。leafがstringでなければerrorにする（catalogの値はtemplate stringだけである）。

宣言順はsourceの行走査で読む。go-tomlはmapへdecodeするとkey順を失い、診断を宣言順で出せないとどのentryを直すかがfile上で辿りにくい。行走査の結果とdecode結果の**key集合が一致しない場合はerror**にした。`[error]`のtable記法で書かれると両者が食い違うため、その検出を兼ねる。

multi-line literal stringの本文に`<key> = ...`の形が現れても2件目の宣言と数えない。数えると、実際の失敗（templateに改行がある）ではなく「宣言順を読めないkey」という無関係な診断が出る。

### 2.4 「秘密値展開の禁止」をmask規則から導いた

§20は「template内ANSI、terminal control、秘密値展開を禁止する」と定めるが、何が秘密値かは§20に無い。[10-security.md](../10-security.md)§9.2がmask対象（`*_TOKEN`／`*_PASSWORD`／`*_SECRET`／`*_KEY`、HTTP authorization/cookie/proxy header）を定めているため、`internal/security`の判定をそのまま使い、その名前のplaceholderを拒否した。

同じpatternをregistry側へ複製しなかったのは、mask規則を変えたときに片方だけ古いままになるためである。`check_imports.py`へ`internal/registry`→`internal/security`を根拠付きで追加した。

### 2.5 制御文字は改行とtabも拒否した

§20の「terminal control」はC0制御文字とDELを指すが、改行とtabも拒否した。catalogの値は1行のtemplateであり、幅計算と折返しは表示側が行う。catalogが改行を持つと、table表示とJSON envelopeで同じmessageが別のlayoutになる。

Unicodeのformat文字（`unicode.Cf`、U+200E LEFT-TO-RIGHT MARKなど）も拒否した。表示上の文字順を変えてtextの意味を偽装できるためである。

### 2.6 license textはupstreamから取得し、内容をtestで照合した

§2は「内容はupstream取得元とlicense identifierをregistry reviewで照合する」と定める。`https://raw.githubusercontent.com/astral-sh/python-build-standalone/main/LICENSE`から取得した本文（373行・16,725 byte・SHA-256 `1f256eca…f1d5`）をそのまま置いた。

「registry review」を機械的に再現できる形にするため、testで題名（`Mozilla Public License Version 2.0`）と本文10節（§1〜§5、§8〜§10、Exhibit A/B）の存在を照合した。題名だけの要約やdiffで欠けた本文を通さないためである。あわせてASCII・LFのみ・BOMなし・末尾LFを固定した。registryは両OSへ同じbytesで配られるため、CRLFやBOMが混ざるとplatformごとにbytesが変わる。

`python.toml`のproviderは`MPL-2.0`を宣言しており、別のlicense本文へ差し替わるとPlanが表示するlicenseと同梱本文が食い違う。

## 3. 仕様で一意に決まらなかった点

### 3.1 license fileのsize上限

§5第2項は「registry TOML、definition TOML、message/licenseがsize上限内」と定めるが、§21の表にlicense fileの行が無い。§21の「registry manifest各file 2 MiB」をregistry treeのfileへ適用する読みを採り、2 MiBとした（実file 16,725 byteで余裕がある）。**§21へlicense fileの行を足すか、この読みを明記するかは未決である。**

### 3.2 §2の「license file名はASCII kebab grammar」

§2が定める唯一のlicense file名`python-build-standalone-MPL-2.0.txt`は、大文字（`MPL`）とdot（`2.0`）を含み、repositoryが他所で使うkebab grammar `^[a-z][a-z0-9]*(-[a-z0-9]+)*$` に一致しない。**仕様の本文と自身の例が食い違っている。**

grammar検査を実装すると§2が指定したfile名を拒否することになるため、実装していない。§2のexact treeがfile名を1件に固定しており、`CheckTree`が完全一致で検査するため、v0.1では追加のgrammar検査に検出力が無い。**§2の文言修正（識別子部分の大文字とversion番号を許す旨）か、grammarの明示が必要である。**

## 4. 検査が固定したこと

| 検査 | 対象 |
|---|---|
| `TestParseMessageCatalogAcceptsSpecShape` | §20の形が通り、宣言順が保たれること |
| `TestMessageCatalogPlaceholders` | 出現順・重複なし、`{{`/`}}`がliteral braceであること |
| `TestParseMessageCatalogRejects` | ID grammar 6件、値の契約2件、placeholder構文4件、placeholder名3件、秘密値6件、制御文字7件、multi-line 1件、TOML破損4件（計33件） |
| `TestParseMessageCatalogRejectsTableSyntax` | `[error]`記法を拒否すること |
| `TestParseMessageCatalogSizeLimits` | 1 message 8 KiBちょうど/超過、file全体2 MiB超過 |
| `TestParseMessageCatalogRejectsInvalidUTF8` | 不正なUTF-8 |
| `TestRepositoryMessageCatalog` | 実catalogが85件で、分類が7種であること |
| `TestRepositoryMessageCatalogIsSorted` | 分類ごとに固まり、分類内がID順であること |
| `TestRepositoryPythonLicenseExists` / `Identity` / `Encoding` | 存在・size、MPL-2.0の題名と10節、ASCII/LF/BOMなし |
| `TestRepositoryRegistryMatchesExactTree` | §2のexact treeと**完全一致**（P4-01の余分検出のみから切替え） |
| `scripts/ci/check_messages.py` | source全体の網羅と未使用なし（`policy` jobの両OS） |

## 5. 検証

すべてLinux containerで実行した。

| command | 結果 |
|---|---|
| `gofmt -l .` | 出力なし |
| `go build ./...` / `go vet ./...` | 成功 |
| `go test ./... -race -shuffle=on -covermode=atomic` | 全package成功。`internal/registry` 99.6% |
| `python3 scripts/ci/check_messages.py` | 成功（catalog key 85／source参照ID 85） |
| `check_policy.py` / `check_imports.py` / `check_docs.py` / `check_licenses.py` | すべて成功 |
| `check_pr_refs.py` / `git diff --check` | 成功・出力なし |

`internal/registry`で覆えていないのは`describeDecodeError`の最終fallback 1行だけである（P4-01から継続。libraryの契約が変わった場合の防御）。

## 6. 未実施・制約

- **§5のsource validation 10項目のうち、2本目の範囲は未実装である。** alias衝突（第4項）、platform tuple（第5項）、§7〜§10表の一致（第6項）、OSI license対応（第9項）、`lifecycle_map`網羅（第10項）と`ValidateSource`の組立てが残る。
- **message catalogのrenderは実装していない。** placeholderの走査と検査までで、`{name}`を実parameterで置換する表示側はP8-05（human表示とJSON envelope）の範囲である。`Placeholders`はそのための入力として公開した。
- **catalogのplaceholderと、code側が実際に渡すparameterの一致は検査していない。** 例えば`{path}`を持つmessageに対して`path`を設定しないcodeがあっても検出できない。parameter集合の抽出はhelperごとの呼出し形に依存し、静的走査では取りこぼす。P8-05でrender経路が入った時点でgolden testとして扱うのが確実である。
- §3.1のlicense file size上限と§3.2のfile名grammarは仕様側の未決事項である。
- `python.toml`の`lifecycle = "unknown"`はP3-04から継続。release前に`devguide.python.org`へ到達できる環境で確認する。
- 言語追加時のkey/parameter集合一致（§20）は、catalog fileが1件しかないため検査対象が無い。
