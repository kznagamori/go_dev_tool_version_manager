# P3-03 決定記録（1/3）: 文書取得・RFC 6901 pointer・`json` source

対象タスク: `docs/13-progress.md` P3-03（3分割の1本目）。規範仕様は[06-tool-definition.md](../06-tool-definition.md)§6.1（共通key・pointer・公開日時）、§6.3（version変換・channel）、[04-storage-and-data.md](../04-storage-and-data.md)§21（組込み上限）、[02-architecture.md](../02-architecture.md)§2・§4.1（責務とport）。

## 1. 着手時に見つけた仕様の矛盾（利用者判断で解決）

### 1.1 `items_pointer = "/"` とRFC 6901

§6.1は「pointerはすべてRFC 6901」と定める。RFC 6901では空文字が文書全体を指し、`/`は**keyが空文字のmember**を指す。ところが仕様の3か所が `items_pointer = "/"` を使い、いずれもtop-levelがJSON配列の文書を対象にしていた。

| 箇所 | 対象文書 |
|---|---|
| §6 冒頭例 | `https://example.invalid/versions.json` |
| §15 Node.js | `https://nodejs.org/dist/index.json`（配列） |
| §16.2 Go | `https://go.dev/dl/?mode=json&include=all`（配列） |

配列に対する `/` は解決できず、§6.1の「definitionが参照するfieldの欠落はsource error」に従うと**GoとNode.jsの標準定義が必ずsource errorになる**。

利用者判断で**仕様例を `""` へ修正**した。§6.1本文へ「空文字が文書全体を指し、`/`はkeyが空文字のmemberを指す」「配列やobjectの型に応じて`/`をrootへ読み替える等の代替解釈を行わない」を明記し、3例とP3-01のfixture 3件を同じ変更で合わせた。読み替えを許すと、同じdefinitionが上流文書の形の変化で黙って別のnodeを指すようになり、source errorで気付けない。

### 1.2 任意pointerの「未宣言」と「空文字宣言」を区別できなかった

P3-01の`optionalPointer`は両者を `""` として返していた。§6.1は`channel_pointer`を**省略した場合**に正規versionのprerelease構文からchannelを導出すると定めるため、区別できないと `channel_pointer = ""` のsourceが黙って構文導出へ落ちる。`published_at_pointer`の「どれも宣言しないsourceは空文字とし」、`item_flatten_pointer`の「指定した場合」も同じ性質である。

`definition.OptionalPointer`（`Declared()`／`Value()`）を導入し、任意pointer 8件（`channel_pointer`, `published_at_pointer`, `assets_pointer`, `item_flatten_pointer`, `item_parent_published_at_pointer`, `lifecycle_pointer`, `document_lifecycle_pointer`, `required_tokens_pointer`）の型を変えた。P3-01が表構造体で「keyが無い」と「空の値」を`*string`で分けたのと同じ方針を、公開型でも保つ。

## 2. 判断

### 2.1 source errorのcodeは`E_DEFINITION_INVALID`

仕様はsource error専用のcodeを定めていない。[03-cli.md](../03-cli.md)§7の完全性group（exit 6）にsource側の不一致を置ける唯一のcodeであり、[08-install-runtime.md](../08-install-runtime.md)§111が「definition参照不正」を同codeへ割り当てている。上流文書に対してdefinitionのpointer/regexが解決できない状態は同じ性質で、直す先もregistryのdefinitionである。取得そのものの失敗は`E_NETWORK`（retryable）とした。

### 2.2 retryとbackoffを持たない

[08-install-runtime.md](../08-install-runtime.md)§70の「network timeout/5xx/429だけ初回後に最大3回retry」は`docs/13-progress.md` P5-01が持つHTTPClient adapterの責務である。`FetchDocument`が二重に再送すると上限が積算する。本packageは1回の要求結果だけを扱う。

### 2.3 16 MiB上限を呼出し側でも切る

`MaxBodyBytes`はadapterへ渡す要求値である。adapterの実装差で上限が効かない場合に備え、`io.LimitReader`で上限+1 byteまで読んで超過を検出する。§21の16 MiBを二重に守る。redirect後のHTTPS検査も同じ理由で呼出し側に置いた。平文経路の内容を取り込むと、以降のdigest照合まで信頼の根が無くなる。

### 2.4 数値は`json.Number`で保持する

§6.5が「IDもprecision lossを避けるためdecimal stringとして扱う」と定める。`float64`へ落とすと2^53を超えるrelease ID/asset IDが桁落ちし、receiptの再現性が壊れる。復号は`UseNumber()`を使い、公開境界の[domain.Scalar]へ落とすのは写像が必要な箇所だけにした。

### 2.5 JSON文書はexactly 1値として読む

後続に値が続く入力を受けると、どちらを文書とみなすかが決まらない。復号後に2件目のdecodeを試し、`io.EOF`でなければsource errorにする。

### 2.6 部分結果を返さない

§6.1は欠落と型違いをsource error、§6.2は「1件でも取得・parseに失敗したらcatalog全体をsource error」と定める。`BuildItems`は1件でも壊れていれば全体を失敗させる。読めたitemだけでcatalogを作ると、上流のlayout変更が「versionが減っただけ」に見えて気付けない。

### 2.7 item数の上限は連結後に判定する

§6.1の「全文書合計のitemsは10,000の組込み上限以下」は`json-index`では子文書の合計に効く。`BuildItems`は件数上限を適用せず、呼出し側が連結後に`CheckItemLimit`を呼ぶ形にした。ただし**1文書だけで組込み上限を超える入力はper-item処理の前に止める**。上流の暴走した応答へ比例した計算量を持たないためである。超過は切り捨てずerrorにする。黙って打ち切ると、上限に達した以降のversionが存在しないことと区別できない。

## 3. 検査が固定したこと

### 3.1 RFC 6901

- RFC 6901 §5の例（`""`, `/foo`, `/foo/0`, `/`, `/a~1b`, `/m~0n`ほか12件）をそのまま通す。
- **`/`をrootへ読み替えない。** top-levelが配列の文書へ`""`は解決でき、`/`は解決できない。
- escape解除は`~1`→`/`の後に`~0`→`~`。順序を逆にすると`~01`が`/`になり本来の`~1`と区別できなくなることを7件で固定した。
- 拒否: 先頭が`/`でない、keyが無い、配列範囲外、`-`、leading zero、符号付き、非数値、空index、scalarを辿る。

### 3.2 取得

| 観点 | 固定した内容 |
|---|---|
| 成功 | HTTPS GET 1件、redirect上限10、body上限16 MiB |
| 桁落ち | 2^53+1のIDが`json.Number`のまま残る |
| 失敗 | 404/500/204は`E_NETWORK`、壊れたJSON・空body・余分な値は`E_DEFINITION_INVALID` |
| 再送 | transport失敗は`E_NETWORK`かつ`Retryable=true` |
| scheme | redirect後が`http://`ならerror |
| size | adapterが上限を無視しても16 MiBで止まる |

### 3.3 `json` source評価

- §15 Node.js形（top-level配列、`items_pointer = ""`、full-dateの`date`、`channel_pointer`省略）で正規version・raw version・正規化した公開日時・構文導出channelを固定した。
- §16.2 Go形で`stable` booleanの写像を固定した。**真がstable**である。
- 宣言したpointerが構文導出より優先することを、構文はstableだがpointerが`false`のitemで固定した。
- `item_flatten_pointer`の1段展開、親の並び順の保持、親公開日時の全子itemへの継承、**再帰展開しないこと**を固定した。
- source layout違反14件（regex不一致、参照fieldの欠落/型違い、`items_pointer`が配列でない、公開日時が日時でない、channel値が数値/未知string、flatten先が配列でない/無い、親公開日時の欠落/不正、schemeに合わないversion）がすべて`E_DEFINITION_INVALID`になることを固定した。
- 空の配列は正当な結果として扱う。上流が一時的に0件を返した場合とlayoutが壊れた場合を混同しない。
- `CheckItemLimit`は`max_items`が組込み上限を**縮小する方向にだけ**働くことを6件で固定した。

### 3.4 公開日時とscalar

- full-dateは`T00:00:00Z`へ、0 offsetの`+00:00`は`Z`へ、秒未満は正規形へ落とす。
- 拒否: 空、UTCでないoffset、offsetなし、space区切り、区切りなし、範囲外の月、日時でない文字列、末尾space付き。
- `toScalar`はstring/bool/integer/nullだけを受け、配列・object・非整数・範囲外の数値を拒否する。

## 4. 検証

### 4.1 ローカル検証

Linux container（Go 1.26.5、Python 3.11.15）で実行した。

| command | 結果 |
|---|---|
| `gofmt -l .` | 0 file |
| `go vet ./...` | 成功 |
| `go test ./... -race -shuffle=on -covermode=atomic` | 成功。`internal/catalog` 95.9%（test 77件）、`internal/definition` 91.9% |
| `scripts/ci/check_policy.py` | 成功。production Go file 106件 |
| `scripts/ci/check_imports.py` | 成功。package 20件 / internal import 113件 |
| `scripts/ci/check_docs.py` | 成功。file 42件 |
| `scripts/ci/check_licenses.py` | 成功。module 14件 |
| `scripts/ci/check_pr_refs.py` | 成功（task-id=p3-03, slug=json-source） |
| `git diff --check` | 差分なし |

### 4.2 CI

PR #67で、6 job×2 OSの **12 checkすべてがsuccess** になった（run 31947801691）。

## 5. 未実施・制約

- `json-index`の2段取得と部分catalog禁止、`static` source、`document_lifecycle_pointer`／`lifecycle_map`、lifecycle overrideの適用と`W_LIFECYCLE_OVERRIDE_UNUSED`は**2本目**の範囲である。本PRの`BuildItems`は`json-index`の子文書1件にもそのまま使えるが、子文書URLの収集とhost検査は持たない。
- asset field／`required_tokens`／artifact template・selector／checksum 2 kind／§15 catalog組立てと並びは**3本目**の範囲である。本PRは`VersionItem.Node`にitem本体を残すところまでとした。
- catalogの`published_at`は「item pointer、親pointer、**選択assetの`published_at`**の順で最初の宣言済み値を使い、複数の非空値が異なればsource error」（§6.1）である。本PRはitem pointerと親pointerまでを解決し、選択assetとの突き合わせは3本目で行う。
- `source_identity`、`fetched_at`、`expires_at`、`definition_sha256`を含むcatalog JSONの生成は3本目である。`cache_ttl`は本PRで参照していない。
- retryとbackoff、timeout、proxy、TLSは**P5-01**のHTTPClient adapterの責務である。本PRは1回の要求結果だけを扱う。
- source error専用のerror codeは仕様に無く、`E_DEFINITION_INVALID`は§2.1の根拠から導いた**本PRの判断**である。仕様へ明記するかは利用者判断である。
- `go tool govulncheck ./...`はこのcontainerから`vuln.go.dev`が`Forbidden`で到達できず、ローカルでは未実行である（P0-02から継続）。本PRは新規依存を追加していない。
- Windowsでの実行はCI matrixでのみ確認する。
