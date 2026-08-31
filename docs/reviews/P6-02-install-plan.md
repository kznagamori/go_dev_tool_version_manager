# P6-02 決定記録（1/2）: §12 path root render

対象タスク: `docs/13-progress.md` P6-02の1本目。規範仕様は[06-tool-definition.md](../06-tool-definition.md)§10.1・§11・§12、[04-storage-and-data.md](../04-storage-and-data.md)§6・§16・§21、[02-architecture.md](../02-architecture.md)§4、[10-security.md](../10-security.md)§5。

## 1. 着手時の確認事項（P6-01の停止記録より）

2件とも解決した。

### 1.1 `app.Scope`はPlanから組み立てる向きで良い

P5-04の`app.Scope`は`writes[]`／`probes[]`／download URLを受けてallowlistを作る型として設計してある。Planがその3つを持つため、向きはPlan→Scopeで正しい。組立てはPlanを見る側の責務であり`internal/app`へ置く。2本目で`internal/app`のimport表へ`internal/store`を足す。

### 1.2 Plan `inputs`は供給されるもので、Plan builderが集めるものではない

[04-storage-and-data.md](../04-storage-and-data.md)§16はrevision/digestを「gdtvm自身が計算」と定める。計算の主体はそれぞれのfileを所有するpackageである。

| `inputs`の項目 | 供給元 |
|---|---|
| `RootID` / `ConfigSHA256` / `ProjectSHA256` | `internal/config` |
| `RegistrySHA256` / `DefinitionSHA256` | `internal/registry` |
| `CatalogSHA256` | `internal/catalog` |
| `Selections` / `Setup` / `ReceiptIndex` の各revision | `internal/store` |

Plan builderは受け取った値を組み立てるだけの純関数にする。Executeが同じ供給元から読み直して照合することで`E_PLAN_STALE`が成立する。builderが自分で集めると、作成時と照合時で読む経路が変わり、stale判定が「同じ関数を2回呼んだ結果の比較」に退化する。

## 2. 分割の見直し

**着手時の調査でP6-02の分割線を引き直した。** Planは§16でprobe args・storage path・command targetを`PathValue`として持つ。それを作るには§12のpath root評価が要るが、実装されていたのはURL/file templateのrender（`internal/catalog`、artifact用）と**検証側**（`internal/definition`の`templateScope`、文脈ごとにどのrootを許すか）だけで、**path rootを実pathへ評価する経路が無かった**。

そのため1本目をrender、2本目をPlan組立てと検証にした。台帳の項目もこの順で書き直した。

## 3. 判断

### 3.1 path templateは「root単体」か「rootの子path」だけに限る

[06-tool-definition.md](../06-tool-definition.md)§10.1・§11はliteral prefix/suffixの連結を禁じる。したがって受け付ける形は`{{root}}`と`{{root}}/a/b`の2つだけである。

| 形 | 扱い | 理由 |
|---|---|---|
| `{{payload}}` | 通す | root単体 |
| `{{payload}}/bin/go` | 通す | rootの子path |
| `/opt{{payload}}` | 拒否 | literal prefix |
| `{{payload}}bin`・`{{payload}}.bak` | 拒否 | literal suffix。連結を許すと兄弟directoryを指せる |
| `{{payload}}/{{version}}` | 拒否 | 子pathへ変数を許すと、rootの外を指す値を後から差し込める |
| `{{payload}}//bin`・`{{payload}}/bin/` | 拒否 | 空component |

`{{payload}}.bak`を許すと、render後の値は`.../payload.bak`になり**payload rootの外**を指す。containment検査はrootの子であることを見るので、これを弾けるのは形の制限だけである。

### 3.2 子pathはPOSIX slashで書き、変換は`security.Join`に任せる

§12「logical pathはPOSIX slashで記述し、OS adapterがseparatorへ変換する」。`/`で分けたcomponent列を[security.Join]へ渡す。componentのまま渡すのは、文字列連結だと`..`を先に潰された値を受け取ってしまい検出できなくなるためである（[security.Join]のdoc commentが同じ理由を述べている）。

これにより`..`、区切り混在（`bin\go`）、NUL、予約名、component長超過はすべて既存の検査が拒否する。renderが独自に持つ規則は増やさない。

### 3.3 render後のcanonical containment再検査はここで行わない

§12はrender後の再検査を求めるが、**Plan作成時点ではpayloadがまだ存在せずrealpathへ解決できない**。

| 検査 | 実施箇所 | 時点 |
|---|---|---|
| 語彙的なcontainment（`..`・区切り混在・空component） | `RenderPath`→[security.Join] | Plan作成時 |
| canonical containment（symlink/reparse経由の逸脱） | P5-04の記録wrapper（Guard） | 実書込み時 |

構成上、`RenderPath`の結果は必ず宣言rootの子pathになる。symlink経由の逸脱は書込み時にしか判定できず、[11-quality-and-ci.md](../11-quality-and-ci.md)§7.2のwrapperがその時点で見る。この分担を`RenderPath`のdoc commentへ書いた。

### 3.4 未設定rootは空文字列で代替せず失敗させる

`{{probe_temp}}`を空で置き換えると`{{probe_temp}}/x`が`/x`になり、filesystem rootからの絶対pathになる。`{{probe_temp}}`が未設定であること自体を「probe以外の文脈では使えない」（§12）の表現に使い、zero値なら失敗させる。

### 3.5 §12のtemplate grammarを`internal/definition`へ集約した

`internal/catalog`と`internal/definition`が同じ正規表現（`\{\{[^{}]*\}\}`と`{{storage.<id>}}`のID抽出）を各自で持っていた。renderをもう1つ書けば**3つ目の複製**になる。

`internal/definition`が§12のgrammarを所有する側であり、`internal/catalog`・`internal/install`の双方がすでに同packageをimportしている。`ReplaceTemplateVars`と`StorageTemplateID`として公開し、両方の呼出し側をそれへ寄せた。

| | before | after |
|---|---|---|
| grammarの実体 | `definition`・`catalog`・（新規に`install`） | `definition`だけ |
| global変数表 | `catalog.templateVarRe` | 削除（`install`側の追加も不要） |

P6-01で`isOffline`をport境界へ集約したのと同じ理由である。**検証側（`templateScope`）とrender側が同じ切り出し方を共有することで、「検証は通るがrenderが拾わない」形の食い違いも防ぐ。**

## 4. 検査が固定したこと

| 検査 | 対象 |
|---|---|
| `TestRenderPathResolvesRootsAndChildren` | root単体と子path、role継承、Windowsでのseparator変換 |
| `TestRenderPathRejectsLiteralConcatenation` | §3.1の拒否10件 |
| `TestRenderPathRejectsEscape` | `..`、区切り混在、NUL |
| `TestRenderPathRejectsUnknownRoot` | 未知root、未宣言storage、ID grammar違反、path文脈でのscalar変数 |
| `TestRenderPathRejectsUnsetRoot` | 未設定rootを空文字列で代替しないこと（root単体と子pathの両方） |
| `TestRenderPathEnforcesRenderedLimit` | render結果32 KiB |
| `TestRenderTextSubstitutesScalars` / `RejectsUnknownVariable` / `RejectsUnsetScalar` | path以外の文脈 |
| `TestReplaceTemplateVarsCoversEveryOccurrence`（definition） | 未知変数を素通りさせない切り出し |
| `TestStorageTemplateIDFollowsIdentifierGrammar`（definition） | §3のkebab-case identifier |

### 4.1 変異testが検査の穴を1件見つけた

3件の変異を入れて検査が落ちることを確かめた。**2件目が生き残った。**

`resolvePathRoot`の未設定検査を外しても`TestRenderPathRejectsUnsetRoot`が通った。子path付きのcase（`{{payload}}/bin`）しか置いておらず、[security.Join]のzero root検査が先に落としていたためである。**root単体（`rest == ""`）は[security.Join]を通らない**ので、そこだけが`RenderPath`自身の検査を固定する。

root単体のcaseを足したところ、**実装側の欠陥も1件出た**。storage rootは宣言の有無しか見ておらず、宣言はあるが値がzeroの場合にzero値の`PathValue`をそのまま返していた。`IsZero`検査を足した。

| 変異 | 初回 | 検査追加後 |
|---|---|---|
| literal suffixの連結を許す | 落ちた | — |
| 未設定root検査を外す | **通った** | 落ちる |
| 未知変数を素通りさせる | 落ちた | — |

## 5. 検証

Linux containerで実行した（Go 1.26.6）。両OSの判定はCI matrixで行う。

| command | 結果 |
|---|---|
| `gofmt -l .` / `go build ./...` / `GOOS=windows go build ./...` | 出力なし・成功 |
| `go vet ./...` / `GOOS=windows go vet ./...` | 成功 |
| `go test ./... -race -shuffle=on -covermode=atomic` | 全package成功。`internal/install` 92.5%・`internal/definition` 92.5% |
| `check_policy.py` / `check_imports.py` / `check_docs.py` / `check_licenses.py` / `check_messages.py` | すべて成功 |
| `git diff --check` | 出力なし |

## 6. 未実施・制約

- **`RenderPath`／`RenderText`をまだ誰も使っていない。** 呼出し側は2本目のPlan組立てである。
- **`internal/store`のtemplate grammarが`internal/definition`と食い違う（新規）。** 同じ§12・§3の規則を`internal/store/template.go`が別に持っており、値が一致しない。

  | 入力 | `internal/definition` | `internal/store` |
  |---|---|---|
  | `{{storage.9x}}` | 拒否（§3は先頭英字） | 通す（`[a-z0-9]+`） |
  | `{{a{{b}}`の切り出し | `{{b}}` 1件 | `{{a{{b}}` 1件 |

  どちらも未知tokenを最終的にはerrorにするためfail closedは保たれており、先頭数字のstorage IDは正当なdefinitionからは生じない。本PRでは`internal/store`へ手を入れていない——同packageは受入れ側（receipt）の検証で、`internal/definition`へ依存させるかは[02-architecture.md](../02-architecture.md)§2の責務表を要する判断であり、renderの範囲を超える。2本目かP6-03で扱う。
- **P6-01から継続する未実装**: Registry portと`LinkManager` portの記録wrapper（P7）、[11-quality-and-ci.md](../11-quality-and-ci.md)§7.2のE2E照合（P6以降）、`port.FileSystem`／`port.Environment`のproduction実装（P8-01）、Windowsの起動とjob割当ての隙間。
- **P5-03から継続する未決が1件**（`./`始まりのtar entryを[10-security.md](../10-security.md)§5に従って拒否しており、標準4 toolの実archiveを繋ぐP6-03で該当があれば仕様側で扱いを決める）。**P6-01で埋めた仕様の空白が1件継続**（exact指定で`installable=false`のときの`E_PLATFORM_UNSUPPORTED`）。**仕様側の未決が2件継続**（[07-registry-and-tools.md](../07-registry-and-tools.md)§5第2項のlicense file size上限が§21の表に無い、§2の「license file名はASCII kebab grammar」が§2自身の例と食い違う）。`python.toml`の`lifecycle = "unknown"`はP3-04から継続。source error専用のerror code（`E_DEFINITION_INVALID`）、version数値要素の64 bit上限、`logs/` file名規約と§11「専用lock」の解釈は未決である。
