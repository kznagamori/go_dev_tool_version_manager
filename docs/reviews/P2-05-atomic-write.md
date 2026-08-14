# P2-05 決定記録（2/2）: atomic writeとlog rotation

対象task: `docs/13-progress.md` P2-05（2分割の2本目、これでP2-05完了）
規範仕様: [04-storage-and-data.md](../04-storage-and-data.md) §2・§4・§6・§21、[10-security.md](../10-security.md) §12、[05-configuration.md](../05-configuration.md) §3.6、[02-architecture.md](../02-architecture.md) §2・§11・§12

## 1. 1本目から持ち越した確認事項

1本目の停止記録で、着手時に確認するとした2点を先に解いた。

### 1.1 revision fieldを持たないfileは無い

§4は「正本stateは`state/schema.toml`, `state/selections.toml`, `state/setup.toml`の3件」とし、「revision fieldを持つfileはnext=current+1（新規は1）を計算する」と定める。§8・§9・§11を確認したところ、**3件すべてが`revision`を持つ**。`NextRevision`を一様に適用でき、fieldを持たないfileの扱いを決める必要はなかった。

1本目の停止記録は「revision fieldを持たない`schema.toml`」と書いていたが、これは誤りである。§8の`schema.toml`は`revision`を持つ。

### 1.2 `.bak`の復元候補判定は`internal/store`が持つ

§4の復元候補条件は「strict parse/digest/root IDが一致する」である。strict parseは本packageのcodecそのもの、root IDは内容の一部であり、どちらも外部を必要としない。digestだけが内部SHA-256を要するが、[02-architecture.md](../02-architecture.md) §2がこれを`internal/security`の責務としているため、`internal/security`へ`SHA256Hex`を足して`internal/store`から使う構成にした。`check_imports.py`のALLOWED表へ根拠付きで追記している。

## 2. §4 atomic writeの判断

### 2.1 段階の分担

7段階のうち、本関数が持つのはstep 4〜7である。

| step | 担当 | 理由 |
|---|---|---|
| 1 対象lock取得 | 呼出し側 | ここでlockを取ると、1操作の中で§12のlock順を守れない |
| 2 revision計算 | 呼出し側（`NextRevision`を提供） | encodeがfile形式ごとに違い、ここで組み立てるとfile形式の数だけ分岐が増える |
| 3 temp書込みとflush | `port.FileSystem.AtomicWrite`とadapter（P9） | 1本目の利用者判断。§4.1のport表がflush/syncを挙げていない |
| 4 strict再parse | 本関数 | `AtomicWrite`へ渡す**同じbytes**に対して行う |
| 5 `.bak`退避→atomic replace | 本関数 | `ReadFile`＋`AtomicWrite`で退避してから`AtomicWrite`でreplaceする |
| 6 directory metadata flush | adapter（P9） | 同上 |
| 7 公開file再読と照合 | 本関数 | disk上の検証はここで行う |

### 2.2 `.bak`は必ずreplaceの前に書く

§4の「Windowsでreplace APIの制約がある場合も、旧fileを失った状態で新fileを書き始めない」を、順序で満たす。replaceだけを失敗させるfailure injectionで、`.bak`が既に書かれていて旧内容が残ることを固定した。

### 2.3 revisionを別に照合しない

§4 step 7は「expected digest/revisionと一致させる」とする。本実装は公開fileを再読して**digestだけ**を比べる。revisionは`Data`の一部であり、byte単位のdigestが一致すればrevisionも必ず一致するためである。digest照合はrevision照合を含む、より強い検査になっている。

### 2.4 rollbackは復元候補が妥当な場合だけ行う

検証（再読・digest・再parse）に失敗したとき、`.bak`が§4の復元候補条件を満たす場合だけ書き戻す。**壊れたbackupで公開fileを上書きすると、破損を1世代分広げることになる。** 戻せなかった場合も同じerrorを返し、`StateWriteResult.RolledBack`で「書けなかったが旧内容は残っている」と「書けず旧内容も失った」を呼出し側が区別できるようにした。

### 2.5 親directoryを作らない

`WriteState`は`MkdirAll`を呼ばない。§2のroot layoutはsetupが作るため、ここでmkdirすると**layoutに無い場所へstateを書けてしまう**。この判断はtest helperが`MkdirAll`する形に現れている。

## 3. log rotationの判断

### 3.1 file名の規約を導入した

§2のlayoutは`logs/`だけを定め、file名を定めていない。[05-configuration.md](../05-configuration.md) §3.6の`max_files`が複数fileを前提にするため、現行1件と退避複数件という構成が要る。本PRで次の規約を導入した。

| 種別 | 名前 |
|---|---|
| 現行 | `gdtvm.log` |
| 退避 | `gdtvm-<UTC timestamp>-<invocation IDの先頭8桁>.log` |

**この命名は仕様に無く、本PRで導入した規約である。** 仕様へ昇格させるか変更するかは利用者判断とし、[04-storage-and-data.md](../04-storage-and-data.md) §2は変更していない。

### 3.2 番号のcascadeにしない

`gdtvm.1.log`→`gdtvm.2.log`…とずらすlogrotate方式を採らない。**1回のrotationがN回のrenameになり、途中で中断すると番号の重複と欠落が残る。** timestampを名前へ入れれば1回のatomic renameで完了する。

timestampの書式`20060102T150405Z`は固定幅・zero paddingであるため、**名前のbyte順がそのまま時刻順になる**。保持上限の削除順は名前のsortで決めるため、この一致が崩れると新しいlogから消える。時刻を進めながら名前の順序が単調増加することと、非UTC時刻を渡してもUTCへ揃うことをtestで固定した。

invocation IDを付けるのは、同一秒に別processがrotationしても退避先が衝突しないようにするためである。

### 3.3 §11の「専用lock」と§12のlock分類の齟齬

[02-architecture.md](../02-architecture.md) §11はlog rotationが「Plan/Approveを持たず、専用lockとatomic write/deleteを使う」とする。しかし**同§12のlock分類6件（state/catalog/install/storage/setup/shim）にlogは無い**。

§12はPlan transactionの取得順を定めるものであり、logはそこへ含まれないと読んだ。7つ目のlock classを追加していない。1本目で`LockClassCount = 6`と6分類の名前をtestで固定しており、使わないenum値を先行導入しない方針（CLAUDE.md §7）にも反するためである。代わりに**構造で並行安全にした**。

1. 退避先の名前がinvocationごとに一意で、別processと衝突しない
2. renameに負けたprocess（source不在）は「他がすでに退避した」として続行する
3. 保持上限の削除は同じ集合に対して何度実行しても同じ結果になる

8 workerの並行testで、退避に成功するのがちょうど1 workerであること、残るfileが退避名1件であることを固定した。

**この解釈は仕様書へ反映していない。** log用のlock classを§12へ足すか、§11の「専用lock」の意味を明確にするかは利用者判断である。

### 3.4 `max_files`は現行fileを含む総数

§3.6は`max_files`を1〜100とだけ定め、現行fileを含むかを書いていない。**含む総数**として扱った。`max_files=1`が「履歴を持たない」という自然な意味になるためである。退避した直後にその1件を消す挙動になる。

### 3.5 削除対象は退避名に完全一致するregular fileだけ

§6が「削除はreceiptまたはsetup stateで所有を証明できるregular file/directory/linkだけを対象にする」と定める。前方一致で判定すると利用者が置いた`gdtvm-old.log`のようなfileを消してしまうため、prefix・timestamp・8桁hex・suffixのすべてが一致する名前だけを対象にする。symlinkは名前が一致しても対象にしない。

`logs/`にsub directoryがあれば拒否する。削除対象をfile名だけで決めているため、§2のlayoutが定める「平坦である」という前提が崩れた時点で止める。

### 3.6 保持上限の適用はrotation直後だけ

1行ごとにdirectoryを走査すると、log出力の費用がfile数に比例して増える。退避fileが1件増えた直後にだけ適用する。configの`max_files`を小さくした場合、次のrotationまで反映されない。

### 3.7 rotationの失敗でoperationを失敗させない

呼出し側がerrorを診断へ回して続行するか諦めるかを決める契約とし、doc commentへ明記した。**logが書けないことでinstallを巻き戻すと、診断のための機構が本体の可用性を下げることになる。**

### 3.8 上限の正本を重複させない

`internal/store`は§3.6の範囲（`max_files` 1〜100、`max_bytes_per_file` 1 MiB〜1 GiB）を再検査しない。config値の検査は`internal/config`の責務であり、定数を両packageへ書くと片方だけが動いた状態が静かに成立する。本packageが検査するのは動作に必要な不変条件だけである。

- `max_files >= 1`
- `max_bytes_per_file >= LogLineMaxBytes`（§21の256 KiB）

後者が成り立たないと、新しいfileへ書いても即座に上限を超えてrotationが終わらない。

## 4. 検査が固定したこと

### 4.1 2つの仕様の上限が両立する

§21の「log 1 line 256 KiB」が§3.6の`max_bytes_per_file`下限1 MiBを下回ることをtestで固定した。逆転すると§3.7の不変条件が成り立たなくなる。片方の仕様だけが動いたときにここで気付ける。

### 4.2 digest不一致は専用のtest doubleで再現した

fakeの`ReadFile`は書いた内容をそのまま返すため、§4 step 7のdigest照合が不一致になる状況をfakeだけでは作れない。指定回目の読込みだけ内容を差し替える`tamperingFileSystem`を`_test.go`へ置き、他processが公開file直後に書き換えた場合を決定的に再現した。

### 4.3 並行`WriteState`は「壊れていないこと」だけを固定する

lockを取らずに並行`WriteState`を並べると、検証の再読が他workerの書込みとすれ違ってdigest不一致でrollbackする。これは**§4 step 1のlockを取らないことの帰結であり、想定内の失敗**である。testは公開fileが常に完全な1文書としてparseできることだけを固定した。更新の順序はstep 1のlockが決めるものであり、本関数の責務ではない。

### 4.4 主なnegative test

| 対象 | 内容 |
|---|---|
| §4 step 4 | strict parseできない内容を拒否し、公開fileを作らない |
| §4 step 5 | replaceだけ失敗させ、`.bak`が先に書かれていることを確認 |
| §4 step 7 | 検証の読込み失敗、digest不一致、再parse失敗の3経路すべてでrollback |
| §4 復元候補 | 3件。backup不在、strict parse不可、root ID不一致。いずれも公開fileを触らない |
| `WriteState` request | 6件。FileSystem/Parse/path/data未設定、Backup=trueでroot ID未設定、data上限超過 |
| 退避名のparse | 12件。現行file名、識別子の桁数違い、hexでない識別子、拡張子違い、13月、prefix違いなど |
| rotation request | 10件。FileSystem/directory/host/時刻/invocation未設定、`max_files`の0と負、追記行の負と上限超過、`max_bytes`が1行の上限未満 |
| 現行fileの位置 | directoryが占めている場合は`E_PATH_CONFLICT`、symlinkは`E_PATH_UNSAFE` |
| 失敗注入 | `Stat`/`Rename`/`Walk`/`Remove`の4経路。いずれも`E_FILESYSTEM`とrole `log` |
| 管理外file | `notes.txt`、`gdtvm-old.log`、退避名と一致するsymlinkを残す |
| path組み立て | logical path 32 KiB超過を`E_PATH_UNSAFE`で拒否（`security.Join`経由であることの確認） |

すべてのerrorが実pathをparametersへ載せないことも確認した（[10-security.md](../10-security.md) §9.2）。

## 5. 検証

### 5.1 ローカル検証

Linux container（Go 1.26.5、Python 3.11.15）で実行した。

| command | 結果 |
|---|---|
| `gofmt -l .` | 0 file |
| `go vet ./...` | 成功 |
| `go test ./... -race -shuffle=on -covermode=atomic` | 成功 |
| `scripts/ci/check_policy.py` | 成功。production Go file 82件 |
| `scripts/ci/check_imports.py` | 成功。package 20件 / internal import 87件 |
| `scripts/ci/check_docs.py` | 成功 |
| `scripts/ci/check_licenses.py` | 成功。module 14件 |
| `scripts/ci/check_pr_refs.py` | 成功（task-id=p2-05, slug=atomic-write） |
| `git diff --check` | 差分なし |

package別coverage: `internal/progress` 100.0%、`internal/security` 97.2%、`internal/domain` 95.8%、`internal/app` 94.9%、`internal/domain/port` 93.8%、`internal/config` 93.3%、`internal/store` 89.2%、`internal/domain/port/fake` 87.0%、`cmd/gdtvm` 66.7%。

### 5.2 CI

PR #52（workflow run 31790305980）で、6 job×2 OSの **12 checkすべてがsuccess** になった。

## 6. 未実施・制約

- `logs/`のfile名規約（§3.1）と§11「専用lock」の解釈（§3.3）は、**本PRで導入・決定したものであり仕様書へ反映していない**。仕様へ昇格させるか変更するかは利用者判断である。
- log行の実際の追記は`port.Logger`の実装（**P9**）の責務である。`port.FileSystem`はappendを持たず、[02-architecture.md](../02-architecture.md) §4.1のport表にもappendが無い。本PRは追記前のrotation判定までを持つ。
- `go tool govulncheck ./...` はこのcontainerから`vuln.go.dev`が`Forbidden`で到達できず、ローカルでは未実行である（P0-02から継続）。本PRは新規依存を追加していない。
- `WriteState`と`RotateLogs`を`Initialize`等のuse caseへ接続するのは**P8-01**の範囲である。
- 実OS lockのadapter実装、実symlink/reparse pointを作る統合testは**P9**の範囲である。本PRはfake `port.FileSystem`が`IsSymlink`を返すcaseで検証した。
- `ToolID`の長さ上限は仕様に規定が無く未設定のままである（P1-02から継続）。
- Windowsでの実行はCI matrixでのみ確認する。
