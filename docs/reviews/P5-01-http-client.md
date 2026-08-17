# P5-01 決定記録: HTTP client adapter

対象タスク: `docs/13-progress.md` P5-01。規範仕様は[05-configuration.md](../05-configuration.md)§3.4（network）、[10-security.md](../10-security.md)§10（network）・§9.2（mask）、[04-storage-and-data.md](../04-storage-and-data.md)§21（redirect/retry上限）、[02-architecture.md](../02-architecture.md)§1・§4.1。

## 1. 着手時の確認事項（P4-02の停止記録より）

「§21のretry規定と§9.2のmask規則が`port.HTTPClient`のどのsignatureを前提にするか」を確認した。**結論として、`port.HTTPClient`と`port.HTTPRequest`は変更不要である。**

| 項目 | 仕様 | 置き場所 |
|---|---|---|
| timeout | §3.4の`connect_timeout` 1s〜5m／`request_timeout` 10s〜1h は**global config** | 要求structではなくadapter構築時（`ClientConfig`） |
| retry | §21で固定（初回後3回、backoff 1/2/4秒、Retry-After最大30秒）。§3.5「retry countは§21で固定」 | adapter内の固定policy。呼出し側は設定できない |
| redirect | `MaxRedirects`が要求structにあり、§21が上限10 | 要求側の指定と組込み上限の小さい方 |
| body上限 | `MaxBodyBytes`が要求structにある | そのまま |
| proxy/TLS | §3.4「`ProxyFromEnvironment`契約」「OS trust store」、config keyなし | adapter内で固定。構築時にも受け取らない |
| mask | §9.2のmask対象は`internal/security`に実装済み | error messageへ載るURLへ適用 |

## 2. 利用者判断: `Clock` portへ`Sleep`を追加した

**`port.Clock`に待機の手段が無かった。** `Now`／`Since`／`Monotonic`だけである。§10が「429/5xx/一時networkだけ有限retry」を求め§21がbackoffを1/2/4秒と固定するため、backoffの検証には待機をportで制御する必要がある。実時間で待つとbackoff testだけで7秒かかり、CLAUDE.md §11の「fake clockによるdeterministic test」を満たせない。

§4.1のClock行が「現在時刻、単調時間」であり待機を含むか書かれていなかったため、利用者判断を仰いだ。

| 選択肢 | 結果 |
|---|---|
| **Clock portへ`Sleep`を追加** | **採用** |
| 専用のSleeper portを追加 | 不採用（portが1件増え、時刻と待機を分ける必然性が仕様から読めない） |
| adapter内で`time`を直接使う | 不採用（deterministic testを満たせず、§6のport迂回禁止にも反する） |

`Sleep(ctx, d) error`を追加し、§4.1のClock行を「現在時刻、単調時間、待機」へ修正した。`fake.Clock`は**実際には待たず**、要求された待機時間を`Sleeps()`で記録して時計を進める。cancel済みcontextでは待機を記録せず時計も進めない。これで「retryの途中cancelが待機せずに抜ける」ことを検査できる。

実測: HTTP adapterのtest全体が**0.09秒**で完了する（backoff合計7秒×複数caseを含む）。

## 3. 判断

### 3.1 配置は`internal/platform`

§2の表に「network」領域が無い。§1がInfrastructure adapterを「FS、HTTP、Process、Windows/Linux、TOML」と列挙し、§2でInfrastructure adapterに当たる領域は`internal/platform`だけである。**§2の`internal/platform`行は「Windows/Linux固有のリンク、プロセス、権限、パス」でHTTPを含んでいなかった**ため、§1の列挙に合わせて同行へ「HTTP client」を追記した。

### 3.2 redirectを自前で追う

`http.Client.CheckRedirect`へ`http.ErrUseLastResponse`を返し、hopごとに自分で追う。`net/http`に任せると、§10が求める**hopごとのscheme/host/credential検査**を挟めない。httpsからhttpへ落とすredirectと、userinfo付きの遷移先を拒否する。

timeoutはhopごとに与える。redirect chain全体で1つにすると、hopが増えるほど各hopの実効時間が縮む。

### 3.3 body上限は切り詰めずerrorにする

`io.LimitReader`だけではEOFとして静かに切り詰められ、途中までのarchiveを完全なものとして扱ってしまう。上限＋1まで読める`limitedBody`を返し、超えた時点でerrorにする。`Content-Length`が上限を超えると分かっている応答は、読まずに拒否する。

### 3.4 TLS検証失敗をretryしない

§10は「checksum/schema/404/security errorをretryしない」と定める。`tls.CertificateVerificationError`と`tls.RecordHeaderError`を明示的に非retryにした。同じ相手へ同じ検証を繰り返しても結果は変わらず、失敗を隠すだけになる。

`Retry-After`は上限30秒で丸める。上流が過大な値を返しても従わない。解釈できない値・非正値・過去のHTTP-dateは「指定なし」として扱い、backoffの既定へ戻す。

## 4. 実装中に見つけた欠陥3件

2件はtestが、1件はCIが検出した。

### 4.1 mask済みURLをwrapしてもcause経由でsecretが漏れた

`TestClientMasksCredentialInError`が失敗した。`net/http`はrequest URLを**maskせずに**`*url.Error`へ含めるため、`%w`で包むと`?access_token=SECRETVALUE`がerror文字列へそのまま出た。

```text
platform: https://…?access_token=<redacted> の取得に失敗した:
  Get "https://…?access_token=SECRETVALUE": dial tcp …
```

`unwrapURLError`で`*url.Error`の内側だけを取り出し、URLはmask済みの側から与えるよう直した。**§9.2のmaskは、自分が組み立てる文字列だけでなくwrapするcauseにも及ぶ。**

### 4.2 connection refusedが非retryになっていた

`isTemporaryNetwork`が`net.Error`（`Timeout()`で判定）を`*net.OpError`より先に見ていた。`*net.OpError`は`net.Error`でもあるため、timeoutでないdial失敗（connection refused、reset、host unreachable）がすべて非retryとして落ちていた。

より具体的な`*net.OpError`を先に判定するよう直した。相手側の再起動や一時的な経路障害は§10の「一時network」であり、有限回の再試行対象である。

### 4.3 Go toolchainをgo1.26.6へ上げた（CIが検出）

初回CIの`lint` jobが両OSで失敗した。**本taskのcodeが初めて`net/http`を呼んだため、`govulncheck`がstdlibの脆弱性を到達可能と判定した。**

| ID | 内容 | 到達経路 |
|---|---|---|
| GO-2026-6218 | `net/url`の`resolvePath`が二次計算量 | `resolveRedirect`の`url.URL.Parse` |
| GO-2026-6090 | `crypto/tls`のpost-handshake message数が無制限 | `Client.send`／`limitedBody.Read`／`Close` |
| GO-2026-5972 | `encoding/asn1`の再帰深度 | `limitedBody.Close`経由の証明書処理 |
| GO-2026-5026 | `x/net/idna`がASCII-onlyのPunycode labelを拒否しない | `Client.send`の`http.Client.Do` |

4件すべて`go1.26.6`で修正済みである。[11-quality-and-ci.md](../11-quality-and-ci.md)§1が`go.mod`の`toolchain`を「採用minorの最新security patch」と定めるため、`go1.26.5`から`go1.26.6`へ上げ、同§の「現在は」表記も同じ変更で更新した。Go versionの正本は`go.mod`だけで、workflowへ数値を書かない契約に従っている。

**`vuln.go.dev`へこのcontainerから到達できない**（403）ため、`govulncheck`のローカル再実行はできていない。修正の根拠はCIが出した「Fixed in: go1.26.6」であり、確認はCIで行う。

## 5. 検査が固定したこと

| 検査 | 対象 |
|---|---|
| `TestClientGetReadsBody` / `HeadDiscardsBody` | 成功応答、FinalURL、header、HEADがbodyを返さないこと |
| `TestClientRejectsInsecureRequest` | http／scheme無し／file／userinfo／host無し／上限0・負／credential header 3種（11件） |
| `TestClientMasksCredentialInError` | error messageへtokenを載せないこと |
| `TestClientFollowsRedirect` / `RejectsRedirectToInsecureScheme` / `RejectsRedirectWithCredential` | hopごとの検査 |
| `TestClientEnforcesRedirectLimit` | 要求側上限、組込み上限10への丸め、redirect禁止 |
| `TestClientEnforcesBodyLimit` | Content-Lengthでの事前拒否、読取り中の超過、上限ちょうど |
| `TestClientRetriesTransientStatus` | 429/500/503はretry（backoff 1/2/4秒）、400/403/404はretryしない（6件） |
| `TestClientSucceedsAfterRetry` | retry後の成功とbackoff回数 |
| `TestClientHonorsRetryAfter` / `HTTPDate` | 秒数・上限超過・上限ちょうど・解釈不能・0・負・未来・過去（9件） |
| `TestClientStopsOnCancel` | cancelで待機せず抜けること |
| `TestNewClientUsesStandardProxyAndTrustStore` | Proxy設定、`InsecureSkipVerify=false`、`RootCAs=nil`、TLS 1.2以上 |
| `TestIsTemporaryNetworkExcludesTLSFailures` | TLS失敗を非retry、timeout／dial失敗をretry |
| `TestRetryBackoffMatchesRetryCount` | backoff表とretry回数の一致 |
| `TestClockSleep*`（fake） | 待機の記録、非正値の無視、cancel時に待機も時計進行もしないこと |

TLS serverのself-signed証明書は、**client側の検証を緩めるのではなく**server証明書をtransportへ与えて信頼する。production実装がTLS検証を無効化する経路を持たないことが本taskの契約である。

## 6. 検証

すべてLinux containerで実行した。

| command | 結果 |
|---|---|
| `gofmt -l .` / `go build ./...` / `go vet ./...` | 出力なし・成功 |
| `go test ./... -race -shuffle=on -covermode=atomic` | 全package成功。`internal/platform` 93.3% |
| `check_policy.py` / `check_imports.py` / `check_docs.py` / `check_licenses.py` / `check_messages.py` | すべて成功 |
| `check_pr_refs.py` / `git diff --check` | 成功・出力なし |

覆えていないのは、public APIから到達しない防御分岐（`validateRequest`が先に拒否する`readBody`の上限検査、`url.Parse`が成功したURLの再parse失敗）である。

## 7. 未実施・制約

- **structured logを出していない。** §18の`LogRecord`がinvocation/operation IDを必須とし、adapterがそれらを持たない。秘密値のmaskはerror messageへ載るURLに適用した。記録はrequestを発行するApplication Serviceの責務である（P8）。
- **実際のproxy動作をtestしていない。** `ProxyFromEnvironment`が設定されていることは検査したが、proxy経由の通信そのものはproxy serverを要する。§9の利用者確認チェックリストの範囲である。
- **`.part` stream、digest計算、progress、cache identityはP5-02の範囲**である。本taskはHTTP層まで。
- **download cacheとoffline判定は実装していない**（P5-02）。
- `ClientConfig`は`internal/config`の`GlobalConfig`から値を受け取る前提だが、**両者を接続する組立てコードは無い**。`Ports`の構築はP8-01の範囲である。
- §21の`redirect / network retry`行は「10 / 初回後3回」と2つの値を1行に持つ。redirectが10、retryが3という読みで実装した。
