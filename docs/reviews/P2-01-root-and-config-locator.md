# P2-01 決定記録: mode決定、portable/user root決定、`--home`、config locator

対象task: `docs/13-progress.md` P2-01
規範仕様: [04-storage-and-data.md](../04-storage-and-data.md) §1・§1.1・§1.2、[05-configuration.md](../05-configuration.md) §2・§3、[02-architecture.md](../02-architecture.md) §2・§4.1、[09-platform.md](../09-platform.md) §2

## 1. 実装した範囲

| 対象 | 実装 |
|---|---|
| mode決定 | `internal/config` の `DecideMode`、`ModeRequest`、`ModeDecision`、`ModeSource`（3値） |
| root決定 | `internal/config` の `DecideRoots`、`RootRequest`、`Roots` |
| `--home` | `DecideRoots`内の受入検査（portableとの排他、絶対path、filesystem root、distribution root） |
| config locator | `Roots.ConfigFile`（role=`config`） |
| Known Folder | `port.UserIdentity.AppDataLocal` の追加 |

structured logの担当package確定（利用者判断）も同じbranchに含む。§3.5に記す。

## 2. 判断

### 2.1 `ModeSource`で由来も返す

[04-storage-and-data.md](../04-storage-and-data.md) §1は優先順位を「CLIの一時`--mode`、有効なsetup state、導入経路の既定」と定める。決まった値だけを返すと`doctor`がmodeの由来を説明できないため、3件をそのまま`ModeSource`の3値にして一緒に返す。

global `gdtvm.toml`はmode keyを持たない（同§）ため、設定fileは`ModeRequest`の入力に含めない。

### 2.2 distribution rootは実行fileのdirectory

[04-storage-and-data.md](../04-storage-and-data.md) §1.2は「bootstrapによるactive distribution rootは`<data-root>/distribution/current`」と書くが、[05-configuration.md](../05-configuration.md) §2は「**active distribution rootの`gdtvm[.exe]`と同じdirectory**にある`gdtvm.toml`だけをglobal設定として読む」と定める。

実装は後者を基準にした。実行中binaryの位置を正にしないと、どのbinaryが動いているかとどのconfigを読むかがずれる。`<data-root>/distribution/current`はbootstrapがそこへ配置する結果として満たされるものであり、client側が計算する値ではない。

`--home`を与えてもdistribution rootは動かない。§1が「`user --home`はその実行だけdata rootを上書きする」と定めるためである。

### 2.3 `port.UserIdentity`へ`AppDataLocal`を追加

§1.2はuser modeのdata rootをWindowsで`LocalAppData`直下`gdtvm`と定めるが、`LocalAppData`（`C:\Users\x\AppData\Local`）はaccount home（`C:\Users\x`）とは別directoryで、既存の`Home` fieldでは代用できなかった。

[02-architecture.md](../02-architecture.md) §4.1がKnown Folderの取得をUserLookup portの責務としているため、OS差をport側で吸収した。Linuxには対応概念が無いので空にし、`Home`直下`.local/share/gdtvm`を使う。

値が取れない場合（Windowsで`AppDataLocal`が空、Linuxで`Home`が空）は`E_FILESYSTEM`で失敗させる。§1.2が「`HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`で暗黙置換しない」と定めるため、環境変数で代用しない。

### 2.4 path区切りは引数のplatformから決める

`runtime.GOOS`ではなく`RootRequest.Host`（`domain.Platform`）から区切りを決める。どちらのrunnerからでも両OSの規則をtestできるようにするためである（`CLAUDE.md` §5「WindowsとLinuxを同時に開発する」「片方のOSだけを回す分岐を作らない」）。

`joinPath`、`isAbsolutePath`、`isFilesystemRoot`、`samePath`が区切りを引数で受ける。Windowsはdrive付き・UNC・case insensitive、Linuxは`/`始まり・case sensitiveとして扱う。`path/filepath`はhostの区切りを使うため、Linux runnerからWindowsのpathを組み立てられない。

### 2.5 `--home`の検査をP2-01とP2-03へ分けた

§1.2は`--home`の拒否条件を6件挙げる。

| 条件 | 担当 |
|---|---|
| 絶対pathでない | P2-01 |
| filesystem root | P2-01 |
| distribution rootそのもの | P2-01 |
| 他user所有 | P2-03 |
| network share | P2-03 |
| symlink/reparse loop | P2-03 |

filesystem操作を要する3件をP2-03（「root layout/containment/owner/reparse/unsafe filesystemを実装・negative testする」）へ送った。台帳の既存の分担に合わせたものであり、条件を落としたわけではない。`DecideRoots`は`port.FileSystem`を受け取らない設計にして、この分担を型で示している。

### 2.6 structured logの担当は`internal/store`

[04-storage-and-data.md](../04-storage-and-data.md) §18のJSON Lines形式と[10-security.md](../10-security.md) §12のrotation/保持上限について、書き出し先packageが[02-architecture.md](../02-architecture.md) §2の18論理領域に無く、担当taskも台帳に無かった（P1-04から持ち越し）。

利用者判断で`internal/store`へ割り当てた。同packageは既に「state、catalog、receipt、atomic write」でdata root配下の永続fileをすべて持っており、新packageを増やさずに済む。§2の責務文へ「structured logの出力とrotation」を追記し、serializationをP2-04、rotation・保持上限をP2-05のscopeへ追記した。

### 2.7 `check_imports.py`のALLOWED表

`internal/config` → `internal/domain`、`internal/domain/port` を追加した。root決定がmode/platform/path roleのdomain値と、OS user lookupの結果（`port.UserIdentity`）を使うためである。

## 3. 検証

### 3.1 CI

PR #31（commit `7ae0035`、workflow run 31672152652）で、6 job×2 OSの **12 checkすべてがsuccess** になった。

### 3.2 ローカル検証

Linux container（Go 1.26.5、Python 3.11.15）で実行した。

| command | 結果 |
|---|---|
| `gofmt -l .` | 0 file |
| `go vet ./...` | 成功 |
| `go test ./... -race -shuffle=on -covermode=atomic -coverprofile=coverage.out` | 成功。total 91.9% |
| `scripts/ci/check_policy.py` | 成功。production Go file 55件 |
| `scripts/ci/check_imports.py` | 成功。package 20件 / internal import 28件 |
| `scripts/ci/check_docs.py` | 成功。file 30件 |
| `scripts/ci/check_pr_refs.py --head claude/feature-p2-01-root-and-config-locator --base claude/work` | 成功 |
| `scripts/ci/check_licenses.py` | 成功。module 13件 |
| `git diff --check` | 差分なし |

package別coverage: `internal/progress` 100.0%、`internal/security` 100.0%、`internal/domain` 95.8%、`internal/app` 94.9%、`internal/config` 92.8%、`internal/domain/port` 92.7%、`internal/domain/port/fake` 86.1%、`cmd/gdtvm` 66.7%。

test件数（subtest込み）: `internal/config` 39件。

### 3.3 主なnegative test

| 対象 | 内容 |
|---|---|
| mode | override/setup state/導入経路の既定それぞれで不正値、既定が空 |
| `--home` | portableとの併用、相対path（両OS）、単なる名前、filesystem root（Linux `/`、Windows drive root、UNC share）、distribution rootそのもの（末尾区切り、Windows case違い） |
| `paths.user_data_root` | 相対path（`E_CONFIG_INVALID`） |
| OS user lookup | Windowsで`AppDataLocal`が空、Linuxで`Home`が空（いずれも`E_FILESYSTEM`） |
| request | executable dirが空、host未設定、mode未設定 |
| 環境変数 | `HOME`/`XDG_DATA_HOME`/`XDG_STATE_HOME`/`XDG_CACHE_HOME`を設定してもrootが変わらないこと |

返すtyped errorはすべて`Error.Validate()`を通ることも確認している。

## 4. 未実施・制約

- `go tool govulncheck ./...` はこのcontainerから`vuln.go.dev`が`Forbidden`で到達できず未実行である（P0-02から継続）。検証手段はCIだけである。
- rootのfilesystem安全検査（owner、既存親chainの他user/world書込み可能性、network share、symlink/reparse loop、現在userが書けないrootの拒否、canonical化）は未実装で、P2-03の範囲である。`DecideRoots`が受け取る`ExecutableDir`は、呼出し側がport経由でrealpathへ解決済みであることを前提にしている。
- setup stateの読込みは未実装のため、`DecideMode`は呼出し側から`*domain.Mode`を受け取る形にしている。実際の読込みはP2-04のcodecが入ってから接続する。
- Windowsでの実行はCI matrixでのみ確認する。
