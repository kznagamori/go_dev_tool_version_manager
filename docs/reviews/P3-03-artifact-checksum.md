# P3-03 決定記録（3/3）: asset・artifact・checksum・catalog組立て

対象タスク: `docs/13-progress.md` P3-03（3分割の3本目、これでP3-03完了）。規範仕様は[06-tool-definition.md](../06-tool-definition.md)§6.2・§6.5・§6.6・§7.1・§7.2、[04-storage-and-data.md](../04-storage-and-data.md)§15・§21。

## 1. 着手時に見つけた仕様の穴（利用者判断で解決）

2本目の停止記録が残した確認事項である。§15は「lifecycle evidenceは公式/providerのHTTPS URL、assessmentはUTC RFC 3339で**全状態に必須**。source fieldならsource URL/fetch時刻、override/staticならdefinition記録を使う」と定めるが、**優先順位3の既定`unknown`**（source lifecycle fieldもoverrideも無い状態）の記録元を挙げていなかった。

§6.1の利用者判断で`json` sourceはlifecycle pointerを持てないため、Go／Node.jsの**overrideを書いていない全version**がこの状態になる。Node.jsは公開versionが数百件あり、全件へoverrideを書く運用は現実的でない。

利用者判断で**source URL＋fetch時刻を使う**ことにし、§15へ「上流がlifecycleを示さず既定の`unknown`になったitemもsource URL/fetch時刻を使う。『この公式sourceをこの時刻に取得した時点でlifecycle情報が公開されていなかった』ことを記録する。取得元を持たない根拠不明のitemを作らない」を明記した。schema変更もJSON schema変更も不要である。

## 2. 実装中に見つけた欠陥

### 2.1 unavailable itemを表現できなかった（修正した）

P2-04の`internal/store`は`artifact_file`・`artifact_url`・`artifact_digest`を**常に**非空として検証していた。ところが§7.1は「selectorに0件一致したversionは`installable=false/artifact-not-found`」、§6.2は「required tokenが1件でもないversion itemは`installable=false/artifact-not-found`」と定める。そのitemにはartifactが存在せず、file名もURLもdigestも書けない。

§15はこの3 fieldへ非空を要求していない（要求しているのは「key集合が例のとおり」であり、`unavailable_reason`の空/非空とdigest未解決itemをinstallableにしないことだけである）。P2-04の検証は仕様より強く、**仕様が要求する状態を表現できなくしていた**。

`installable=true`のときだけ必須とし、`installable=false`では空を許すよう直した。keyは常に存在し、値だけが空になる。installable itemの検証は従来どおり強いままである。

### 2.2 §16.2のGo例ではartifact URLを決められない（P3-04へ送る）

§16.2のGoは`source = "asset"`・`url = ""`だが、`asset_fields`に`url`を宣言していない。Goの`https://go.dev/dl/?mode=json`の`files[]`にもURL fieldは無い。§7.1の「assetはversion itemのassetsからselectorでexactly 1件選ぶ」に従うと、選んだassetからdownload URLを取り出せない。

本PRの実装は`asset_fields.url`を宣言したsourceで正しく動く。**registry定義側の問題**であり、標準4 toolのdefinition TOMLを作る**P3-04**で解決する（`asset_fields`へ`url`を足すか、`source = "template"`＋`url = "https://go.dev/dl/{{asset.name}}"`＋`checksum.kind = "asset-field"`にするか）。本PRのtest fixtureは`url`を宣言する形にした。

## 3. 判断

### 3.1 `unavailable_reason`のmessage ID

§6.2・§7.1は概念名を`artifact-not-found`と書くが、§15の`unavailable_reason`は**message ID**であり、その grammar（`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$`、§7）はhyphenを許さない。同じ概念を`catalog.artifact_not_found`へ写した。

### 3.2 asset fieldの型を暗黙変換しない

§6.5が「値はstring、sizeだけ非負integer。IDもprecision lossを避けるためdecimal stringとして扱う」と定める。数値をstringへ、stringを数値へ変換しない。上流が型を変えたらsource errorにしてlive smokeで気付く。

### 3.3 URL componentをpercent encodeする

§7.1が「URL componentは値をpercent encode、fileはbasename grammarを検査する」と定める。値に`/`が入るとrender後のURLが別のpathを指しうるため、`url.PathEscape`で閉じる。file名は逆にescapeせず、path区切り・`.`・`..`・NUL・255 byte超を拒否する。artifact fileはdownload先のbasenameになるため、区切りを含む値を通すと`payload`の外へ書ける。

### 3.4 checksum text fileはURLのbasenameで引く

file名templateと配布物の名前が違うsourceがあるため、checksum fileの行は**artifact URLのbasename**で照合する。同じbasenameの行が2件あればどちらが正しいか決められないためsource errorにする。同一URLのchecksum textは1回だけ取得する。

### 3.5 sizeはtemplateで使わない

§6.5で唯一integerのfieldであり、文字列化の表記をtemplate側で決めるとcatalogとreceiptで揺れる。`{{asset.size}}`は拒否する。

### 3.6 unavailable itemではchecksumを取得しない

必要tokenが無い、またはselectorが0件一致したitemはartifactを持たないため、checksum取得へ進まない。unavailableなversionのために上流へ要求を出さない。

## 4. 検査が固定したこと

### 4.1 asset・token

- §6.5の13 field解決、`size`の非負integer、型違い（`name`が数値、`size`がstring/負）、`assets_pointer`が配列でない、参照fieldの欠落、未知の`digest_algorithm`。
- `required_tokens`の充足／不足／pointer欠落（source error）／非string要素／重複／未宣言。**不足はsource errorでなくunavailable**である。

### 4.2 artifact

- selectorが**exactly 1件**を選ぶこと。同じentryにinstaller・source archive・他platform分が並ぶ状況で目的のzipだけを選ぶ。
- 0件は`installable=false/catalog.artifact_not_found`で、artifact 3 fieldが空のままcatalogとして書き出せること。
- 2件以上はsource error。
- template render（`{{version}}`・`{{metadata.<key>}}`・`{{asset.<field>}}`）、percent encode、file名はescapeしない、未宣言metadata・使えないroot・`{{asset.size}}`・空値・asset未選択の拒否。
- render後のfile名（path区切り・`.`・`..`・NUL・長さ）とURL（非HTTPS・credential・host無し・相対）の拒否。

### 4.3 checksum

- `sha256-space-filename`の受理7種（space 1個/2個、binary modeの`*`、CRLF、末尾改行なし、空行、他fileの行が並ぶ）。
- 拒否14種（BOM、NUL、duplicate、SHA-512長、短いhex、大文字hex、非hex、path付きfile名、backslash、tab区切り、区切り無し、対象行なし、basenameが空、file名が空）。
- `asset-field`のalgorithm決定8種（definition側／source側／両方一致／両方食い違い／決められない／hex長不一致／digestが空／assetが無い）。
- 取得の成功・404・client未注入。

### 4.4 catalog組立て

| 観点 | 固定した内容 |
|---|---|
| 並び | comparison降順。同値なら**version byte順**（goの`1.20`と`1.20.0`） |
| provider release | assetの`release_tag`が非空ならその値、なければregex適用前のraw version |
| 公開日時 | itemのみ／assetのみ／同値／どちらも無い／**食い違いはsource error** |
| lifecycle記録 | overrideはdefinition記録、staticはentry記録、既定`unknown`はsource URL＋fetch時刻 |
| 期限 | networkは`fetched_at + cache_ttl`、staticは期限なし |
| provider kind | `third-party`が`store.ProviderThirdParty`になる |
| codec往復 | 組み立てたcatalogがそのまま`EncodeCatalog`→`ParseCatalog`を通る |

## 5. 検証

### 5.1 ローカル検証

Linux container（Go 1.26.5、Python 3.11.15）で実行した。

| command | 結果 |
|---|---|
| `gofmt -l .` | 0 file |
| `go vet ./...` | 成功 |
| `go test ./... -race -shuffle=on -covermode=atomic` | 成功。`internal/catalog` 88.6%、`internal/store` 89.2% |
| `scripts/ci/check_policy.py` | 成功 |
| `scripts/ci/check_imports.py` | 成功 |
| `scripts/ci/check_docs.py` | 成功 |
| `scripts/ci/check_licenses.py` | 成功 |
| `scripts/ci/check_pr_refs.py` | 成功（task-id=p3-03, slug=artifact-checksum） |
| `git diff --check` | 差分なし |

### 5.2 CI

PR #73で、6 job×2 OSの **12 checkすべてがsuccess** になった（run 31951284256）。

## 6. 未実施・制約

- **§16.2のGo例はartifact URLを決められない**（§2.2）。標準4 toolのdefinition TOMLを作る**P3-04**で解決する。
- §6.6の「両platformの正規version集合が完全一致することを検査する」、registry全体のID/alias/command衝突は**P4-01**の範囲である。
- artifactの実download、download応答と`size`の一致、download直後のdigest照合は**P7**の範囲である。本PRはPlan前のdigest解決までとした。
- catalog cacheのfile書込み（`cache/catalogs/<tool-id>/<platform-id>.json`）、`W_CACHE_STALE`付きのoffline利用、`definition_sha256`の算出はuse case側（**P8**）の範囲である。本PRは`store.Catalog`値の組立てまでとした。
- retry／backoff／timeout／proxy／TLSは**P5-01**のHTTPClient adapterの責務である。
- `W_LIFECYCLE_OVERRIDE_UNUSED`と`catalog.artifact_not_found`のmessage catalog登録とCLI表示は`internal/message`とCLI adapterの範囲である。
- source error専用のerror codeは仕様に無く、1本目の`E_DEFINITION_INVALID`をそのまま使っている。仕様へ明記するかは利用者判断である。
- `go tool govulncheck ./...`はこのcontainerから`vuln.go.dev`が`Forbidden`で到達できず、ローカルでは未実行である（P0-02から継続）。本PRは新規依存を追加していない。
- Windowsでの実行はCI matrixでのみ確認する。
