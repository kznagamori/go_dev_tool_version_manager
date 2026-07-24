# W00 開発準備・仕様固定（W00-02〜W00-07 決定記録）

本書はW00フェーズの計画・決定タスクの正本証跡である。規範値は各仕様章を正とし、本書は実装作業のための運用決定を固定する。実行環境はLinuxコンテナのため、Windows固有の**実行検証**は行わず、Windows非依存のunit/単体testを実施しながら仕様の実装順序で進める。Windows固有testは全実装・test完了後に利用者が実施する（利用者指示・2026-07-24）。

---

## W00-02 Windows対象matrix（固定）

正本: 01章4節、12章2節、09章2節。本タスクはWindows評価対象を固定する。

| 項目 | 固定値 |
|---|---|
| OS | Windows 10 / Windows 11 |
| 必須arch | amd64（Windows 10/11） |
| 追加arch | arm64（Windows 11、必須。native runner または承認済み評価方法でbuild/test。未評価範囲を明記） |
| filesystem | ローカルwritable NTFS を推奨。junction/hardlinkはローカルvolume限定。FAT/exFAT/policy制限時はshim-onlyへfallback |
| 権限 | 標準ユーザー（非管理者token）。UAC/HKLM/system環境変数変更を行わない |
| shell | `cmd`（HKCU AutoRun）、Windows PowerShell 5.1、PowerShell 7+（各profile）。execution policyは変更しない |
| エディタ連携 | `code.exe .` 起動時のVS Code本体・統合terminal・拡張processへの環境継承 |
| link能力判定 | group membershipでなく現在tokenと実操作成功でprobe（09章2.1節の非破壊probe手順） |

**本環境での扱い**: 上記matrixに対するWindows実機での実行検証（W11/W12）は本Linux環境で不可。CIのWindows job（W00-06）を作成し、実行は利用者に委ねる。platform-neutralなlink/shell抽象・fake・fixtureはWindows段階でも実装する（16章ゲート但し書き）。

---

## W00-03 Go toolchain・module・command（固定）

正本: 13章1節・1.2節・2節。

| 項目 | 固定値 |
|---|---|
| module path | `github.com/kznagamori/go_dev_tool_version_manager`（`go.mod`） |
| Go版 | `go 1.26.5`（`go.mod` の `go` directive）。bootstrap go1.24.7 でも `GOTOOLCHAIN=auto` で go1.26.5 を自動取得することを実証確認済み |
| CGO | client buildは全target `CGO_ENABLED=0` |
| build target | windows/amd64, windows/arm64, linux/amd64, linux/arm64（13章2節） |
| format | `gofmt -l -w`（差分ゼロをCIで検査） |
| vet/static | `go vet ./...`。CIでは追加で golangci-lint v2（`.golangci.yml`）を実行 |
| lint注意 | golangci-lintは対象Go版(1.26.5)**以上**でbuildした版が必要。CIは `go install .../golangci-lint/v2/cmd/golangci-lint` を go1.26 toolchainで行いbuild版を1.26に揃える。ローカルの既存v2.5.0(go1.25 build)は非互換のため使わず、`gofmt`＋`go vet` を最低線とする |
| unit test | `go test ./...` |
| race test | `go test -race ./...`（対応host） |
| integration/E2E | build tag `integration` / `e2e` で分離（`go test -tags=integration ./...`）。Windows実機E2Eは利用者実施 |
| coverage | `go test -coverprofile=.artifacts/coverage/coverage.out -covermode=atomic ./...` → `go tool cover -func`/`-html` |
| vulnerability scan | `govulncheck ./...`（CI） |
| license scan | 依存追加時に SPDX と再配布条件を記録（02章9節、CI補助） |

開発の標準commandは `Makefile`（Linux/CI）へ収録し、Windowsでも同等の生 `go` command で再現できる。lint/vulnerability toolは未導入環境ではCIで実行する前提とし、ローカルは `gofmt`＋`go vet` を最低線とする。

---

## W00-04 test artifact・log・coverage等のdirectoryと命名（固定）

すべてrepositoryへ**commitしない**build/CI出力とし、`.gitignore` で除外する。secretを保存しない。

| 種類 | path | 命名 |
|---|---|---|
| ルート | `.artifacts/`（gitignore） | — |
| coverage | `.artifacts/coverage/` | `coverage.out`, `coverage.html`, `coverage-<pkg>.out` |
| test log | `.artifacts/test-logs/` | `<suite>-<YYYYMMDD-HHMMSS>.log`（UTC） |
| benchmark | `.artifacts/bench/` | `<target>-<YYYYMMDD-HHMMSS>.txt` |
| SBOM | `.artifacts/sbom/` | `gdtvm_<version>_<os>_<arch>.sbom.spdx.json`（13章2節） |
| provenance | `.artifacts/provenance/` | `gdtvm_<version>_<os>_<arch>.provenance.intoto.jsonl` |
| attestation | `.artifacts/attestation/` | `<subject>.attestation.jsonl`（監査用、13章4節） |
| release archive | `.artifacts/dist/` | `gdtvm_<version>_<os>_<arch>.{zip,tar.gz}`, `checksums.txt`（13章2/4節） |

task証跡（監査・決定記録）は commit対象として `docs/reports/` に置く。生成途中file・debug出力・一時scriptはrepositoryへ残さない（CLAUDE.md §9）。secret/token/個人home path/内部URLをlog・fixture・証跡へ保存しない。

---

## W00-05 Git・commit・review・schema/registry変更・release tag（固定）

| 項目 | 固定運用 |
|---|---|
| 開発branch | `claude/go-dev-tool-version-manager-jhz6rj`（指定。ここへ開発・push） |
| commit粒度 | タスク単位。message先頭に `Wxx-yy:` 等のタスクID、本文に変更概要と証跡。日本語可 |
| commit trailer | 指定の `Co-Authored-By` / `Claude-Session` を付与 |
| PR | 利用者が明示要求した場合のみ作成（既定は作成しない） |
| review | schema/observable behavior変更は仕様・fixture・test・`docs/16` を同一変更で更新（CLAUDE.md §11、README §112） |
| schema変更 | tool定義schema版または状態schema版、互換性・migration・unknown key・negative testを同時に扱う（06章16節、14章17節） |
| registry変更 | 専用branch/tag/単体updateを作らず、定義変更は新client releaseとして扱う（07章1節） |
| release tag | `vYYYY.mm.DD.XX`（JST日付、`00`開始）。正本は `/VERSION`。tag=archive名=Release version を一致（13章1.2/14節） |
| モデル識別子 | commit/PR/コード/artifactへモデルIDを記載しない（chat応答のみ） |

---

## W00-07 test基盤方針（固定）

正本: 02章5節（全port差替え可能）、13章5〜7節。決定的testのため次を固定する。実体portは `internal/*` 実装時（W01-03以降）に整備し、本方針をその設計制約とする。

| 抽象 | fake方針 |
|---|---|
| Clock | 単調・現在時刻を注入する fake clock。時刻依存（TTL、interval,タイムスタンプ）を決定化 |
| HTTPClient | in-memory transport。GitHub pagination/ETag/rate limit/redirect chain/TLS test CA/404,429,5xx/Retry-After/Content-Length不一致/Range resume/接続切断を模倣（13章7節） |
| ProcessRunner | fake process。argv・env・cwd・exit code・stdout/stderr・timeout・cancel応答・process tree終了を検証 |
| FileSystem | temporary directory実装または in-memory。atomic write/rename/permission/realpath/walk を注入 |
| LinkManager/Lock/Archive | capability probe・lock所有・archive安全検査（traversal/bomb/case collision）を fake で決定化 |
| failure injection | download中断・step失敗・commit失敗・電源断相当（journal未terminal）・並行競合・cancel を注入するhook |

production では実HTTPを拒否しても、test port へ注入したHTTPClientは in-memory で試験できるようにする（13章7節）。全fakeはpackage global stateを持たず、Service生成時にDIする（02章、10章2節）。

---

## W00-06・W00-08 の扱い

- **W00-06（Windows CI runner）**: `.github/workflows/ci.yml` に windows-latest（標準ユーザー相当、管理者権限不要）のjobを作成する。本環境では実行できないため、実行検証は利用者が行う。Linux jobは本環境の対象。
- **W00-08（G-WIN-START判定）**: 上記W00-02〜W00-07とgo.mod/scaffolding完了をもって判定し、`docs/16` のゲートと本書へ記録する。
