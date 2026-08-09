# P1-01 package骨格と依存方向static checkの決定記録

## 1. 目的と対象

[13-progress.md](../13-progress.md) P1-01の実施記録である。規範は[02-architecture.md](../02-architecture.md)§1・§2・§16と[11-quality-and-ci.md](../11-quality-and-ci.md)§1.2・§7.1とする。本書は判断と実測の証跡であり規範仕様ではない。

環境はLinux container（Go 1.26.5、Python 3.11.15）とCI matrix（`ubuntu-latest`／`windows-latest`）。

## 2. package骨格

§2の18論理領域をpackageとして作り、各packageへ責務と依存範囲を書いたpackage documentation commentを置いた（`CLAUDE.md`§9）。

`cmd/gdtvm`は§16の薄層契約をcommentで固定し、9 commandはP8-04で実装する。`main`は`run(io.Writer) int`へ委譲し、exit codeをprocess終了と切り離してtestできるようにした。骨格段階のCLIが`exit 0`を返さないことをtestで固定している。未実装のまま成功を返すと、CIやscriptが実装済みと誤認するためである。正式なexit code体系はP1-04で定める。

## 3. 依存方向static checkの設計

`scripts/ci/check_imports.py`を`policy` jobへ追加した。

### 3.1 不変条件と表の二層構造

§1・§2・§16から一意に決まるものだけを不変条件として常に検査し、それ以外はALLOWED表へ明示登録したものだけを通す。

| 不変条件 | 根拠 |
|---|---|
| `internal/domain`配下は`internal/domain`配下しかimportしない | §1「抽象ポートはcore側が所有し、DomainはCLI・具体的OS API・具体的HTTP clientを参照しない」 |
| どのpackageも`cmd`配下をimportしない | §1「CLI adapterが最外層」 |
| `internal/app`は`internal/platform`をimportしない | §1「Application Serviceは具体的OS APIを参照しない」 |

18領域すべての依存関係は仕様から一意に決められない。§2は責務を定めるが、`internal/install`がApplication Service側かInfrastructure側かといった層の割当てまでは書いていない。18×18のmatrixを今ここで発明すると、根拠のない制約を規範として固定することになる。

そこで表は空から始め、importを増やすtaskがその時点の仕様根拠とともに追記する形にした。表に無いimportも、表に無いpackageの出現も失敗として扱う。「後で決める」が黙って通らないためのfail closedである。

### 3.2 `_test.go`も対象にした

testからのimportも依存方向を壊しうるため対象に含める。fake portのimportだけはtestに許すが、それは`check_policy.py`が別途扱う。

## 4. 自分の検査が自分のcommentを弾いた件

`policy` jobが次を検出した。

```text
internal/shell/doc.go:5: 禁止symbol `HKLM` (HKLM変更)
```

「system環境変数とHKLMを変更しない」という**説明文**が違反として扱われていた。

禁止すべきは実装がその機能を持つことであって、commentでの言及ではない。§9が「なぜ禁止か」をcommentへ書くことを求める以上、正しい説明を書くほど検査が落ちる状態だった。commentを書き換えて回避すると、同じ問題が次に説明を書いた人へ再発する。

`check_policy.py`の検査対象からcommentを除外し、§7.1へ理由とともに規範化した。string literal内の`//`をcomment開始と誤認しないよう、literalの状態も追う。

## 5. 検証結果

| 検証 | 結果 |
|---|---|
| `gofmt -l .` / `go vet ./...` / `go build ./...` | すべて成功 |
| `go test ./... -race -shuffle=on` | 成功。coverage 84.3% |
| 依存方向検査 negative | `domain→platform`、`app→cmd`、`app→platform`、表に無いimport、表に無いpackage の5件がすべて期待どおり失敗 |
| 禁止API検査 positive/negative | commentのみのfileは成功、実codeの違反は検出、string literal内の`//`を誤認しない |
| 全14 run stepを`bash -eo pipefail`で実行 | `govulncheck`以外すべて成功 |
| **CI（両OS 12 check）** | **全て success** |

`govulncheck`はこのcontainerから`vuln.go.dev`へ到達できずローカル検証できない（P0-02で記録済みの制約）。CIで実行されている。

## 6. 次タスクへの申し送り

- P1-02でdomain値（ToolID、Version、Platform、Scope、Mode、Channel、Lifecycle、Digest、InstallKey、EffectiveSelection）と3 version schemeを実装する。`internal/domain`はALLOWED表で内部importを持たない状態から始まる。
- domain値が入ったら、P0-03で`string`と標準library型で定義したport signatureをdomain型へ寄せられるか見直す。
- importを増やすtaskは`scripts/ci/check_imports.py`のALLOWED表へ仕様根拠とともに追記する。追記なしにimportを足すと`policy`が失敗する。
