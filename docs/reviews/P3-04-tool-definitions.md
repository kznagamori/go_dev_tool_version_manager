# P3-04 決定記録（1/2）: `source=asset`のurl/file templateと標準3 toolのdefinition

対象タスク: `docs/13-progress.md` P3-04（2分割の1本目）。規範仕様は[06-tool-definition.md](../06-tool-definition.md)§7.1・§8〜§11・§15・§16、[07-registry-and-tools.md](../07-registry-and-tools.md)§7〜§10。

## 1. 着手時に見つけた仕様の穴（利用者判断で解決）

3本目（P3-03）の停止記録が残した確認事項である。§16.2のGoは`source = "asset"`・`url = ""`・`file = ""`だが、Go公式JSONの`files[]`は`filename`/`os`/`arch`/`version`/`sha256`/`size`/`kind`を持ち、**download URLを持たない**（実際の取得先は`https://dl.google.com/go/<filename>`で、`https://go.dev/dl/<filename>`がそこへredirectする）。

P3-01は§16.2・§16.3の例に従い`source=asset`で`url`/`file`を空に強制し、`source=template`＋`checksum.kind=asset-field`も拒否していた。この組合せでは**選択assetからもtemplateからもdownload URLを決められない**。

利用者判断で§7.1へ次を明記した。

> `source=asset`の`url`/`file`は、**空なら選択assetの`url`/`name`を使い、非空なら選択assetを`{{asset.<field>}}`で参照できるtemplateとしてrenderする**。upstreamがasset listにdownload URLを載せず、file名からURLを組み立てる配布元（Go）に使う。selectorはどちらの場合も必須で、artifactの同一性はselectorが決める。

§16.2のGo例を`url = "https://go.dev/dl/{{asset.name}}"`・`file = "{{asset.name}}"`へ直した。`redirect_hosts = ["dl.google.com"]`の宣言はartifact URLのhostが`go.dev`でそこからredirectすることを示しており、この読みと整合する。

## 2. 判断

### 2.1 `url`と`file`は組で宣言させる

片方だけのtemplateはURLとfile名の出所が食い違う。§7.1はどちらも「空なら選択asset」と定めるため、両方空か両方templateかに限る。`{{asset.<field>}}`は`asset_fields`の宣言と突き合わせるため、宣言していないfieldを参照するdefinitionはschema検証で落ちる。

### 2.2 Goの`GOTOOLCHAIN=local`

[07-registry-and-tools.md](../07-registry-and-tools.md)§7.2が「選択した完全versionから別toolchainを暗黙download/実行させない」と定める。`GOROOT`/`GOBIN`だけでなく`GOENV`/`GOPATH`/`GOMODCACHE`/`GOCACHE`もstorageへ向け、上流標準commandの保存先だけをredirectする（設定keyを独自解釈しない）。

### 2.3 Node.jsのnpm/npx entrypointはOSで位置が違う

§16.1が「npm/npx引数は`{{payload}}/lib/node_modules/npm/bin/npm-cli.js`」と定める。Windowsは`{{payload}}/node_modules/...`、Linuxは`{{payload}}/lib/node_modules/...`である。`.cmd`/shell wrapperをtargetにせず、managed `node`実体へ同梱JS entrypointをfixed argvとして渡す。

### 2.4 .NET SDKの`license_notice`はWindowsだけ

§16.4が「Linux platformは`license_notice`を**宣言しない**（Linux配布物はMIT）」と定める。`provider.license`もWindowsは`LicenseRef-dotnet-library`、Linuxは`MIT`である。`DOTNET_CLI_TELEMETRY_OPTOUT`と`DOTNET_NOLOGO`は`override_allowed`へ入れない（[01-requirements.md](../01-requirements.md)§9「telemetryを追加しない」）。

### 2.5 required commandは両platformで同じ集合

shim名がOSで変わると、利用者のscriptがplatform間で動かなくなる。contract testで両platformのcommand名列を突き合わせる。

## 3. `python.toml`を含めない理由

§6.6は`static_versions`のassetへ**実digest**（algorithmに応じた64/128 lowercase hex）を要求し、§7.2は「v0.1はchecksumを公開しないartifactを扱わない」と定める。仕様例自体も`digest = "<64 lowercase hex>"`のplaceholderである。

このcontainerから取得先へ到達できない。

| host | 用途 | HTTP status |
|---|---|---:|
| `nodejs.org` | Node.js index | 200 |
| `github.com` | python-build-standalone release | 400 |
| `api.github.com` | 同上 | 403 |
| `go.dev` | Go download JSON | 到達不可 |
| `builds.dotnet.microsoft.com` | .NET release index | 到達不可 |

再現command: `curl -sS -o /dev/null -w "%{http_code}" https://api.github.com/repos/astral-sh/python-build-standalone`

**digestを推測や仮値で埋めない。** 構文上妥当だが誤ったdigestは、install時のchecksum照合で必ず失敗する定義を「正規のregistry」として持ち込み、しかも一見正しく見える。digestを取得できる環境で別途作る。Go／Node.js／.NET SDKはdigestをTOMLへ書かない方式（`asset-field`と`text-file`）のため影響しない。

## 4. 検査が固定したこと

### 4.1 §7.1のurl/file template

- `source=asset`でurl/file templateが通ること（`asset_fields`に`name`を宣言する§16.2のGo sourceで確認）。
- `url`だけ非空、`file`だけ非空はどちらも拒否。
- catalog側: templateがrenderされて`https://go.dev/dl/go1.25.0.windows-amd64.zip`になること、digestはasset fieldから決まること、空のときは選択assetの`url`/`name`を使うこと。

### 4.2 registry contract

| 観点 | 固定した内容 |
|---|---|
| parse | `registry/tools/*.toml`が全件schema 1を通る |
| ID | file basenameとtool IDが一致し、platformは`windows-amd64`と`linux-amd64-glibc`の2件 |
| tool別契約 | version scheme／source kind／artifact source／checksum kind／`strip_components`を§7〜§10の表と突き合わせる |
| command | **両platformで同じrequired command集合**（go/gofmt、node/npm/npx、dotnet） |
| license_notice | .NETのWindowsだけ宣言、Linuxは宣言しない |
| provider | 3 toolがofficialで`adoption_reason`を持たない |

tool固有のGo分岐は追加していない（CLAUDE.md §7・§11）。

## 5. 検証

### 5.1 ローカル検証

Linux container（Go 1.26.5、Python 3.11.15）で実行した。

| command | 結果 |
|---|---|
| `gofmt -l .` | 0 file |
| `go vet ./...` | 成功 |
| `go test ./... -race -shuffle=on -covermode=atomic` | 成功。`internal/definition` 91.8%、`internal/catalog` 88.4% |
| `scripts/ci/check_policy.py` | 成功 |
| `scripts/ci/check_imports.py` | 成功 |
| `scripts/ci/check_docs.py` | 成功 |
| `scripts/ci/check_licenses.py` | 成功 |
| `scripts/ci/check_pr_refs.py` | 成功（task-id=p3-04, slug=tool-definitions） |
| `git diff --check` | 差分なし |

### 5.2 CI

PR #76で、6 job×2 OSの **12 checkすべてがsuccess** になった（run 31955206955）。

## 6. 未実施・制約

- typed storage／install parameter（`strip_components` 0と1）／runtime command/env／probe／`license_notice`の**全conditional違反negative fixture**は**2本目**の範囲である。本PRはpositive側（実定義がparseを通ることと仕様の表との一致）までとした。
- **`registry/tools/python.toml`は§3のとおり作成できない。** P3-04の項目は2本目のmerge後もpython.tomlが揃うまで完了にしない。
- `registry/registry.toml`（§2のexact tree）、registry manifest、file digest検証、command別load範囲は**P4-01**の範囲である。
- 実artifactによるinstall／probe実行は**P7**、live smokeは[11-quality-and-ci.md](../11-quality-and-ci.md)§13手順7の範囲である。定義に書いたprobe regexとcommand targetが実配布物と一致するかはlive smokeで確認する。
- `go tool govulncheck ./...`はこのcontainerから`vuln.go.dev`が`Forbidden`で到達できず、ローカルでは未実行である（P0-02から継続）。本PRは新規依存を追加していない。
- Windowsでの実行はCI matrixでのみ確認する。
