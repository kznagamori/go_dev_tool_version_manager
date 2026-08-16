# P3-02 決定記録: version grammar・比較・channel/lifecycleの境界test

対象タスク: `docs/13-progress.md` P3-02。規範仕様は[06-tool-definition.md](../06-tool-definition.md)§4（version scheme）、§6.1（channel導出と写像）、§6.3（version変換とlifecycle優先順位）、§6.4（lifecycle override）、および[02-architecture.md](../02-architecture.md)§2（`internal/catalog`の責務）。

## 1. 境界testで見つけた欠陥

### 1.1 桁あふれでprocessが落ちる（修正した）

P1-02の`domain.ParseVersion`は、正規表現が数値と確定させた部分文字列を`mustAtoi`で変換し、変換失敗時に**panicしていた**。「正規表現を通った値だから失敗しない」という前提だったが、正規表現`(0|[1-9][0-9]*)`は**桁数を制限しない**。境界testで次の2件が再現した。

| 入力 | 結果 |
|---|---|
| `ParseVersion(semver, "18446744073709551616.0.0")` | panic |
| `ParseVersion(semver, "1.0.0-18446744073709551616")` は成功し、その後の`Compare` | panic |

2件目はparseを通ってしまうため、**panicが比較時まで遅延する**。version文字列は上流catalogのJSONに由来する外部入力であり（§6.3の`version_pointer`＋`version_regex`）、桁数はgdtvm側で決められない。[CLAUDE.md](../../CLAUDE.md)§9の「production pathでpanicを通常のerror処理として使わない」と§8のfail closedに反する。

修正:

- `mustAtoi`を削除し、`parseNumbers`（`strconv.ParseUint(part, 10, 64)`）へ置き換えた。桁あふれは**parse error**にする。
- semverのprerelease数値識別子もparse時に数値化し、`semverIdent{text, num, numeric}`として保持する。`Compare`は`error`を返せないため、**比較の途中で失敗しうる変換を残さない**。parseを通ったVersionは必ず比較できる、という不変条件を型で持たせた。
- 表現範囲は**64 bit符号なし整数**とした。`int`のままだと32 bit platformと64 bit platformで受理するversionが変わり、同じregistryがplatformごとに違うcatalogになる。

上限値そのものは仕様に無い。§4は数値要素の桁数を規定していないため、**本PRで導入した規約**である（§5に記載）。

### 1.2 §6.4のoverride矛盾検査が無かった（実装した）

§6.3はlifecycleの優先順位を「1. override、2. `lifecycle_map`の写像結果、3. `unknown`」と定め、§6.4は「source lifecycle fieldと同じversionで**矛盾するoverrideを拒否する**」と定める。両者は両立する。優先順位は解決の順序、§6.4はその上の妥当性検査であり、結果として1と2が食い違うcatalogは成立しない。

この検査はsourceの値が判明する`internal/catalog`でしかできない。definitionのparse時には上流の値がまだ無いため、P3-01では実装できなかった。`ResolveLifecycle`が両方を受け取る形にして、食い違いをerrorにした。優先順位1で黙って勝たせると、上流が`supported`へ戻したのに古い`eol` overrideが残っていることに誰も気付けない。

## 2. 判断

### 2.1 channel/lifecycle判定を`internal/catalog`へ置く

[02-architecture.md](../02-architecture.md)§2が`internal/catalog`へ「配布元照会、版正規化、**channel/lifecycle判定**、catalog cache」を割り当てている。判定規則はP3-02の対象だが実体が無かったため、本PRで`internal/catalog/judge.go`を作った。上流文書の取得・pointer解決はP3-03の範囲であり、本PRは**判定規則だけ**を持つ。

`scripts/ci/check_imports.py`へ`internal/catalog` → `internal/domain`・`internal/definition`を根拠付きで追加した。

### 2.2 prerelease判定はdomainが持つ

「正規versionが各schemeのprerelease構文を持つか」はversion grammarの性質であり、scheme別のstage値を知っているのは`domain.Version`だけである。`Version.IsPrerelease()`をdomainへ置き、catalogは`DeriveChannel`でそれを§6.1のchannelへ写像する。catalog側で文字列を再解析すると、grammarの正本が2か所になる。

### 2.3 overrideの照合は正規version文字列の完全一致

§4が「入力versionはcatalogの正規文字列完全一致であり、comparison keyへ変換した近似一致をしない」と定める。goの`1.20`と`1.20.0`は**comparison keyが同じで正規文字列が違う**。comparison keyで照合すると、`1.20`へ書いたoverrideが`1.20.0`へも当たる。`ResolveLifecycle`と`UnusedOverrides`の両方を文字列一致にし、testで固定した。

### 2.4 `MapChannel`のbooleanは真がstable

§6.1は「booleanは`true`を`stable`、`false`を`prerelease`へ写像する」と定める。Goの`https://go.dev/dl/`が`"stable": true`で正式版を示すためで、向きを逆に取ると**全versionのchannelが反転する**。doc commentとtestの両方へ理由を書いた。

### 2.5 `UnusedOverrides`は宣言順を保つ

§6.4の`W_LIFECYCLE_OVERRIDE_UNUSED`は、どのentryを直せばよいかを利用者へ示すためのものである。並べ替えるとdefinitionと突き合わせにくくなるため、definitionの宣言順のまま返す。

## 3. 検査が固定したこと

### 3.1 数値要素

| 観点 | 固定した内容 |
|---|---|
| 多桁 | `1.9 < 1.10`、`22.9.0 < 22.18.0`、`1.20beta9 < 1.20beta10`など、全schemeの全位置で**数値順**（文字列順ではない） |
| 桁あふれ | `2^64`はparse error、`2^64-1`は受理。3 scheme全位置＋semver prerelease識別子＋go/pythonのprerelease番号の12件 |
| leading zero | `01.2.3`／`1.02.3`／`1.2.03`／`1.2.3-01`／`1.20beta01`など13件を拒否。`0`そのものは受理 |

### 3.2 grammar境界

- 前後の空白・LF・CRLF・tab・NULを拒否する。改行は正規表現実装によっては`$`に一致するため明示的に固定した（Goは末尾改行を許さない）。
- go/pythonのstage表記は小文字だけ（`1.20Beta1`、`3.13.0RC1`を拒否）。semverのprerelease識別子は大文字を許す。
- goのprereleaseはpatchを持たない（`1.20.1beta1`を拒否）。
- semver識別子: `alpha-1`・`--`・`0.3.7`は受理、`alpha..1`・`alpha.`・`.alpha`・`alpha_1`・`alpha+build`・非ASCII・spaceは拒否。

### 3.3 比較

- **goとpythonでstageとpatchの比較順が違う**ことを対比で固定した。goはstageをpatchより先に見るため`1.20rc1 < 1.20.9`、pythonはpatchをstageより先に見るため`3.13.1a1 > 3.13.0`になる。「prereleaseは常に小さい」と一括実装すると後者を逆順にする。
- 3 schemeそれぞれ11〜18件の昇順列について**全ペア**で反射律・反対称律・比較値を検査する。既存testは隣接ペアだけを見ており、比較段の抜けで生じる非推移的な順序を検出できなかった。
- SemVer 2.0.0のprecedence 6件（prerelease<final、数値<非数値、数値順、前方一致なら識別子数、ASCII順）。

### 3.4 完全一致

- `String()`が入力byte列をそのまま返す（byte長も含めて検査する）。parseがどこかで正規化すると、利用者入力とcatalogのkeyが一致しなくなる。
- goの`1.20`と`1.20.0`は`Compare`が0を返すが、正規文字列で引くと当たらない。

### 3.5 channel/lifecycle

| 観点 | 固定した内容 |
|---|---|
| channel導出 | 3 scheme 12件。**構文だけ**で決め、`0.0.0`もstable |
| channel写像 | string 2値とbooleanだけ。未知string・空・大文字・前後空白・integer・nullを拒否 |
| lifecycle優先順位 | 1 override / 2 source / 3 default の3経路と`From` |
| override照合 | goの`1.20`と`1.20.0`で当たり分けする |
| override矛盾 | sourceと食い違えばerror、一致すればoverride採用 |
| channelとlifecycleの独立 | prereleaseへの`eol` overrideが通り、channelは`prerelease`のまま |
| `lifecycle_map` | 未定義値・空文字・大文字・前後空白・map無し・空mapを拒否 |
| 未使用override | 宣言順で返す。全件使用時とoverride無しは0件 |

## 4. 検証

### 4.1 ローカル検証

Linux container（Go 1.26.5、Python 3.11.15）で実行した。

| command | 結果 |
|---|---|
| `gofmt -l .` | 0 file |
| `go vet ./...` | 成功 |
| `go test ./... -race -shuffle=on -covermode=atomic` | 成功。`internal/domain` 97.3%（test 89件）、`internal/catalog` 100.0%（test 27件） |
| `scripts/ci/check_policy.py` | 成功。production Go file 102件 |
| `scripts/ci/check_imports.py` | 成功。package 20件 / internal import 103件 |
| `scripts/ci/check_docs.py` | 成功。file 41件 |
| `scripts/ci/check_licenses.py` | 成功。module 14件 |
| `scripts/ci/check_pr_refs.py` | 成功（task-id=p3-02, slug=version-grammar） |
| `git diff --check` | 差分なし |

### 4.2 CI

PR作成後に6 job×2 OSの12 checkの結果を記録する。

## 5. 未実施・制約

- version数値要素の**64 bit符号なし整数上限**は§4に規定が無く、**本PRで導入した規約**である。以前は`int`への変換失敗でpanicしており、範囲を決めずに済ませることはできない。仕様へ昇格させるか別の上限にするかは利用者判断である。
- 上流文書の**取得と評価**（`json`/`json-index`/`static`の読取り、index 2段取得、部分catalog禁止、`item_flatten_pointer`の1段展開、親公開日時の継承、pointer解決、`document_lifecycle_pointer`）は**P3-03**の範囲である。本PRの`internal/catalog`は判定規則だけを持ち、HTTPにもJSONにも触れない。
- `W_LIFECYCLE_OVERRIDE_UNUSED`の**warning発行**（message ID、`ResultWarning`への載せ替え、CLI表示）はP3-03以降の範囲である。本PRは対象entryを返すところまでとした。
- §6.6 static sourceの`channel`/`lifecycle`はitem自身が持つため、本PRの導出・写像を通らない。static版の一意検査とsortはP3-03の範囲である。
- catalogの`published_at`決定（item／親／assetの優先順と複数非空値の矛盾検査）は**P3-03**の範囲である。
- `go tool govulncheck ./...`はこのcontainerから`vuln.go.dev`が`Forbidden`で到達できず、ローカルでは未実行である（P0-02から継続）。本PRは新規依存を追加していない。
- Windowsでの実行はCI matrixでのみ確認する。
