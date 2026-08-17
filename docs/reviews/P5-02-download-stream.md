# P5-02 決定記録（1/2）: streaming digest

対象タスク: `docs/13-progress.md` P5-02の1本目。規範仕様は[05-configuration.md](../05-configuration.md)§3.5、[04-storage-and-data.md](../04-storage-and-data.md)§7・§21、[10-security.md](../10-security.md)§7.2、[02-architecture.md](../02-architecture.md)§2・§4.1・§7。

## 1. 着手時の確認事項（P5-01の停止記録より）

2件とも確認した。

### 1.1 §21の上限は用途ごとに別行で、使い分けに曖昧さはない

| 用途 | 上限 |
|---|---|
| upstream metadata response各文書 | 16 MiB |
| checksum text | 2 MiB |
| catalog JSON各file | 64 MiB |
| **artifact download** | **20 GiB** |

hasher側で1つに決められないため、上限は呼出し側が渡す形にした。

### 1.2 既存digest APIは1 passの複数algorithm同時計算に対応していない

`internal/security`が持つのは`SHA256Hex(data []byte) string`だけで、全量をmemoryへ載せる。§3.5の「1 artifactはstreamで処理し全量memoryへ載せない」と20 GiB上限に対して使えない。

## 2. 利用者判断: portにせず`internal/security`へ直接実装した

確認の過程で**仕様の食い違いを1件見つけた**。

- §7は「`Ports`は最低限、Filesystem、Link、Registry、HTTP、Archive、**Hash**、Process、Environment、UserLookup、Clock、Lock、Random、Loggerを持つ」と定め、§4.1も`HashCalculator`を「streaming digest計算」として挙げる。
- 一方§6がportを要求する理由は「filesystem、link、registry、HTTP、process、archive、clock、lock、progress**等の外部作用**」であり、hashingは外部作用を持たない純計算である。§2は「upstream SHA-256/SHA-512、内部SHA-256」を`internal/security`の責務としている。

| 選択肢 | 結果 |
|---|---|
| **`internal/security`へ直接実装し、仕様文を同期修正** | **採用** |
| `HashCalculator` portを追加 | 不採用（純計算へportを作ることになり、§6の根拠から外れる） |
| 実装は直接、仕様修正は後回し | 不採用（CLAUDE.md §2が矛盾の先行同期修正を求める） |

§4.1の表から`HashCalculator`行を削除し、§7の列挙から`Hash`を外して「digest計算はportにしない。外部作用を持たない純計算で、同じ入力が常に同じ結果を返すため差し替える意味がなく、§2が『upstream SHA-256/SHA-512、内部SHA-256』を`internal/security`の責務としている」を明記した。

差し替えられないことによるtest上の不都合は無い。digest不一致のtestは、実際に違うbytesを流せば作れる。

## 3. 判断

### 3.1 2 passにしない

内部SHA-256とupstream digest（`sha256`または`sha512`）を**同じ走査で**計算する。downloadしたbytesを読み直すと、読み直しの間にfileが差し替わったときに「検証したbytes」と「使うbytes」が別物になりうる。1 passにすることで、両方が同じbytesに対する結論であることを構造で保証する。

内部SHA-256とupstreamが同じ`sha256`でも、別のhash instanceを持つ。共有すると、片方だけを読み出した時点でもう片方の状態に影響する書き方を誘発する。

### 3.2 上限超過を切り詰めずerrorにする

`ErrSizeLimit`をsentinelにした。呼出し側が「上限超過」と「読取り失敗」を区別して、partial fileの破棄理由を決められるようにするためである。超過後の書込みも同じerrorを返し、hashの状態を進めない。

### 3.3 algorithm違いを「不一致」で片付けない

`VerifyUpstream`は、期待digestのalgorithmが計算側と違う場合を専用のerrorにする。§7.2が「providerが公開したalgorithmでの照合を必須にする」と定めるため、`sha512`の期待値を`sha256`の計算結果と突き合わせて「一致しない」と報告すると、照合していないことが失敗の理由に見えなくなる。

### 3.4 panicさせない

`Internal`／`Upstream`のparseは、algorithmが構築時に検証済みでhex長がalgorithmから決まるため失敗しない。それでもpanicさせず、zero値を返す。digest計算の途中でprocessを落とすと書きかけの`.part`が残る（CLAUDE.md §9）。zero値は`IsZero`で検出でき、照合は必ず失敗する。`internal/definition`の`reason()`と同じ方針である。

## 4. 検査が固定したこと

| 検査 | 対象 |
|---|---|
| `TestStreamHasherComputesBothDigestsInOnePass` | sha256/sha512の両方で、内部digestが常にSHA-256、upstreamが指定algorithmであること |
| `TestStreamHasherIsStableAcrossReads` | 読出しを繰り返しても値が変わらないこと |
| `TestStreamHasherSplitsWritesIdentically` | 7 byteずつの分割書込みと一括で同じ結果になること |
| `TestStreamHasherEnforcesLimit` | 上限ちょうど、1 byte超過、超過後の書込み |
| `TestStreamHasherPropagatesReadError` | 読取り失敗を`ErrSizeLimit`と混同しないこと、途中までのSizeが残ること |
| `TestNewStreamHasherRejectsInvalidInput` | 上限0/負、未知algorithm、空、sha1（5件） |
| `TestStreamHasherVerifyUpstream` | 一致、不一致、未設定、algorithm違い |

## 5. 検証

すべてLinux containerで実行した（Go 1.26.6）。

| command | 結果 |
|---|---|
| `gofmt -l .` / `go build ./...` / `go vet ./...` | 出力なし・成功 |
| `go test ./... -race -shuffle=on -covermode=atomic` | 全package成功。`internal/security` 94.8% |
| `check_policy.py` / `check_imports.py` / `check_docs.py` / `check_licenses.py` / `check_messages.py` | すべて成功 |
| `check_pr_refs.py` / `git diff --check` | 成功・出力なし |

## 6. 未実施・制約

- **2本目の範囲が残っている。** `.part` fileへのstream書込み、progress通知、失敗時のpartial破棄、cache identity（URL＋digest）、offline判定は`claude/feature-p5-02-download-pipeline`で実装する。本PRはdigest計算の基盤だけである。
- **`StreamHasher`をまだ誰も使っていない。** 呼出し側は2本目のdownload pipelineである。CLAUDE.md §7の「使わないものは残さない」に対しては、同じtaskの2本目で必ず使う前提での分割であることを本記録と進捗台帳へ明示した。
- `SHA256Hex`は残している。state fileのatomic write（§4 step 7）が全量bytesを既にmemoryへ持つ場面で使っており、streamingへ置き換える理由が無い。
- §21のartifact download上限20 GiBは、実際にその大きさのstreamを流すtestを書いていない。上限判定は境界値（上限ちょうど／1 byte超過）で検査した。
