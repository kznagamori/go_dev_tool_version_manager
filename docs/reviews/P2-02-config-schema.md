# P2-02 決定記録: global/project TOML schemaのstrict実装とGit境界探索

対象task: `docs/13-progress.md` P2-02
規範仕様: [05-configuration.md](../05-configuration.md) §1・§2・§3・§4・§4.1、[04-storage-and-data.md](../04-storage-and-data.md) §7・§21、[02-architecture.md](../02-architecture.md) §17、[03-cli.md](../03-cli.md) §7

## 1. 実装した範囲

| 対象 | 実装 |
|---|---|
| global schema 1 | `internal/config` の `ParseGlobalConfig`、`GlobalConfig`、`DefaultGlobalConfig`、`ColorMode`（3値） |
| project schema 1 | `internal/config` の `ParseProjectConfig`、`ProjectConfig` |
| Git境界探索 | `internal/config` の `FindProjectFile`、`ProjectSearchRequest`、`ProjectSearchResult` |
| 外部module | `github.com/pelletier/go-toml/v2` v2.4.3 の採用と[11-quality-and-ci.md](../11-quality-and-ci.md) §1.6の新設 |

## 2. 利用者判断: TOML parser

このprojectで最初のruntime外部依存のため、[02-architecture.md](../02-architecture.md) §17に従って実装前に確認した。

`github.com/pelletier/go-toml/v2` v2.4.3 に決まった。[05-configuration.md](../05-configuration.md) §1が求める「unknown key、重複key/table、型違い、enum外、上限外を**位置付き**`E_CONFIG_INVALID`として拒否」を、`Decoder.DisallowUnknownFields()`と行・列を持つ`DecodeError`/`StrictMissingError`でそのまま満たせることが選定理由である。TOML 1.0のdatetime、多行文字列、inline table、array of tablesを自前parserでstrictに実装するより誤りが少ない。

§17が求める記録項目は[11-quality-and-ci.md](../11-quality-and-ci.md) §1.6として新設した。記録場所は§17が定めていないため、§1.3のlicense許可listの隣に置いた。moduleを増減させるtaskが同じ場所を更新できる。

実測は次のとおりである。

| 項目 | 結果 |
|---|---|
| SPDX license | MIT（§1.3の許可listに含まれる） |
| transitive dependency | 0件（`go mod graph`はGo 1.21以上の要求だけを示す） |
| `check_licenses.py` | 成功。module 13→14件、MIT 1→2件 |
| `govulncheck`（CI `lint`） | `No vulnerabilities found.` |

## 3. 判断

### 3.1 table/keyをpointerで持つ

`globalFile`の全fieldをpointerにした。未設定と明示的なzero値を区別するためである。区別しないと`stop_at_vcs_root = false`と未設定が同じ扱いになり、既定値`true`で上書きされてしまう。`auto_install_on_use = false`も同じである。

戻り値の`GlobalConfig`はpointerを持たず、既定値で埋めたあとの値だけを返す。「設定されたか」を呼出し側へ見せると、§2の優先順位を各keyで再実装させることになる。

### 3.2 VCS marker検査をproject file検査の後に置く

同じdirectoryを「project file → `.git`」の順で見る。[05-configuration.md](../05-configuration.md) §4.1は停止条件を定めるが、同一directory内での判定順を明示していない。worktree root直下の`.gdtvm.toml`は境界の**内側**にあるため使うのが正しく、marker検査を先に置くと使えなくなる。この解釈をtestで固定した。

`.git`はdirectoryとfileの両方を認識する（同§）。判定を存在検査だけで行うため、submoduleやworktreeで`.git`がfileになっていても同じく境界として扱える。

### 3.3 version検証をscheme非依存の範囲に限る

§4は「値はcatalog正規完全versionだけ。latest、channel、range、配列、provider、storage設定を保存しない」と定めるが、**schemeごとのgrammarはtool definitionが決める**（[06-tool-definition.md](../06-tool-definition.md) §4）。definitionはP3で入るため、P2-02ではschemeに依存せず判定できるものだけを拒否した。

| 拒否する | 判定方法 |
|---|---|
| 配列、table、数値 | decode時の型違い |
| 空文字列 | 値の検査 |
| 前後空白 | 値の検査 |
| `latest` | 値の検査 |
| range/wildcard記号（`* ^ ~ < > = , \|` と空白） | 値の検査 |

同様に「aliasを保存したfileは`E_PROJECT_CONFIG_INVALID`」の判定は**registryを要する**ためP4へ送り、ここではkebab-case grammar（`domain.ParseToolID`）までを見る。分担はdocumentation commentへ書いた。

### 3.4 globalとprojectでerror codeを分ける

[03-cli.md](../03-cli.md) §7の終了code表はglobal設定を`E_CONFIG_INVALID`、project設定を`E_PROJECT_CONFIG_INVALID`とし、どちらも終了code 3である。codeを分けるのはどちらのfileが悪いかを利用者が区別できるようにするためであり、まとめない。

### 3.5 `paths.user_data_root`の検査範囲

§3.2は「filesystem root、distribution root、network share、他user所有、symlink/reparse loopを拒否」と定めるが、filesystem操作を要する分は**P2-03**の範囲とした（P2-01と同じ分担）。P2-02で行うのは、portableでの非空拒否（§3.2「user modeだけに許し、portableでは非空を拒否する」）と絶対path判定である。

### 3.6 探索でStat失敗を握りつぶさない

§4.1は「symlink loop、permission error、競合caseを明確に失敗させる」と定める。`Stat`が`fs.ErrNotExist`以外を返したら`E_FILESYSTEM`にする。読めなかったことを「無かった」として通すと、権限の問題でproject fileが黙って無視される。

typed errorには`PathRole`だけを載せ、実pathをmessage parameterへ入れない（[10-security.md](../10-security.md) §9.2）。testでparametersに実pathが現れないことを確認している。

## 4. 検証

### 4.1 CI

PR #34（commit `f45e92f`、workflow run 31687583573）で、6 job×2 OSの **12 checkすべてがsuccess** になった。

新規依存に対する検査結果は次のとおりである。

- `go tool govulncheck ./...`（`lint` job）: `No vulnerabilities found.`
- `scripts/ci/check_licenses.py`: 検査module 14件（Apache-2.0 2件、BSD-3-Clause 10件、MIT 2件）で成功

### 4.2 ローカル検証

Linux container（Go 1.26.5、Python 3.11.15）で実行した。

| command | 結果 |
|---|---|
| `gofmt -l .` | 0 file |
| `go vet ./...` | 成功 |
| `go test ./... -race -shuffle=on -covermode=atomic -coverprofile=coverage.out` | 成功。total 92.3% |
| `scripts/ci/check_policy.py` | 成功。production Go file 57件 |
| `scripts/ci/check_imports.py` | 成功。package 20件 / internal import 36件 |
| `scripts/ci/check_docs.py` | 成功。file 31件 |
| `scripts/ci/check_pr_refs.py --head claude/feature-p2-02-config-schema --base claude/work` | 成功 |
| `scripts/ci/check_licenses.py` | 成功。module 14件 |
| `git diff --check` | 差分なし |

package別coverage: `internal/progress` 100.0%、`internal/security` 100.0%、`internal/domain` 95.8%、`internal/app` 94.9%、`internal/config` 93.9%、`internal/domain/port` 92.7%、`internal/domain/port/fake` 86.1%、`cmd/gdtvm` 66.7%。

test件数（subtest込み）: `internal/config` 114件。

### 4.3 主なnegative test

| 対象 | 件数と内容 |
|---|---|
| global strict | 30件。schema欠落/不一致/型違い/空file、unknown top-level key・table・table内key、重複key・重複table、colorのenum外/大文字/真偽値、`filename`の他値、durationの非duration/下限未満/上限超過/負、`cache_max_bytes`・`max_files`・`max_bytes_per_file`の境界外、log levelのenum外、壊れたTOML |
| 位置付き診断 | 3件。unknown key、型違い、重複keyで`detail`に行・列が入ること |
| file上限 | ちょうど1 MiBは通し、+1 byteで拒否 |
| project strict | 18件。schema欠落/不一致/空file、unknown key・table、重複key、tool IDの大文字/underscore/空、値の配列/table/数値/空/`latest`/range/caret/wildcard/前後空白 |
| 探索 | 最近傍1件のみ採用、VCS rootで停止、`stop_at_vcs_root=false`で越える、worktree root直下のfileは使う、Windows drive rootで打ち切る、`--project-file`と`--no-project`の併用は`E_USAGE`、相対`--project-file`拒否、Statのpermission errorを`E_FILESYSTEM`にして実pathをparametersへ入れない、request不備4件 |

仕様の例をそのまま読んだ結果が`DefaultGlobalConfig()`と一致することもtestで固定した。§3は各keyの右コメントを既定値としているため、一致しなければどちらかが仕様とずれている。

P1-03のglobal state検査が新規の`colorModes`を検出したため、根拠を添えて許可表へ追加した。検査が空振りしていないことの実地確認になっている。

## 5. 未実施・制約

- `go tool govulncheck ./...` はこのcontainerから`vuln.go.dev`が`Forbidden`で到達できず、ローカルでは未実行である（P0-02から継続）。本taskは新規依存を追加したため、CI `lint` jobの結果（§4.1）を一次証跡とした。
- `paths.user_data_root`と`--home`のfilesystem検査（owner、network share、symlink/reparse loop）、およびpathのcanonical化はP2-03の範囲である。
- tool IDの実在性とalias判定はP4のregistry読込み、version schemeの検証はP3のdefinitionが担当する。
- global設定fileの読込み自体（`port.FileSystem`経由の`ReadFile`と不在時の既定値適用）は`Initialize`の責務であり、本taskはbytesを受け取る関数までとした。接続はP8-01で行う。
- Windowsでの実行はCI matrixでのみ確認する。
