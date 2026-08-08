# W00-01 仕様再監査記録

> **歴史記録**: 本書は再構成前の文書構成（`docs/README.md` ＋ `01`〜`18`、全19本）を対象とした監査の証跡である。その後の`S00-01`で仕様全体をv0.1 scopeへ再構成し、文書は16本へ統合された。本書が参照する章番号は現在の構成と対応しない。現在の規範は[docs/README.md](../README.md)の文書一覧を正とし、v0.1で削除した機能は[docs/15-deferred.md](../15-deferred.md)を参照する。本書は判断の履歴として保存する。

## 1. 位置づけ

本書は`W00-01`再監査の証跡であり、規範仕様そのものではない。実装時は[docs/README.md](../README.md)と番号付き仕様を正とする。

再監査baselineは`main` commit `8138ab3c74c5b8fe512d578a43344a43eb1e51c5`、開始時worktreeはclean。対象は`docs/README.md`、01～17、root `AGENTS.md`/`CLAUDE.md`、既存監査記録。`anyvm_win/`は規範入力・挙動推測に使用していない。

環境はWindows 10.0.26200 x64、PowerShell 7.6.4、Go 1.26.5 windows/amd64。再監査で18章を新設したため、完了時の規範集合はREADME＋01～18の19文書。

## 2. 利用者が固定した優先順位

1. WindowsとLinuxで開発toolを簡単に導入できる。
2. 初心者が本/サイトに記載された完全版を間違えず導入し、version違い問題を避けられる。「簡単」はcommand数の少なさではなく正確な再現を指す。
3. 中級者以降が各toolの標準設定command/fileを使える。
4. 企業向けsecurity機能は必要最低限。一般利用にも必要なchecksum/path/archive/credential等はfail closedを維持する。

以後の判断はこの順序で比較した。

## 3. 判断記録

| ID | 確定判断 | 主な反映先 |
|---|---|---|
| R01 | 初心者標準は`setup → install tool@exact → use tool@exact → current`。installは`--use`なしで選択しない | 01,04,08,17 |
| R02 | tool home/cache/configをgdtvm管理rootへ隔離する | 03,05,06,12 |
| R03 | upstream checksumがあれば必須。なければprovider/未検証を表示・承認し、取得後digestはrecordのみ | 06,08,11,14 |
| R04 | 初期schemaからPGP/Minisignを除く | 06,11,13 |
| R05 | 専用audit logを除き、secret-mask済みstructured log/receipt/journalを使う | 05,10,11,14 |
| R06 | 初期modeはportable/userだけ。multi-user/system-wideなし | 01,03,09 |
| R07 | Linux user rootはOS user lookup homeの`.local/share/gdtvm`、HOME/XDGで暗黙変更しない | 03,05,09,13 |
| R08 | 初期UIはCLI。core progress/cancel境界だけ残しWails DTO/bridge/event protocolは延期 | 01,02,10,13,18 |
| R09 | 初期releaseは2 archive＋checksums＋2 bootstrap scriptのexactly 5 assetと再現build/公開後検証。SBOM/provenance/attestation/企業approvalは初期対象外 | 11,13,17 |
| R10 | Plan全詳細を維持し、完全版/provider/checksum/warningを冒頭と確認直前に目立たせる | 01,04,08,13 |
| R11 | setupは現在shellだけを1回確認。Windows PATH方式はR28で追加確定 | 04,09 |
| R12 | Windows/Linux共通の検証済みapply childでself-updateする | 08,13,14 |
| R13 | 公式portable artifact優先。要件を満たせない場合だけthird-partyを理由/license付きで採用 | 06,11,12,18 |
| R14 | 初期standard toolはGo、Node.js、Pythonだけ | 01,07,12,16,17 |
| R15 | schema 1は初期3 toolに必要な能力だけ。拡張手順/promptを文書化 | 06,18 |
| R16 | Pythonは両OSともAstral `python-build-standalone`。pip/pip3は選択Pythonの`-m pip`、pip/stdlib/venvを検証 | 06,12,15 |
| R17 | 初期版にruntime local definition/override/pluginなし | 01,06,07,11 |
| R18 | `--json`は完了時単一JSON。human CLIはprogress bar。将来GUIも内部progress/cancelを再利用 | 04,08,10,14 |
| R19 | optional最小global config＋安全な組込みdefault。高度設定追加手順/promptを用意 | 05,14,18 |
| R20 | Go標準proxy環境動作とOS trust store。HTTPS/redirect/limit/maskはgdtvmが強制 | 05,08,11 |
| R21 | 初期正式targetはWindows amd64とLinux amd64/glibcだけ。arm64/muslは追加workflowへ | 01,06,09,13,16,18 |
| R22 | 公式`install.ps1`/`install.sh` bootstrapとmanual archiveを提供 | 01,13,17 |
| R23 | bootstrap既定はuser、manual archive既定はportable | 03,13,17 |
| R24 | 保守promptは汎用、Codex、Claude Codeを用意 | 18,AGENTS,CLAUDE |
| R25 | registryはclient同梱、client release時だけ更新。単体branch/download/updateなし | 07,08,13 |
| R26 | exact not-foundはstrict error＋`available`案内。近似version提案なし | 04,08,17 |
| R27 | EOL/prerelease exactは重要警告/確認後に許可。latestはstableかつlifecycle非EOLだけ | 04,08,12 |
| R28 | Windows setupは`user-path|shell-profile|none`、推奨user-path。Win32 Registry APIでHKCU Path 1 entry、`setx`なし、型/raw/長さ/backup/re-read/rollback | 04,09,11,14,17 |
| R29 | `use <tool> --system`でdefinition駆動のexternal path/version/link identity/siblingを固定。初期版は公式性未検証の`system/external`表示、変化時fail、managed exactで復帰 | 03,04,06,08,12,14 |
| R30 | tool設定は`go env -w`, `npm config`, `pip config`/venv等の上流標準interface。gdtvmは保存先だけredirect | 05,08,12,17 |
| R31 | storage共有/削除はdefinitionで型付き宣言。config/cache/Go global binaryはtool共有・既定保持、Node global package/Python site-packageはversion別・version削除時に同時削除 | 03,06,08,12,14 |
| R32 | Go/Node.js/Pythonだけを具体仕様化し、その他は調査→仕様→schema→実装→両OS検証の手順/promptから追加 | 12,16,18 |

## 4. 再監査中に解消した主な矛盾

### 4.1 対象規模

旧仕様の多数tool/helper/backend/local definition/arm64/musl/Wails必須taskが、利用者優先順位と初期scopeに反していた。standard registryを3 tool・2 platformへ固定し、未着手taskと公開文書表を同期した。

### 4.2 Pythonの再現性

Python公式Windows installerとLinux source buildでは、両OS共通の短時間・非root・portable体験にならない。一方Astral buildは同じCPython完全版を別releaseで再buildし得る。解決として、static registryにCPython完全版＋provider build/asset/digestを固定し、将来同じversionのartifactを黙って差し替えない。問題時はrevocationを使う。

### 4.3 schema不足

初期3 tool自身に必要なGo/Python prerelease grammar、Node platform token、Python static assets、asset digest algorithm、success/path probeが旧最小schemaに不足していた。予約機能ではなく実在要求に限定してschema 1へ追加し、15章例と16章taskを同期した。

### 4.4 Windows PATHとregistry risk

all-shell利便性とregistry破損/肥大化懸念を両立するため、HKCU Path既存value 1件だけにshim directory 1 entryを追加する。版/toolごとのregistry valueは作らず、raw/type/他entryを保持し、24,576 UTF-16 code unitの安全上限、backup、再読、rollback、所有entryだけのremoveを規定した。cmd profileを追加しない。

### 4.5 global package共有

「共通toolは共有」の意図を、全言語で物理共有する危険と分離した。Go binaryは共有、Node native addonはABI差のためversion別、Python packageはX.Y/ABI差のためversion別・venv推奨。engineはtool名ではなくdefinitionのscopeを読む。

### 4.6 初心者表示

Plan詳細を削る案ではなく、重要要約→折畳可能な全詳細→重要要約/確認とし、download/probe/commitのprogressをTTY/非TTYで継続表示する。single JSONのstdoutは汚さない。

### 4.7 system版と設定駆動

`use --system`の候補名、primary、root、required sibling、probeがschemaに無いままではtool ID分岐が必要だった。platform definitionへ最小`system` tableを追加し、path/link cheap identity、SHA-256、完全版、fixed argument fileをsnapshotする。初期版は配布元provenanceを証明しないため、利用者が公式手順で導入したtoolも「非公式」と断定せず`system/external`（公式性未検証）と表示する。

## 5. 変更した規範領域

- README/01: 優先順位、scope、固定判断、受入条件
- 02～05: CLI-only architecture、portable/user root、16 command、最小config
- 06～08: 3 tool用schema、exact registry、install/runtime/system/update
- 09～11: Windows PATH/Linux home、typed Service、必要最低限security
- 12～15: 3 tool完全仕様、quality/release、data contracts、reference definition
- 16～17: 初期scopeの実装順、公開README/USER_GUIDE仕様
- 18: tool追加、不具合、schema/config/platform/GUI拡張と3種agent prompt
- AGENTS/CLAUDE: 初期scopeと18章導線

## 6. 一次資料の確認

仕様判断の補助として次の一次資料を確認した。変化するupstream情報はdefinition作成taskで再確認する。

- Go公式download/install資料: `https://go.dev/dl/`, `https://go.dev/doc/install`
- Node.js公式distribution/API資料: `https://nodejs.org/dist/index.json`, `https://nodejs.org/api/n-api.html`
- Python公式Windows/venv/configure資料: `https://docs.python.org/3/using/windows.html`, `https://docs.python.org/3/library/venv.html`, `https://docs.python.org/3/using/configure.html`
- Astral python-build-standalone repository/running/distribution資料: `https://github.com/astral-sh/python-build-standalone`, `https://github.com/astral-sh/python-build-standalone/blob/main/docs/running.rst`
- Microsoft environment/PATH/setx資料: `https://learn.microsoft.com/en-us/windows/win32/procthread/environment-variables`, `https://learn.microsoft.com/en-us/windows-server/administration/windows-commands/path`, `https://learn.microsoft.com/en-us/windows-server/administration/windows-commands/setx`
- GitHub Release asset API（200/302）: `https://docs.github.com/en/rest/releases/assets`

## 7. 検証証跡

2026-08-07T22:15:33+09:00に次を実行した。検証対象は`AGENTS.md`、`CLAUDE.md`、`docs/**/*.md`の22 Markdown file。

| 検証 | command概要 | 結果 |
|---|---|---|
| code fenceとlink | Python `tomllib`/`json`で全fenceをparseし、prose内relative Markdown linkを解決 | TOML 27、JSON 5、JSON Lines 2、relative link 29、全成功 |
| 初期対象count | 04章command見出し、07章registry TOML、06章platform enum、13章asset許可集合を抽出し期待配列と比較 | command 16、tool 3（go/node/python）、platform 2、release asset 5、全一致 |
| error code | 全文書の`E_*`集合と04章集合を比較 | 全39件、04章不足0 |
| lock順 | 02、03、10章から順序を抽出して比較 | 3章とも`migration,update,state,catalog,install,storage,setup,shim` |
| 旧語検索 | 廃止したexec用tool option、旧tool件数表現、実装先送り表現、未定義data schema表現を固定文字列検索 | すべて0件 |
| 将来enum監査 | system版の将来verified enum候補を検索 | 06章の将来追加gate説明1件だけ |
| storage削除契約 | 全`scope="version"`例の近傍を検索 | 3件すべて`purge="with-version"` |
| whitespace | `git diff --check` | 成功 |
| 変更scope | `git status --short`, `git diff --stat`, branch/HEAD/環境確認 | `main` / `8138ab3c74c5b8fe512d578a43344a43eb1e51c5`、仕様・agent指示・監査recordだけ。Go code/registry実file変更なし |

最終台帳更新後に同じparse/count/error/lock/旧語/diff検証を再実行し、上記結果が変わらないことを確認した。進行中taskは0件。

## 8. 未実施と次task

本taskは仕様監査であり、Go実装、registry実file/schema、fixture、unit/integration/E2E、bootstrap/release assetはまだ存在・実行していない。実装済みとは宣言しない。

W00-01完了後の次taskは`W00-02`（Windows amd64/NTFS/標準user/shell/VS Code評価matrix固定）。
