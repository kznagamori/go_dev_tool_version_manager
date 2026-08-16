# P3-03 決定記録（2/3）: `json-index`・`static` source・lifecycle

対象タスク: `docs/13-progress.md` P3-03（3分割の2本目）。規範仕様は[06-tool-definition.md](../06-tool-definition.md)§6.1〜§6.4・§6.6、[04-storage-and-data.md](../04-storage-and-data.md)§16.2・§21、[10-security.md](../10-security.md)§8。

## 1. 着手時の確認事項

### 1.1 index子文書のhost許可元（仕様から一意に決まった）

1本目の停止記録が残した確認事項である。§6.2は「子文書URLのhostはindex文書のhost、または`artifact.redirect_hosts`と**同じ規則で宣言した**完全hostだけを許す」と書く一方、§6.1の許可key 24件にhostを宣言するkeyが無い。

読み方は2つありうるが、**version source側に宣言keyが無いため後者は成立しない**。

| 読み | 成否 |
|---|---|
| 同じplatformの`artifact.redirect_hosts`が宣言したhost集合を指す | 成立する。`[platforms.artifact]`と`[platforms.version_source]`は同じplatform配下にある |
| version sourceが独自のhost keyを持つ | **成立しない**。§6.1の許可key 24件に該当keyが無く、unknown keyとして拒否される |

したがって前者を実装した。「同じ規則」はwildcardなしのASCII lowercase完全host（§7.1）を指す。§16.4の.NET SDKは`redirect_hosts`を宣言せず、index文書と子文書が同じ`builds.dotnet.microsoft.com`にあるため、v0.1の標準registryではindex hostだけで足りる。利用者判断を要する曖昧さではないと判断し、そのまま実装した。

### 1.2 `W_LIFECYCLE_OVERRIDE_UNUSED`

[04-storage-and-data.md](../04-storage-and-data.md)§16.2の`ResultWarningCode` exactly 5件に含まれる。P1-04の`internal/progress`が型と検証を持つため、`internal/catalog`はそこへ`ResultWarning`を組み立てるだけでよい。

## 2. 判断

### 2.1 lifecycleの根拠に`static`を足した

P3-02の`LifecycleSource`は§6.3の優先順位3段と1対1だった。§6.3はnetwork sourceの規則であり、static sourceはitem自身へlifecycleを書き（§6.6）、§6.4がoverrideを禁じるため、3段の優先順位を通らない。`source`（上流から`lifecycle_map`で写像した値）と同じ表現にすると、上流から読んだ値とdefinitionへ書いた値の区別が`doctor`のevidence欄で付かなくなる。`LifecycleFromStatic`を足した。

### 2.2 `document_lifecycle_pointer`は文書ごとに1回だけ解決する

§6.2は「子文書の**top-level**から1つの値を読み、その子文書由来の全itemへ同じlifecycleを与える」と定める。`BuildItems`は子文書rootを受け取っているため、item loopの外で1回だけ解決してitemへ配る。item内で毎回解決すると、pointerがitem相対と誤解される実装になりやすい。

`lifecycle_pointer`（item相対）との同時宣言はdefinitionのschema検証が拒否済みである。

### 2.3 `max_documents`は重複除去後の件数へ効く

§6.2は「`max_documents`は必須で1以上、組込み上限32以下。重複URLは1回だけ取得する」と定める。上限は**取得する文書数**であるため、重複除去後の件数で判定する。除去前で判定すると、同じ子文書を複数回参照するindexが正当な範囲で拒否される。超過は切り捨てずerrorにする。読む文書を黙って打ち切ると、残りの子文書に載っていたversionが存在しないことと区別できなくなる。

### 2.4 子文書URLはportとcredentialを拒否する

`redirect_hosts`は「ASCII lowercase完全host」であり（§7.1）、portの有無で同じhostが2通りに書けると照合が一致しなくなる。credentialを含むURLは[10-security.md](../10-security.md)の禁止事項である。lowercaseでないhostも拒否する。**index応答から任意hostを動的に信頼しない**（§6.2・[10-security.md](../10-security.md)§132）。

### 2.5 static sourceは記載順に依存しない

§6.6が「version itemをfile記載順で解釈せず、正規version byteで一意検査してcomparison keyでsortする」と定める。一意検査はP3-01の`definition`が済ませているため、ここではcomparison keyでsortする。記載順に依存すると、registryのdiffで行を並べ替えただけでcatalogの内容が変わる。schemeが混ざって比較できない場合はerrorにし、順序を決められないまま結果を返さない。

### 2.6 未使用overrideはentryごとに1件報告する

まとめて1件にすると、どのentryを直せばよいかがparameterから読めない。definitionの宣言順で返す。**overrideのversionからcatalog itemを合成しない**（§6.4）。上流に無いversionをoverrideだけで作ると、installできないversionが`available`へ並ぶ。

## 3. 検査が固定したこと

### 3.1 `json-index`

| 観点 | 固定した内容 |
|---|---|
| 宣言順 | index記載順のまま子文書URLを返す |
| 重複除去 | 最初の出現順を保って1回だけにする |
| host | index hostは許可、未宣言hostは拒否、`redirect_hosts`宣言hostは許可 |
| URLの形 | 非HTTPS・相対・credential・port・大文字host・host無し・`ftp:`の7件を拒否 |
| `max_documents` | 除去後の件数で判定、超過はerror、組込み上限32を拡大できない |
| index layout | `index_items_pointer`が配列でない/無い、`index_document_pointer`が無い/型違いを拒否 |
| index URL | 非HTTPSのindex URL自体も拒否 |
| 部分catalog禁止 | 2件目の取得失敗・parse失敗で**全体が失敗し、読めた分を返さない** |

### 3.2 `static`

- 記載順（`3.13.7`, `3.9.0`, `3.10.0`, `3.14.0rc1`）から**comparison key昇順**（`3.9.0`, `3.10.0`, `3.13.7`, `3.14.0rc1`）へ並べ替える。多桁の`3.10.0`が`3.9.0`より後になる。
- channel/lifecycleをitem自身から取り、根拠は`static`になる。**channelとlifecycleは独立**で、prereleaseへ`eol`も表現できる。
- 公開日時はUTC RFC 3339へ揃え、未設定は空文字にする（取得時刻で代用しない）。
- `Node`はnil、`Static`が元entryを指す。asset・evidence・assessment時刻をdefinition側から読める。
- `max_items`超過、未知channel、scheme混在を拒否する。

### 3.3 lifecycle

| 観点 | 固定した内容 |
|---|---|
| `document_lifecycle_pointer` | 5 phase（`preview`/`go-live`/`active`/`maintenance`/`eol`）を写像し、同じ子文書由来の全itemへ与える |
| mapに無い値 | source error。黙って`unknown`へ倒さない |
| 参照fieldの欠落 | source error |
| `lifecycle_pointer` | item単位で値が変わる |
| 既定 | `json` sourceは優先順位1と3だけで決まり、overrideが無ければ`unknown`／`default` |
| override | 一致するversionだけに効き、他は既定のまま |
| override矛盾 | source評価経路でも§6.4の拒否が効く |
| 未使用override | 宣言順に1件ずつ、`Validate()`を通る`ResultWarning`になる。使用済み・override無しは0件 |

## 4. 検証

### 4.1 ローカル検証

Linux container（Go 1.26.5、Python 3.11.15）で実行した。

| command | 結果 |
|---|---|
| `gofmt -l .` | 0 file |
| `go vet ./...` | 成功 |
| `go test ./... -race -shuffle=on -covermode=atomic` | 成功。`internal/catalog` 96.5%（test 124件） |
| `scripts/ci/check_policy.py` | 成功。production Go file 109件 |
| `scripts/ci/check_imports.py` | 成功。package 20件 / internal import 129件 |
| `scripts/ci/check_docs.py` | 成功。file 43件 |
| `scripts/ci/check_licenses.py` | 成功。module 14件 |
| `scripts/ci/check_pr_refs.py` | 成功（task-id=p3-03, slug=index-static-source） |
| `git diff --check` | 差分なし |

### 4.2 CI

PR #70で、6 job×2 OSの **12 checkすべてがsuccess** になった（run 31948826942）。

## 5. 未実施・制約

- **3本目の着手時に確認する仕様の穴が1件ある。** [04-storage-and-data.md](../04-storage-and-data.md)§15は「lifecycle evidenceは公式/providerのHTTPS URL、assessmentはUTC RFC 3339で**全状態に必須**。source fieldならsource URL/fetch時刻、override/staticならdefinition記録を使う」と定めるが、**優先順位3の既定`unknown`**（source fieldもoverrideも無い状態）の記録元を挙げていない。§6.1の利用者判断で`json` sourceはlifecycle pointerを持てないため、Node.jsとGoは全itemがこの状態になる。本PRは`LifecycleDecision.From`で根拠の由来までを持ち、evidence/assessment値の決定は3本目へ送った。
- asset field／`required_tokens`／artifact template・selector／checksum 2 kind／§15 catalog組立てと並び（comparison降順、同値ならversion byte順）は**3本目**の範囲である。
- `source_identity`（`json-index`はindex文書のURL）、`fetched_at`、`expires_at`、`definition_sha256`の生成は3本目である。`cache_ttl`は本PRでも参照していない。
- §6.6の「両platformの正規version集合が完全一致することを検査する」はregistry validatorの責務で**P4-01**の範囲である。本PRは1 platform分のstatic version集合だけを扱う。
- retry／backoff／timeout／proxy／TLSは**P5-01**のHTTPClient adapterの責務である。
- `W_LIFECYCLE_OVERRIDE_UNUSED`のmessage catalog登録とCLI表示は`internal/message`とCLI adapterの範囲であり、本PRは`ResultWarning`値の生成までとした。
- `go tool govulncheck ./...`はこのcontainerから`vuln.go.dev`が`Forbidden`で到達できず、ローカルでは未実行である（P0-02から継続）。本PRは新規依存を追加していない。
- Windowsでの実行はCI matrixでのみ確認する。
