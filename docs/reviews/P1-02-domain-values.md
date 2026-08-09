# P1-02 domain valueと3 version schemeの決定記録

## 1. 目的と対象

[13-progress.md](../13-progress.md) P1-02の実施記録である。規範は[02-architecture.md](../02-architecture.md)§3、[06-tool-definition.md](../06-tool-definition.md)§4・§5、[04-storage-and-data.md](../04-storage-and-data.md)§6・§17.1・§17.2とする。本書は判断と実測の証跡であり規範仕様ではない。

環境はLinux container（Go 1.26.5）とCI matrix（`ubuntu-latest`／`windows-latest`）。

## 2. 実装した値

すべて検証付きconstructorを通す型にし、zero値が有効値にならないようにした。parse前の生文字列と検証済み値を型で区別できないと、未検証の値がreceiptやstateへ書かれてしまう。

| 型 | 規範 | 要点 |
|---|---|---|
| `ToolID` | §3 | 正規化済みkebab-case。aliasを保持できない |
| `Version` | §3、§4 | 正規文字列とcomparison keyの両方を保持 |
| `Platform` | §3、§5 | id/os/arch/libcを固定表で閉じ、実行形式suffixを持つ |
| `Digest` | §3、§6 | upstreamは`<algorithm>:<hex>`、内部はSHA-256固定のhexのみ |
| `PathRole` / `PathValue` | §17.2 | 22 role。`WithoutPath`で個人pathを落としroleだけ残す |
| `InstallKey` | §3 | ToolID＋Version＋Platform |
| `EffectiveSelection` | §3、§17.1 | 選択値・由来・由来設定file・導入状態 |
| enum parser | §17.1 | Mode / Scope / Channel / Lifecycle / SelectionSource / Health |

## 3. version比較の設計

### 3.1 正規文字列とcomparison keyの分離

[06-tool-definition.md](../06-tool-definition.md)§4は「入力versionはcatalogの正規文字列完全一致であり、comparison keyへ変換した近似一致をしない」と定める。`Version`は正規文字列とkeyの両方を持ち、**一致判定は文字列、順序判定はkey**と用途を分けた。

goの`1.20`と`1.20.0`はcomparison keyが同値になるが、`InstallKey.Equal`は正規文字列で判定するため別物として扱われる。この性質をtestで固定した。

### 3.2 schemeごとの比較順

| scheme | 比較順 |
|---|---|
| `go` | major/minor → `beta < rc < final` → prerelease番号 → finalのpatch（省略は0） |
| `python` | 数値3要素 → `a < b < rc < final` → prerelease番号 |
| `semver` | SemVer 2.0.0のprecedence。prerelease無しが大きい |

prerelease段階を整数の`stage`で表し、finalを最大にした。goでは`stage`をpatchより先に比較することで、「prereleaseにpatchは無く、finalのpatchだけを比較する」という§4の規則をそのまま表現できる。

### 3.3 異なるscheme同士の比較はerror

暗黙に片方の規則で比較すると誤った「最新」を選ぶため、schemeが異なる`Compare`はerrorにした。未初期化versionとの比較も同様に拒否する。

## 4. 型で防いでいる状態

- `Digest.Equal`はalgorithmの一致も要求する。sha256とsha512をhexだけで比べない
- `Digest.Internal()`はSHA-256以外でerrorにする。内部digestに`<algorithm>:`を付けない形式の取り違えを検出する
- `EffectiveSelection`は「選択ありで`source=none`」「origin未設定」を作れない
- `InstallKey`は3要素のいずれかが未設定なら作れない
- `PathValue`は22 role以外を受理しない。pathの空は許す（typed errorがroleだけを伝えるため）

`PathValue`はpathの絶対性や正規性を検査しない。OS nativeなpath規則はplatform層の責務であり、domainがOS依存の判定を持つと§1の依存方向に反するためである。

## 5. 仕様に無く判断した点

`ToolID`の長さ上限を設けなかった。§3は「正規化済みkebab-case」、[06-tool-definition.md](../06-tool-definition.md)§3は「`id`はfile basenameと一致」と定めるだけで、上限の規定がない。根拠のない数値を規範として固定するより上限なしのほうが安全と判断し、PR本文で明示して確認を求めた。

## 6. 検証結果

| 検証 | 結果 |
|---|---|
| `gofmt -l .` / `go vet ./...` | 成功 |
| `go test ./... -race -shuffle=on` | 成功。domain package **coverage 94.8%**、全体 88.0% |
| version grammar | positive 13件・negative 18件（leading `v`、build metadata、leading zero、prerelease番号0、patchとprereleaseの併用、hyphen区切り、未定義scheme など） |
| 比較順序 | 3 schemeの昇順列（go 8件・python 7件・semver 11件）で隣接ペアの双方向を確認 |
| scheme混在・未初期化の比較 | いずれもerror |
| enum parser | 6 enumのpositive/negative |
| `path_role` | 22件であることを件数で固定 |
| 全14 run stepを`bash -eo pipefail`で実行 | `govulncheck`以外すべて成功 |
| **CI（両OS 12 check）** | **全て success** |

`govulncheck`はこのcontainerから`vuln.go.dev`へ到達できずローカル検証できない（P0-02で記録済みの制約）。CIで実行されている。

## 7. 次タスクへの申し送り

- P1-03でportの依存注入（`NewServices`とPorts組立て）とpackage global mutable state不存在をtestする。
- P0-03で`string`と標準library型で定義したport signatureを、本taskのdomain型（`PathValue`、`Digest`等）へ寄せられるか見直す。寄せる場合は`internal/domain/port`から`internal/domain`へのimportが増えるため、`scripts/ci/check_imports.py`のALLOWED表へ追記が必要になる。
- `ToolID`の長さ上限は未規定のまま。必要になった時点で§3または[06-tool-definition.md](../06-tool-definition.md)§3へ規範化する。
