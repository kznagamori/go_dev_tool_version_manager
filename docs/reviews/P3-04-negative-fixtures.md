# P3-04 決定記録（2/2）: §5・§8〜§11のconditional違反fixture

対象タスク: `docs/13-progress.md` P3-04（2分割の2本目）。規範仕様は[06-tool-definition.md](../06-tool-definition.md)§5・§8〜§12、[04-storage-and-data.md](../04-storage-and-data.md)§21。

## 1. 範囲

1本目は§15〜§16の実定義がparseを通ることを固定した（positive）。本PRは**conditional違反**側で、P3-01(3/3)の既存testが覆っていない箇所を埋める。

既存の網羅状況を先に確認し、重複を作らなかった。`TestStorageRejects`（enum/identifier/path/scope-purge組 11件）、`TestStorageRejectsOverlappingPaths`、`TestCommandRejects`（target/args/profile 12件）、`TestCommandRequiresEveryKey`、`TestProfileRequiresEveryKey`、`TestEnvironmentRejects`、`TestWindowsEnvironmentNamesAreCaseInsensitive`、`TestProbeRejects`、`TestProbeExpectConditionalFields`、`TestRequiredPaths`、`TestProbeRequiresEveryKey`、`TestProbeRejectsDuplicateID`、`TestStorageLimit`はP3-01(3/3)が既に持っている。

## 2. 実装中に確認した仕様

### 2.1 `required_paths`はexpectに依存しない

当初、§11の表の「`success`はexpected fields禁止」を`required_paths`にも適用するtestを書いたが、**受理された**。§11本文は`required_paths`を「`{{payload}}`, `{{probe_temp}}`, `{{storage.<id>}}`配下のpath templateで、probe成功直後に指定種別の存在を要求する」と独立に定めており、表の「expected fields」は`expected_version`と`expected_root`を指す。実装が正しく、testを仕様へ合わせた。

entryは`file:<template>|directory:<template>`である（`dir:`ではない）。

### 2.2 probeのcwdは定義keyではない

§11は「probeごとに空のowner-only probe tempを作り、成功/失敗/cancel後にengineが削除する。**probeのcwdはその probe temp とし、呼出し元のcurrent directoryを継承しない。**」と定める。probe keyにcwdは無く、engineの固定動作である。definition側で固定できるのは`{{probe_temp}}`のscope（validation probe内だけ）であり、それをtestで押さえた。実際のcwd設定はP7の範囲である。

## 3. 検査が追加で固定したこと

| 対象 | 追加した内容 |
|---|---|
| §5 `license_notice` | message ID grammar違反5件（hyphen、大文字、空、URL、末尾dot）を拒否する。これまでpositive側だけだった |
| §9 install | `strip_components`のkey欠落、`[platforms.install]` table欠落、unknown keyを拒否する |
| §8 storage | 5 keyを1件ずつ落として全件必須を確かめる。key数を定数で固定し、keyの増減に気付けるようにする |
| §8 storage kind | 6値すべてが受理されることを確かめる。enumが欠けるとその用途のstorageを宣言できない |
| §11 probe | `expect=success`が`expected_version`/`expected_root`を拒否する |
| §11 probe | 未宣言commandを`runtime_command`に取れない（Plan外のprocessを実行しない） |
| §12 `{{probe_temp}}` | probeの`required_paths`では使え、environmentの`set`と`path_prepend`では使えない |
| §10.1 command | commandが0件の定義を拒否する（shimが作られずinstalledとして扱う意味がない） |
| §11 validation | probeが0件の定義を拒否する（展開しただけの状態をinstalledにしない） |
| §21 上限 | probe 64、runtime command 64を超える定義を拒否する |

## 4. 検証

### 4.1 ローカル検証

Linux container（Go 1.26.5、Python 3.11.15）で実行した。

| command | 結果 |
|---|---|
| `gofmt -l .` | 0 file |
| `go vet ./...` | 成功 |
| `go test ./... -race -shuffle=on -covermode=atomic` | 成功。`internal/definition` 92.3%（test 547件） |
| `scripts/ci/check_policy.py` | 成功 |
| `scripts/ci/check_imports.py` | 成功 |
| `scripts/ci/check_docs.py` | 成功 |
| `scripts/ci/check_licenses.py` | 成功 |
| `scripts/ci/check_pr_refs.py` | 成功（task-id=p3-04, slug=negative-fixtures） |
| `git diff --check` | 差分なし |

### 4.2 CI

PR #79で、6 job×2 OSの **12 checkすべてがsuccess** になった（run 31956079451）。

## 5. 未実施・制約

- **`registry/tools/python.toml`は依然として作成できない。** §6.6が`static_versions`のassetへ実digestを要求するが、このcontainerから`github.com`（400）・`api.github.com`（403）へ到達できない。本PR着手時にも再確認した（再現: `curl -sS -o /dev/null -w "%{http_code}" https://api.github.com/repos/astral-sh/python-build-standalone`）。**P3-04の項目はpython.tomlが揃うまで完了にしない。** 解除条件はdigestを取得できる環境での実行、または利用者からのdigest／`release_id`／`asset_id`／`published_at`の提供である。
- typed storage／install parameter／runtime command/env／probeの**実行時動作**（storage directoryの作成、`strip_components`の適用、環境block生成、probeのcwd固定とtemp削除）はP5・P7の範囲である。本PRはdefinitionのschema検証までとした。
- `registry/registry.toml`とmanifest、file digest検証はP4-01の範囲である。
- 定義に書いたprobe regexとcommand targetが実配布物と一致するかはlive smoke（[11-quality-and-ci.md](../11-quality-and-ci.md)§13手順7）でのみ確認できる。
- `go tool govulncheck ./...`はこのcontainerから`vuln.go.dev`が`Forbidden`で到達できず、ローカルでは未実行である（P0-02から継続）。本PRは新規依存を追加していない。
- Windowsでの実行はCI matrixでのみ確認する。
