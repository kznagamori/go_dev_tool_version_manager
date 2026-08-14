# P2-04 決定記録（1/3）: 共通codec層とstate TOML・structured log

対象task: `docs/13-progress.md` P2-04（3分割の1本目）
規範仕様: [04-storage-and-data.md](../04-storage-and-data.md) §7・§8・§9・§10・§11・§12・§13・§17.1・§18・§21・§22、[02-architecture.md](../02-architecture.md) §2

## 1. 3分割の判断

着手時に規範仕様を読んだ結果、P2-04の対象が[04-storage-and-data.md](../04-storage-and-data.md) §8〜§18の**12形式・150 field超**で、これまでのtaskの3〜4倍あることが分かった。1 PRではdiffがreview不能な規模になるため、利用者判断で3 PRへ分割した。

| # | 範囲 | branch |
|---|---|---|
| 1（本PR） | 共通codec層、§8 `schema.toml`、§9 `setup.toml`、§10 setup backup、§11 `selections.toml`、§12 `shim-index.toml`、§13 `receipt-index.toml`、§18 structured log | `claude/feature-p2-04-storage-codecs` |
| 2 | §14 install receipt、§15 catalog JSON | `claude/feature-p2-04-receipt-catalog` |
| 3 | §16 Plan、§17 CLI JSON envelope | `claude/feature-p2-04-plan-envelope` |

task IDはP2-04のままとし、台帳項目は3本目のmerge後に`[x]`とする。

## 2. 実装した範囲

`internal/store`を新設した。[02-architecture.md](../02-architecture.md) §2が本領域へ「state、catalog、receipt、atomic write、structured logの出力とrotation」を割り当てている。本PRはそのうちcodec、すなわちbytesとtyped valueの相互変換だけを持つ。

| file | 対象 |
|---|---|
| `codec.go` | strict TOML/JSON decode、§7のscalar検査、末尾LF正規化、size上限 |
| `fields.go` | ID/version/command名/enumの共通field検査、`InstallRef` |
| `state.go` | §8・§9・§11・§12・§13のparse/encode |
| `backup.go` | §10 setup backupのparse/encode |
| `log.go` | §18 structured log JSON Linesのencode/decode |

atomic write、revision採番、破損復旧、log rotationは**P2-05**の範囲であり、本packageはfilesystemもnetworkも触らない。

## 3. 判断

### 3.1 `encoding/json`の既定を3か所塞ぐ

§7は「unknown/duplicate key、型違い、enum外、上限超過、trailing dataを拒否する」と定めるが、`encoding/json`の既定はその3つを通す。

| §7が拒否するもの | `encoding/json`の既定 | 本PRの対処 |
|---|---|---|
| unknown key | 無視する | `Decoder.DisallowUnknownFields()` |
| duplicate key | 後勝ちで受理する | `checkDuplicateJSONKeys`によるtoken走査 |
| trailing data | 1 document目だけ読んで残りを見ない | decode後に`Token()`が`io.EOF`を返すことを確認 |

重複keyを後勝ちで許すと、同じ文書が実装ごとに別の値へ読める。走査ではobject階層ごとにkey集合を持ち、arrayの層はnilを積んで階層だけ合わせる。key直後のvalueを1件読み飛ばすのは、値がstringのときにkeyと誤認しないためである。「別objectの同名key」「array内の別要素で同名key」「値がkeyと同名」が通ることをtestで固定し、誤検出しないことを確かめた。

TOMLは`go-toml/v2`が重複key/table、型違い、trailing dataを既定で拒否するため、`DisallowUnknownFields()`だけを足した。

### 3.2 versionを`domain.InstallKey`ではなくtextで持つ

state fileの`version`は`InstallRef`としてtextで保持する。versionのschemeは**tool definitionが決める**（[06-tool-definition.md](../06-tool-definition.md) §4）ため、definitionを持たないcodec層ではschemeを一意にできない。`domain.Version`はschemeを要求するので、ここで使うとschemeを推測することになる。

判定範囲はP2-02の`ParseProjectConfig`と同じにした。

| 拒否する | 例 |
|---|---|
| 空、前後空白 | `""`、`"22.18.0 "` |
| `latest` | `"latest"` |
| range/wildcard記号（`* ^ ~ < > = , \|` と空白） | `"^22.0.0"`、`">=22"`、`"22.*"` |

**`22.x`のようなrange記号を含まない部分versionは通す。** schemeを知らなければ完全versionと区別できず、「`x`を含めば拒否」のような規則を足すとschemeによっては正当な値を拒否する。この境界が動いたら気付けるよう、通ることを`TestSelectionsAcceptsSchemeDependentVersions`で明示的に固定した。scheme検証はP3の責務である。

### 3.3 encode経路もparseと同じbuild関数を通す

`Encode*`は書出し前に`build*`を呼び、parseと同じ検査をかける。parse側だけを検査していると、programが組み立てた不正な値がそのままfileになる。書いた瞬間はerrorにならず、次回の読込みで破損として現れるため原因の特定が難しくなる。

**この検査が実際に欠陥を1件検出した。** Goのzero `time.Time`は`0001-01-01T00:00:00Z`へserializeされ、RFC 3339としては妥当に見えるため、時刻を入れ忘れたstateが西暦1年のtimestampとして黙って保存されていた。`parseTimestamp`でzero時刻を拒否し、読書きの両方で塞いだ。

### 3.4 timestampのoffsetを`Z`に限る

§7は「UTC RFC 3339、秒精度以上、offset `Z`」と定める。`+00:00`もUTCだが拒否する。同じ時刻に2通りの表現を許すと、byte列の比較でstateの同一性を判定できなくなるためである。小数秒は「秒精度以上」に含まれるため通す。

### 3.5 「不存在」の表現を1通りに固定する

§9のintegration identityと§10のbackupは、対象が存在しない場合の表現を定める。

| 対象 | 不存在の表現 |
|---|---|
| integration identity（§9） | `kind = "none"`、他stringは空、digestは64 zero |
| setup backup（§10） | `existed = false`、raw空、digest 64 zero |

これを検査しないと、rollbackが何を書き戻すべきか決まらない。`existed=true`なのにdigestが64 zero、`existed=false`なのにrawがある、といった組合せも拒否する。

backupのbase64 decode後のsize上限はdecode**後**に見る。base64は4/3へ膨らむため、encode後だけを見ていると上限を実質1.33倍に緩めてしまう。paddingなしの`RawStdEncoding`も拒否する。同じbyte列に2通りの表現ができると、digestとfile内容の対応が1対1でなくなる。

### 3.6 structured logは`port.LogRecord.Validate`を正本にする

§18の不変条件はP1-04で`port.LogRecord.Validate`として実装済みである。codec側で同じ検査を書き直すと、片方だけが緩む余地ができる。`EncodeLogLine`は`Validate`を呼び、`DecodeLogLine`はdecode後にも`Validate`を呼ぶ。書けない形のrecordを読めてしまうと、出力側の検査が空振りするためである。

componentのgrammarもP1-04の判断（「§18は値のgrammarを定めていないため、空でないことだけを要求する」）に従い、codec層で独自grammarを足さなかった。仕様に無い規則を2か所目で作らないためである。

`operation_id`は変更transactionだけが持つ。読取りcommandでは空文字列で出す。keyを落とすとexact key集合から外れるためである。

### 3.7 entryの整列と一意性を読書き両方で検査する

§7は「mapを永続化するときはkey UTF-8 byte順…で出力する。parserは順序に意味を持たせない」と定める。parserは順序に意味を持たせないが、**整列と一意性は検査する**。整列していないfileを受理すると、書き戻しでdiffが暴れる。

| file | keyと規則 |
|---|---|
| §11 selections | tool ID byte順、toolごとに最大1件 |
| §12 shim-index | command name byte順で一意 |
| §13 receipt-index | tool/version/platform tupleのbyte順で一意 |

receipt-indexはversionの数値としての大小ではなく**byte順**である（`20.0.0` < `22.18.0`）。書出し側は同じkeyで整列するため、同じ内容から同じbyte列が出ることを`TestEncodeIsDeterministic`で固定した。

### 3.8 診断へ実pathを載せない

parse/encodeの失敗は`E_STATE_CORRUPT`とし、`PathRole`だけを載せて実pathをmessage parameterへ入れない（[10-security.md](../10-security.md) §9.2、P2-02・P2-03と同じ方針）。原因は`Cause`へ入れ、`Error()`が`Cause`を含めないことで公開文へ漏れない（P1-04）。TOML破損には行・列を付ける（[05-configuration.md](../05-configuration.md) §1と同じ扱い）。破損時に利用者が該当行を特定できるようにするためである。

## 4. 検証

### 4.1 ローカル検証

Linux container（Go 1.26.5、Python 3.11.15）で実行した。

| command | 結果 |
|---|---|
| `gofmt -l .` | 0 file |
| `go vet ./...` | 成功 |
| `go test ./... -race -shuffle=on -covermode=atomic -coverprofile=coverage.out` | 成功。total 92.2% |
| `scripts/ci/check_policy.py` | 成功。production Go file 64件 |
| `scripts/ci/check_imports.py` | 成功。package 20件 / internal import 52件 |
| `scripts/ci/check_docs.py` | 成功。file 33件 |
| `scripts/ci/check_licenses.py` | 成功。module 14件 |
| `git diff --check` | 差分なし |

package別coverage: `internal/progress` 100.0%、`internal/security` 99.0%、`internal/domain` 95.8%、`internal/app` 94.9%、`internal/config` 93.3%、`internal/domain/port` 92.7%、`internal/store` 91.6%、`internal/domain/port/fake` 86.1%、`cmd/gdtvm` 66.7%。

test件数: `internal/store` 154件（subtest込み）、うちnegative subtest 112件。

CI（PR）の結果は§4.3へ追記する。

### 4.2 主なnegative test

| 対象 | 件数と内容 |
|---|---|
| §8 `schema.toml` | 18件。schema欠落/2、unknown key、重複key、revision型違い/負、root_idの長さ/大文字/欠落、mode enum外、created_atの非UTC/date形式、client_version不正、state/receipt/catalog schemaの不一致・欠落、BOM、空file |
| §9 `setup.toml` | 15件。unknown top-level/identity key、integration_identity欠落、path_integration enum外、shim_pathが他値、backup_id不正、identity kind enum外、digestの長さ/大文字、shell整合3件（user-pathなのにshellがある／shell-profileなのに空／enum外）、kind=none整合3件 |
| §10 backup | 14件。unknown key、kind enum外、target空、base64でない、padding無しbase64、backup_id不正、existed欠落/型違い、不存在整合5件、value_typeがshell-profileにある、decode後のsize上限 |
| §11 selections | 12件。version 8件（空・`latest`・`^`・`>=`・`~`・`*`・前後空白・カンマ）、tool降順、同一tool重複、install_id不正、同一tool 2件（encode側） |
| §12 shim-index | 9件。name降順/重複/大文字/区切り/空、tool_idがalias形式、receipt_index_revision欠落、client_version欠落、unknown key |
| §13 receipt-index | 10件。health enum外、path絶対/backslash/相対参照/空、receipt_sha256が上流形式、platform_id未対応、install_id欠落、tuple降順、tuple重複 |
| §18 log | 25件。decode 16件（unknown key、重複key、schema 2、level enum外、time非UTC、invocation_id不正、message_id segment 1件、component空、fieldsのnested object/array/小数、field key grammar外、trailing data、BOM、不正UTF-8、空）、encode 9件 |
| 共通codec層 | 32件。JSON重複key 5件（top-level/nested/array内/値がstring/深いnest）、trailing data 3件、timestamp 8件、relative path 12件、非負integer 2件、BOM/不正UTF-8 2件 |
| 上限 | 4件。state TOML 1 MiB、log 1行256 KiB（読書き）、backup raw |

誤検出しないことも同じtestで確認している。JSON重複key検査は「別objectの同名key」「array内の別要素で同名key」「値がkeyと同名」を通し、上限検査は境界ちょうどを通す。

仕様の例（§8〜§13・§18）をそのまま読める、という形でpositive testを書いた。読めなければ仕様か実装のどちらかがずれている。

P1-03のglobal state検査が新規package変数9件を検出したため、根拠を添えて許可表へ追加した。P2-02の`colorModes`、P2-03の`windowsReservedNames`に続き3回目で、この検査が空振りしていないことの実地確認になっている。

### 4.3 CI

<!-- PR作成後にworkflow runとcheck結果を追記する -->

## 5. 未実施・制約

- `go tool govulncheck ./...` はこのcontainerから`vuln.go.dev`が`Forbidden`で到達できず、ローカルでは未実行である（P0-02から継続）。本PRは新規依存を追加していない。
- §14 install receiptと§15 catalog JSONは**2本目**、§16 Planと§17 CLI JSON envelopeは**3本目**の範囲である。`ReceiptFileMaxBytes`のような先行定義も本PRには置かない（`CLAUDE.md` §7「未使用のenum値、kind、fieldを『将来のため』に残さない」）。
- §19 lock metadataはlockの責務であり**P2-05**の範囲とした。P2-04のtask行が列挙する対象に含まれていない。
- atomic write、revision採番、backupの世代管理、破損復旧、log rotationと保持上限は**P2-05**である。本packageはbytesを受け取りbytesを返す純関数として保つ。
- 実際のfile読書きは呼出し側が`port.FileSystem`経由で行う。本packageはportを持たない。
- `ToolID`の長さ上限は仕様に規定が無く未設定のままである（P1-02から継続）。
- Windowsでの実行はCI matrixでのみ確認する。
