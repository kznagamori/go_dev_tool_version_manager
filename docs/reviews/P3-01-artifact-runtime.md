# P3-01 決定記録（3/3）: §7〜§12とJSON schema

対象task: `docs/13-progress.md` P3-01（3分割の3本目、これでP3-01完了）
規範仕様: [06-tool-definition.md](../06-tool-definition.md) §7〜§13、[07-registry-and-tools.md](../07-registry-and-tools.md) §2・§5、[04-storage-and-data.md](../04-storage-and-data.md) §21、[10-security.md](../10-security.md) §13

## 1. 着手時に確認した事項

2本目の停止記録で「§12のtemplate root検査をP2-04の`internal/store/template.go`と共有できるか」を確認するとしていた。**共有しない**という結論になった。理由は2つある。

1. 同fileのhelperはすべて非公開（`validateTemplate`、`storageTemplateRe`ほか）であり、package外から使えない。
2. scopeが違う。§14 receiptのtemplateは`{{payload}}`と`{{storage.<id>}}`の2 rootだけだが、§12のdefinition templateは`{{version}}`、`{{platform.id}}`、`{{payload}}`、`{{probe_temp}}`、`{{storage.<id>}}`、`{{metadata.<key>}}`、`{{asset.<field>}}`の7 rootを、文脈ごとに違う部分集合で許す。

1本目でstrict decodeを共有しなかったのと同じ判断である。receiptとdefinitionは別のlayerであり、片方の都合で共通関数を変えるともう片方の受理範囲が黙って動く。

## 2. 判断

### 2.1 templateを2つのmodeへ分ける

§12のtemplateは文脈で意味が変わる。1つの検査関数にまとめず、modeで分けた。

| mode | 対象 | 規則 |
|---|---|---|
| substitution | §7.1のURL/file | 変数はliteralの任意の位置に現れてよい。`node-v{{version}}-win-x64.zip`が正規例 |
| path root | §10.1 target/args、§10.2 environment値・PATH、§11 probe args・required_paths、§11 `expected_version` | **entry全体がrootそのものか、rootの子path**でなければならない |

path rootで**literal prefix/suffixの連結を拒否する**のは§10.1と§11の明文である。連結を許すと`{{payload}}../etc`のようにrender後へrootの外を指す値を作れる。`{{payload}}bin`のようにrootの直後が`/`でない形も同じ理由で拒否する。

対応の取れない`{`と`}`も拒否する。`{{`だけを許し、conditionやfunctionの記法を素通りさせない（§12）。

### 2.2 文脈ごとの許可rootをscopeで持つ

bool 1つずつではなくscope structで持つ。§12の「`{{probe_temp}}`はvalidation probe内だけ」を型で表すためである。commandが`{{probe_temp}}`を使えると、probe終了後に削除される一時directoryを恒久的な参照先にしてしまう。commandからは落ち、probeでは通ることをtestで固定した。

**command targetは`{{payload}}`だけを許す**（`payloadOnlyScope`）。§10.1が「targetは`{{payload}}`配下のregular executable、または固定interpreterとして別required command targetと同じ実体を指せる」と定めており、どちらもpayload配下である。storageを許さないのは、storageが利用者の公式commandで書き換えられる領域であり（§8）、command targetにすると管理外の実体を起動しうるためである。

### 2.3 宣言済み集合と突き合わせる

`{{storage.<id>}}`、`{{metadata.<key>}}`、`{{asset.<field>}}`は同じplatformの宣言に依存する。宣言していないrootを参照するtemplateは、render時に値が無くinstallできない。`templateContext`へ集合を集めて突き合わせる。

storageを先に確定させてからartifact/runtime/validationを読む順序にしたのはこのためである（§13の検証順序7→6ではなく、依存で並べ替えた。§13は順序を「検証項目の並び」として定めており、実装の評価順を縛るものではない）。

**static sourceは`asset_fields`を持たない**ため、13 field全件を宣言済みとして扱う。§6.6がstatic assetへ13 field全件必須を課しており、どのfieldも必ず存在する。

### 2.4 `source`と`kind`の条件付きkey

§7.1と§7.2は組合せで必須・禁止が変わる。表ではなくswitchで書いたが、各分岐で「何を必須にし、何を禁じるか」を対にして並べている。

- `source=template`: URL/file必須、selector禁止
- `source=asset`: URL/fileは空、selector必須
- `kind=text-file`: URL・line_format必須、algorithm禁止（line formatがalgorithmを含むため）
- `kind=asset-field`: URL・line_format禁止。sourceが`digest_algorithm`を持たない場合だけalgorithm必須

**`source=template`＋`kind=asset-field`を拒否した。** assetのdigestを使う契約なのに、artifact自体をassetから選ばない定義になる。§7.2の文面には無いが、両者の意味から一意に決まる。

### 2.5 URL templateはhost部分に変数を許さない

§7.1の`redirect_hosts`は「artifact URLとchecksum URLに共通の追加許可host」であり、省略時は「それぞれの元host」だけを許す。hostへ変数を埋めると、元hostが上流の値で変わり、許可hostの比較が定義時に決まらない。

render前の値はURLとして解釈できないため、変数を固定文字列へ置換した骨格を作ってからscheme/host/userinfoを見る。

### 2.6 storage pathの包含も拒否する

§8の「同一platform内でrender後pathが重複/包含しないこと」を、重複だけでなく包含まで見る。包含を許すと、片方のpurgeがもう片方を巻き込んで消す。

scopeとpurgeの組は表で閉じた。tool scopeへ`with-version`を許すと、共有storageがversion削除で消えて他versionの設定を失う。

### 2.7 環境変数のcase規則をplatformで決める

§10.2の「Windows env keyはcase-insensitiveに一意」を、platform entryのOSで切り替える。`runtime.GOOS`ではなくdefinitionのplatform IDから決めるため、どちらのrunnerからでも両OSの規則をtestできる（CLAUDE.md §5）。Windowsでは`NPM_CONFIG_CACHE`と`npm_config_cache`の併存を拒否し、Linuxでは別変数として通ることをtestで固定した。

`set`と`unset`の衝突も拒否する。同じkeyをsetしつつunsetすると、どちらが有効かがdefinitionから決まらない。

### 2.8 probeは宣言済みcommandを通してだけ起動する

§11の`runtime_command`が§10.1で宣言したcommand名に一致することを要求する。Plan外のprocessを実行しない（[10-security.md](../10-security.md) §13）ための境界である。

`expected_root`はtemplate形式（`{{...}}`）ではなくroot名（`payload|probe-temp|storage.<id>`）を書く点がargsと違う。仕様の記法どおりに分けて実装し、storage IDの実在だけを突き合わせる。

### 2.9 JSON schemaは補助成果物として置く

[07-registry-and-tools.md](../07-registry-and-tools.md) §5が「schema JSONはTOML parser/semantic validatorの補助成果物であり、JSON Schemaだけで適合を宣言しない」と定める。`registry/schemas/tool-definition-v1.json`をこの位置づけで置き、**file自身の`description`へその旨を書いた**。読む人がJSON Schemaを唯一の正本と誤解しないためである。

JSON Schemaで表せない契約（kindごとの禁止key、template root、storage pathの包含、参照先IDの実在、scope×purgeの組、expect別の必須/禁止field）は`internal/definition`が持つ。descriptionにもその分担を書いている。

**同期はcontract testで担保する。** key集合、enum値、上限をGo側の定数と突き合わせ、片方だけにkeyを足した状態を検出する。全objectが`additionalProperties: false`であることも確かめる。1か所でも開いていると、§1の「unknown key/tableを拒否する」がschema JSON側で崩れる。

## 3. 検査が固定したこと

### 3.1 正規例をfixtureにする

§15のNode.js `windows-amd64`をartifact/storage/runtime/validationまで含めてfixtureにした。2本目までのfixtureは§6以降を最小の中身で置いていたため、本PRで正規例の全体へ差し替えている。

### 3.2 主なnegative test

| 対象 | 内容 |
|---|---|
| §7.1 artifact key | 12件。id非primary、source/format enum外、size負、HTTP URL、hostにtemplate、未知template、fileに区切り、未宣言のmetadata/asset、対応の取れない波括弧 |
| §7.1 source別契約 | 6件。templateでselector、assetでurl非空/selector欠落、条件無しselector、templateでasset-field。asset sourceが通ることも確認 |
| §7.2 checksum | 6件。kind enum外、line_format enum外、text-fileでurl/line_format欠落、text-fileでalgorithm、checksum table欠落 |
| §7.1 redirect_hosts | 6件。wildcard、大文字、scheme付き、port付き、空、重複 |
| §7.1 artifact必須key | 6件を1件ずつ落として拒否 |
| §8 storage | 10件。kind/scope/purge enum外、ID grammar、path absolute/相対参照/空/backslash/template、scope×purgeの不正組2件 |
| §8 path重複 | 3件。同一path、包含、ID重複 |
| §8 上限 | 33件目のstorageを拒否 |
| §9 install | 5件。0/1は通し、2/-1/10と型違いを拒否 |
| §10.1 command | 11件。name grammar、working_directory enum外、未宣言profile、targetがstorage/literal/相対参照/prefix・suffix連結、args未宣言storage、probe_temp、literalの波括弧 |
| §10.1/§10.2 必須key | command 7件、profile 6件＋`set` tableを1件ずつ落として拒否 |
| §10.2 environment | 7件。未宣言storage、`{{version}}`、`=`入りの名前、PATHがliteral/重複、shell_export重複、set/unset衝突 |
| §10.2 Windows case | 大小違いの同名をWindowsで拒否、Linuxで受理 |
| §11 probe | 9件。id grammar、stream/expect enum外、未宣言command、timeout下限/上限/0/解釈不能、args未宣言storage |
| §11 expect別 | 8件。versionのregex/expected_version欠落、capture無し/名前違い、versionでexpected_root、expected_versionが`{{version}}`でない、successでexpected_version、path-withinでregex欠落。path-withinの受理も確認 |
| §11 expected_root | 2件。未宣言storage、enum外 |
| §11 required_paths | 6件。prefix無し、未知prefix、templateでない、未宣言storage、相対参照、重複。file/directoryの受理も確認 |
| §11 probe必須key | 7件を1件ずつ落として拒否 |
| §12 `{{probe_temp}}` | probeで通り、commandで落ちる |
| JSON schema | key集合12種、enum 15種、上限11種をGo側と突き合わせ。全objectの`additionalProperties: false` |

### 3.3 global state検査が10件を検出

`hostnameRe`、`templateRe`、`storageTemplateRe`、`metadataTemplateRe`、`assetTemplateRe`と5つのscopeを根拠付きで許可表へ追加した（8回目）。

## 4. 検証

### 4.1 ローカル検証

Linux container（Go 1.26.5、Python 3.11.15）で実行した。

| command | 結果 |
|---|---|
| `gofmt -l .` | 0 file |
| `go vet ./...` | 成功 |
| `go test ./... -race -shuffle=on -covermode=atomic` | 成功。`internal/definition` 91.9%、test 519件 |
| `scripts/ci/check_policy.py` | 成功 |
| `scripts/ci/check_imports.py` | 成功 |
| `scripts/ci/check_docs.py` | 成功 |
| `scripts/ci/check_licenses.py` | 成功 |
| `scripts/ci/check_pr_refs.py` | 成功（task-id=p3-01, slug=artifact-runtime） |
| `git diff --check` | 差分なし |

### 4.2 CI

PR #60で、6 job×2 OSの **12 checkすべてがsuccess** になった。

## 5. 未実施・制約

- **template rootの評価（render）は行わない。** §12の「render後にcanonical containmentを再検査する」はrender時の検査であり、runtime/installの責務（P5・P7）である。本PRはdefinitionのschema検証までとする。
- §7.1のselectorが**実際にexactly 1件を選ぶか**はcatalogのasset listに依存するため、**P3-03**の取得経路が扱う。本PRはselectorに条件が1件以上あることまでを見る。
- §9の「除去後に空/衝突となる場合は拒否する」はarchive entryを見るため**P5-03**のextract engineの責務である。本PRは`strip_components`の値域だけを固定する。
- registry全体のID/alias/command衝突と、[07-registry-and-tools.md](../07-registry-and-tools.md) §5のcontract（required command集合、typed storage、`license_notice`とOSI承認licenseの対応、`lifecycle_map`のupstream enum網羅）は**P4-01**の範囲である。
- 標準4 toolのdefinition TOMLそのもの（`registry/tools/*.toml`）は**P3-04**で作る。本PRが置いたのは`registry/schemas/tool-definition-v1.json`だけである。
- `registry-v1.json`（registry manifestのschema）は**P4-01**の範囲である。
- 2本目で導入した`cache_ttl`の範囲、pointer 255 byte・regex 1024 byteの上限に加え、本PRでも仕様に無い上限を置いた。SPDX expression 128 byte（1本目）、hostname 253 byteである。いずれも標準4 toolの実値から余裕を持たせた値で、仕様へ昇格させるかは利用者判断である。
- `go tool govulncheck ./...` はこのcontainerから`vuln.go.dev`が`Forbidden`で到達できず、ローカルでは未実行である（P0-02から継続）。本PRは新規依存を追加していない。
- Windowsでの実行はCI matrixでのみ確認する。
