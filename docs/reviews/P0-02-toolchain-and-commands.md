# P0-02 Go module/toolchainと固定commandの決定記録

## 1. 目的と対象

[13-progress.md](../13-progress.md) P0-02「Go module/toolchain、format/vet/lint/test/coverage command、証跡directory・命名・secret除去規則を固定する」の決定内容と検証結果を記録する。

固定した内容は[11-quality-and-ci.md](../11-quality-and-ci.md)§1.1〜§1.5を正本とし、本書は決定理由と実測結果の証跡である。既存の§2以降は番号を変えていない。他文書が`§N`で相互参照しているため、節の挿入で参照がずれることを避けた。

環境はLinux container（Go 1.26.5をGOTOOLCHAINで取得、Python 3.11.15）とCI matrix（`ubuntu-latest`／`windows-latest`、Python 3.12）。

## 2. 利用者判断

2件を確認した。

| # | 項目 | 採用 | 主な理由 | 他の選択肢 |
|---|---|---|---|---|
| 1 | license検査 | **A: 自作script** | 外部tool依存なし。既存CI scriptと同じ形で両OS共通になる。許可listを仕様へ置ける | B: `go-licenses`（最新v1.6.0が2023-01-16で3年半更新なし）、C: PR判断のみ（§1の「CIで実行」を満たさない） |
| 2 | coverage | **A: 計測のみ** | package実装前に根拠のない閾値を固定しない。実測値が揃ってから別taskで判断できる | B: 今すぐ閾値、C: package単位の閾値 |

## 3. 仕様から一意に決めた項目

| 項目 | 値 | 根拠 |
|---|---|---|
| module path | `github.com/kznagamori/go_dev_tool_version_manager` | [06-tool-definition.md](../06-tool-definition.md)の`schema_id`が同URLを使用 |
| `go.mod`の`go` | `1.26.0` | §1「minimum toolchainはGo 1.26系」 |
| `go.mod`の`toolchain` | `go1.26.5` | §1「採用minorの最新security patchへ固定」。`proxy.golang.org`の`golang.org/toolchain`一覧で1.26系の最新が1.26.5と確認 |
| vulnerability検査 | `govulncheck`（`golang.org/x/vuln` v1.6.0） | Go公式tool。他に候補がない |
| 証跡directory | `docs/reviews/<TASK-ID>-<slug>.md` | S00-04〜S00-06、P0-00の4 reportの実績 |

## 4. 設計判断

### 4.1 Go versionの正本を`go.mod`だけにした

workflowへ`go-version: '1.26.5'`と書き、`go.mod`にも`toolchain go1.26.5`と書くと、片方だけ更新された状態が静かに成立する。正本を`go.mod`の`toolchain`行だけとし、workflowは`go-version-file: go.mod`で読む。`lint` jobが`go.mod`の`toolchain`行と実行中のGo versionの一致を検査する。

CI実測では`actions/setup-go`が`go`行の1.26.0を導入し、`GOTOOLCHAIN`が1.26.5へ切り替えていた。一致検査は実効toolchainを見るため、この経路でも正しく1.26.5を確認できている。

### 4.2 package 0件での分岐

`go vet ./...`と`go test ./...`はmatchするpackageが0件のときexit 1になる（`go build`と`go list`は0）。`go list ./...`が空なら未実行を報告して成功する分岐を入れた。P1-01でpackage骨格が入った時点で自動的に実検査へ切り替わる。

### 4.3 外部toolを`tool` directiveで固定した

`go run <module>@<version>`は実行時解決になり、§1の「dependencyは`go.mod`/`go.sum`へ固定」から外れる。`tool` directiveで固定し、`go.sum`のchecksum検証を通す。結果として依存module graphが13件になり、license検査の実入力にもなった。

### 4.4 license検査の対象範囲

`go mod download -json all`の全moduleを対象にする。build対象packageだけへ絞ると、後からimportが増えたとき検査範囲が静かに広がって見落とす。範囲を広く取り、偽陽性は許可listの明示で潰す。

判定はSPDX別の代表条文のexact phraseで行い、license file不在・判定不能・許可list外をいずれも失敗にする（fail closed）。

## 5. CIで見つけた両OS差

ローカル（Linux）だけでは踏めない問題を、一時package`internal/ciprobe`をPRへ入れて実測した。確認後、同じPR内で削除した。

### 5.1 Windowsのgofmtが全Go fileを未format扱いにする

初回runで`lint (windows-latest)`だけが失敗した。

```text
internal\ciprobe\ciprobe.go
internal\ciprobe\ciprobe_test.go
##[error]上のfileがgofmt済みでない
```

Windows runnerのcheckoutが`core.autocrlf`でCRLFへ変換する一方、`gofmt`の出力はLF固定のため、全Go fileが未format扱いになる。P1-01以降のすべてのGo fileに効く問題である。

repository rootへ`.gitattributes`を置き、`* text=auto eol=lf`でtext fileをLF固定にした。既存blobは元からLFのため`git add --renormalize .`で差分は出ないことを確認している。改行をplatform差として扱わず、repository側で1つに固定する方針とした。

### 5.2 `govulncheck`はローカル検証できない

このcontainerからは`vuln.go.dev`へ到達できない。

```text
govulncheck: fetching vulnerabilities:
  Get "https://vuln.go.dev/index/modules.json.gz": Forbidden
```

egress proxyによる遮断で、実装側の問題ではない。CI runnerからは到達でき、package在りの`lint`が両OSでsuccessになったことで実行可能と確認した。

再現command（network制限のない環境で実行する）:

```bash
go tool govulncheck ./...
```

### 5.3 Windowsでrace detectorが使える

`unit (windows-latest)`は`-race`付きで実行された（「race detector 未使用」noticeが出ていない）。`go env CC`による判定はOS名で分岐しないまま両OSで正しく働く。

```text
ok  github.com/kznagamori/go_dev_tool_version_manager/internal/ciprobe  1.063s  coverage: 100.0% of statements
```

## 6. 検証結果

| 検証 | 結果 |
|---|---|
| 全13 run stepを`bash -eo pipefail`で実行（package 0件） | 13件すべて成功 |
| 同（一時package在り） | `go vet`成功、`go test`が`-race`付きで実行、coverage 100.0%、job summary出力 |
| license検査 positive | 13 moduleを判定（Apache-2.0 2件／BSD-3-Clause 10件／MIT 1件） |
| license検査 negative | GPL・license file不在・判定不能の3 fixtureすべてfail |
| license判定 positive | MIT／Apache-2.0／BSD-2-Clause／BSD-3-Clause／ISCの代表条文5件を正しく判定 |
| toolchain一致検査 | `go.mod toolchain=1.26.5 / 実行中=1.26.5` |
| CI（一時package在り、`.gitattributes`前） | 11/12。`lint (windows-latest)`が§5.1で失敗 |
| CI（一時package在り、`.gitattributes`後） | **12/12 success** |
| CI（一時package削除後） | 本PRの最終runで確認する |
| 文書検査・`git diff --check` | 成功。§2以降の番号ずれなし |

## 7. 次タスクへの申し送り

- P1-01でpackage骨格が入ると、`go vet`／`go test`／`govulncheck`が自動的に実検査へ切り替わる。最初のPRでこの3つが両OSでgreenになることを確認する。
- coverageの閾値は実測値が揃ってから別taskで判断する。§1.2は現時点で「閾値でCIを失敗させない」と定めている。
- 依存を追加する場合は、[11-quality-and-ci.md](../11-quality-and-ci.md)§1.3の許可listとの整合を先に確認する。許可list外を入れる場合は許可listと[10-security.md](../10-security.md)のlicense表示契約を同じ変更で更新する。
