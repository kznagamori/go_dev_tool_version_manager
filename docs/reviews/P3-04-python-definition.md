# P3-04 決定記録（3/3）: `python.toml`（P3-04完了）

対象タスク: `docs/13-progress.md` P3-04の残件。規範仕様は[06-tool-definition.md](../06-tool-definition.md)§6.6・§7.2・§16.3、[07-registry-and-tools.md](../07-registry-and-tools.md)§9。

## 1. blockerの解除

1本目・2本目は`registry/tools/python.toml`を作成できないblockerを記録していた。§6.6が`static_versions`のassetへ実digestを要求する一方、`api.github.com`が403で到達できず、release assetを列挙できなかったためである。

**着手時の再確認で、blockerの範囲が思っていたより狭いことが分かった。**

| host | 結果 | 用途 |
|---|---:|---|
| `api.github.com` | 403 | release/asset列挙、ID取得 |
| `github.com`（bare host） | 400 | — |
| `github.com/.../releases/download/...` | **200** | **release資産の取得** |

release downloadは到達できる。python-build-standaloneは各releaseへ**`SHA256SUMS`を公開している**ため、`https://github.com/astral-sh/python-build-standalone/releases/download/20250814/SHA256SUMS`（646行）から**providerが公開したSHA-256**を取得できた。§7.2の「upstream checksumが公開されているものだけを採用し、providerが公開したalgorithmでの照合を必須にする」を満たす。

digestを自分でarchiveから計算していない。それでは受け取ったbytesを自分で承認するだけになり、upstream checksumの意味が無い。

## 2. 固定したversionとasset

[07-registry-and-tools.md](../07-registry-and-tools.md)§13手順7が「Pythonはregistryへ固定した**新旧各1件**」と定める。provider release `20250814`に含まれる安定版から2件を選んだ。

| version | platform | size | digest（SHA256SUMSより） |
|---|---|---:|---|
| 3.13.6 | windows-amd64 | 21069469 | `09e489d2…6fa92` |
| 3.13.6 | linux-amd64-glibc | 32327269 | `d84745854…5f359` |
| 3.12.11 | windows-amd64 | 21187218 | `5ff0f9ae…0d27` |
| 3.12.11 | linux-amd64-glibc | 31382541 | `8592d071…b48f28` |

sizeは各assetへのHEADで得た`Content-Length`である。redirect先が`release-assets.githubusercontent.com`であることも実際のredirectで確認し、§16.3の`redirect_hosts`宣言と一致した。

**§16.3の例が使う`3.13.7`は`20250814` releaseに存在しない**（同releaseは3.9.23／3.10.18／3.11.13／3.12.11／3.13.6／3.14.0rc2を含む）。仕様例のdigestが`<64 lowercase hex>`のplaceholderであることと合わせ、§16.3は形式を示す例であり実在の組合せではないと判断した。実定義は実在するversionとassetで書いた。

## 3. 判断

### 3.1 lifecycleは`unknown`とした

§6.6は`lifecycle_evidence`を「provider/official projectのHTTPS一次資料」、`lifecycle_assessed_at`を評価時刻と定める。**`https://devguide.python.org/versions/`はこのcontainerから到達できない**（HTTP 000）。

3.13と3.12のsupport状態を記憶から`supported`と書くことは、**実施していない評価を記録すること**になる。§6.3が「gdtvm codeが公開日やversionの古さからEOLを推測しない」と定める趣旨と同じく、確認していない状態を断定しない。

§6.6は「**unknownでも「不明と判断した調査根拠」をevidenceへ残す**」と明示的に許すため、`lifecycle = "unknown"`、evidenceはdevguideのURL、`lifecycle_assessed_at`は本作業日とした。**release前にdevguideへ到達できる環境で確認し、`supported`へ更新する必要がある**（§9.2の「lifecycle根拠と評価日」、§13手順7のlive smokeが該当する）。§5に記載した。

### 3.2 `release_id`/`asset_id`は`"0"`

GitHubのrelease/asset IDは`api.github.com`（403）でしか得られない。§16.3の正規例が両方とも`"0"`を書いているため、それに倣った。§6.5の`provider_release`は`release_tag`（非空）を使うためIDは参照されず、catalogやPlanの表示にも出ない。

### 3.3 `published_at`はrelease tagの日付

provider release tag`20250814`が公開日を符号化している。§16.3の例も`2025-08-14T00:00:00Z`を書く。取得時刻で代用していない（§6.1）。

### 3.4 probeは§9.3の5項目

`python --version`の完全一致、`pip --version`成功、`ssl`/`sqlite3`/`venv`のimport、`{{probe_temp}}`へのvenv作成とrequired path確認（Windowsは`file:{{probe_temp}}/venv/Scripts/python.exe`、Linuxは`file:{{probe_temp}}/venv/bin/python`）、`sys.prefix`のpayload内containを`expect="path-within"`で確認する。engineがPython IDからscript名を推測しないよう、required pathをplatformごとに書き分けた。

## 4. 検査が固定したこと

- `python.toml`がschema 1を通り、file basenameとtool IDが一致し、2 platformを持つ（既存のcontract testが4 tool目として拾う）。
- version scheme `python`／source kind `static`／artifact source `asset`／checksum `asset-field`／`strip_components=1`、required command 4件（`python`/`python3`/`pip`/`pip3`）が両platformで一致する。
- **third-party契約**: `artifact_kind=third-party`、`repository`と`adoption_reason`が非空、`license_notice`を宣言しない（MPL-2.0はOSI承認、§9.1）。
- **固定catalog契約**: 各versionのasset 1件、`digest_algorithm=sha256`、digestのhex長64、sizeが正整数、`release_tag`が非空、lifecycle根拠と評価日が非空。
- **§6.6の両platform version集合一致**（片方だけの更新漏れを拒否する）。

## 5. 検証

### 5.1 ローカル検証

Linux container（Go 1.26.5、Python 3.11.15）で実行した。

| command | 結果 |
|---|---|
| `gofmt -l .` | 0 file |
| `go vet ./...` | 成功 |
| `go test ./... -race -shuffle=on -covermode=atomic` | 成功。`internal/definition` 92.4% |
| `scripts/ci/check_policy.py` | 成功 |
| `scripts/ci/check_imports.py` | 成功 |
| `scripts/ci/check_docs.py` | 成功 |
| `scripts/ci/check_licenses.py` | 成功 |
| `scripts/ci/check_pr_refs.py` | 成功（task-id=p3-04, slug=python-definition） |
| `git diff --check` | 差分なし |

### 5.2 CI

PR #82で、6 job×2 OSの **12 checkすべてがsuccess** になった（run 31959065799）。

## 6. 未実施・制約

- **lifecycleは`unknown`のままである。** `devguide.python.org`へ到達できず評価を実施できなかった。**release前に到達できる環境で確認し`supported`へ更新する**必要がある（§9.2・§13手順7）。推測で`supported`と書いていない。
- `release_id`/`asset_id`は`"0"`である（§3.2）。実IDが必要になった場合は`api.github.com`へ到達できる環境で更新する。
- digestは**取得したSHA256SUMSの値をそのまま**記録した。archiveを実際にdownloadして展開・probeするのは**P7**とlive smoke（§13手順7）の範囲である。asset名・layout・probe regexが実配布物と一致するかはそこで確認する。
- `registry/registry.toml`（§2のexact tree）とmanifest、file digest検証、command別load範囲は**P4-01**の範囲である。
- `go tool govulncheck ./...`はこのcontainerから`vuln.go.dev`が`Forbidden`で到達できず、ローカルでは未実行である（P0-02から継続）。本PRは新規依存を追加していない。
- Windowsでの実行はCI matrixでのみ確認する。
