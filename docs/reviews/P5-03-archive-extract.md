# P5-03 決定記録（2/2）: archiveの実展開

対象タスク: `docs/13-progress.md` P5-03の2本目。規範仕様は[10-security.md](../10-security.md)§5、[08-install-runtime.md](../08-install-runtime.md)§6・§7、[04-storage-and-data.md](../04-storage-and-data.md)§17.2・§21、[02-architecture.md](../02-architecture.md)§2・§4。

## 1. 着手時の確認事項（1本目の停止記録より）

2件とも解決した。

### 1.1 symlink raceは`Stat`／`RealPath`で塞げる

[10-security.md](../10-security.md)§5の「検査と実書込みの間にもparent identity/containmentを確認し、symlink raceを防ぐ」を、次の3点で満たす。`port.FileSystem`に`openat`相当の**handleを返すmethodは要らない**。

| 段階 | 検査 | 使うmethod |
|---|---|---|
| directory作成後 | linkでないこと、directoryであること、解決後がpayload root内であること | `Stat`（symlinkを解決しない）＋`RealPath` |
| file書込み前 | 書込み先に**何も無い**こと | `Stat`が`fs.ErrNotExist`を返すこと |
| file書込み後 | 通常fileで、書いたbyte数と一致すること | `Stat` |

書込み先の不在検査が要になる。重複entryは事前検査が拒否済みなので、書込み直前に実体があること自体が「検査後に外から置かれた」ことを意味する。`WriteStream`はlinkを辿って書くため、この1件で塞ぐ。

残る限界は明示しておく。`Stat`と`WriteStream`の間は依然として別呼出しであり、`openat`+`O_NOFOLLOW`のような原子性は無い。staging directoryはoperation専用でowner限定（[08-install-runtime.md](../08-install-runtime.md)§6）であり、そこへ書ける相手は既に同等の権限を持つ、という前提での防御である。

### 1.2 `staging`と`payload`の境界

[04-storage-and-data.md](../04-storage-and-data.md)§17.2は`staging`を「operation staging/probe-temp directoryとその子path（**payloadとして扱う展開後内容を除く**）」、`payload`を「完成または**staging内の**tool payloadとその子path」と定める。したがって、

- `tmp/operations/<operation-id>/` そのものとprobe用temp = role `staging`
- その中の展開先とその子path = role `payload`

`ExtractRequest`は`StagingRoot`（role `staging`）と`Dest`（role `payload`）を別々に受け取り、要求検査でroleを固定する。あわせて`archiveUnsafe`のroleを`staging`から`payload`へ直した。archive entryが行き着く先はstaging内であってもpayloadであり、同§が「最も具体的なroleを使う」と定めるためである。

## 2. 上限を2段で判定する

1本目の記録で「宣言sizeで事前、実展開bytesで展開中」と決めた後半を実装した。

| 判定 | 対象 | 実装 |
|---|---|---|
| 宣言size（事前） | entry数、単一file、総展開、圧縮比 | `InspectEntries` |
| 実bytes（展開中） | 宣言size一致、単一file、総展開 | `extractReader` |
| 展開後stream（tar） | 総展開 | `limitedReader` |

実測bytesの上限は`min(20 GiB, archive size × 1,000)`とする。tarのentryは個別の圧縮後sizeを持たず事前検査で比を出せないため、実測に対して効く分母はarchive file自身のsizeだけである。headerとcentral directory込みで分母がわずかに大きくなるが、緩い側へ倒れるので通すべきarchiveを拒否しない。

`openTarGz`が展開後streamへ上限を掛けるのは、entryを1件も書かない`tar.Reader.Next`でもgzipが展開を進めるためである。上限が無いと、header間に詰めたbombがfileを作らずにCPUとmemoryを消費できる。

### 2.1 stdlibが先に止めることを安全性の根拠にしない

宣言sizeを偽るzipは、`archive/zip`のchecksumReaderが読んだbytesが`UncompressedSize64`を超えた時点で`zip.ErrFormat`を返して止める。tarも宣言sizeで読取りを打ち切る。それでも同じ超過を`extractReader`で独立に検出する。§5の上限をstdlibの実装詳細へ委ねないためである。

testもそのとおりに分けた。end-to-endの`TestExtractCutsOffLyingEntry`は「宣言を偽るarchiveが`E_ARCHIVE_UNSAFE`で拒否され1 byteも残らない」ことだけを固定し、自前の打ち切りは`TestExtractReaderCutsOffDeclaredSize`が直接見る。

## 3. 判断

### 3.1 `OpenAt`をportへ追加した

1本目の記録は「新しいport methodは不要」と結論していたが、**それは書込み側だけを見た結論だった**。`archive/zip`はcentral directoryがfile末尾にあるため[io.ReaderAt]を要求し、`Open`のsequential readでは読めない。

`Open`の戻り値を`io.ReaderAt`へtype assertする案は採らない。実`*os.File`では成功しfakeでは失敗するため、port契約が暗黙になりtestで確かめられない。random access読取りは外部作用そのものであり、§4の一般則（portにするのは外部作用そのものだけ）にそのまま当てはまる。読取り専用のhandleだけを返し、書込みhandleを配らない設計は崩していない。

### 3.2 tarは2 passにし、2 pass目で検査結果と突き合わせる

§5が「archiveは**展開前に**全entryを検査」と定める一方、tarはsequential formatで全entryを知るには最後まで読むしかない。header収集と展開の2 passに分けた。

同じfileを2回開く以上、その間に差し替えられる余地がある。2 pass目では各headerをもう一度`checkEntry`へ通し、strip後のpath・種別・size・executable bitが1 pass目と一致することを確認する。1件でも違えば`E_ARCHIVE_UNSAFE`とする。zipはcentral directoryを1回読むだけなのでこの問題が無く、検査と展開で同じ`zip.Reader`を使う。

### 3.3 permissionはarchiveのmodeを運ばず固定値へ正規化する

[08-install-runtime.md](../08-install-runtime.md)§6の「permissionを正規化し、Linux executableのowner executeを保持しsetuid/setgidを除去する」を、`Entry`へ`Executable bool`だけを持たせて実装した。mode全体を運ばないため、**setuid/setgidは構造上残らない**。落とし忘れる経路がそもそも無い。

directory 0755／executable 0755／その他0644とする。commit時のread-only化（同§7手順5）は別段階であり、展開中はownerが書ける必要がある。

### 3.4 CRC不一致をfilesystem失敗と混ぜない

`WriteStream`はsrc（archiveのdecompressor）が返したerrorをそのまま返す。分類しないとCRC不一致や壊れたheaderが`E_FILESYSTEM`になり、利用者はdiskを疑うことになる。`zip.ErrChecksum`／`zip.ErrFormat`／`tar.ErrHeader`／`io.ErrUnexpectedEOF`を`E_ARCHIVE_UNSAFE`へ寄せた。

封じ込め違反は`E_PATH_UNSAFE`として`E_ARCHIVE_UNSAFE`と分ける。archiveの内容ではなく展開先の状態が原因であり、利用者が見る場所が違う。

### 3.5 cancelはsrc側から伝える

`WriteStream`はcontextを受け取らない。`extractReader.Read`が毎回`ctx.Err()`を見て、cancel後の読取りをそこで止める。`WriteStream`は失敗時に書きかけを消す契約なので、部分fileは残らない。P5-02の停止記録が「cancel経路を実際のcancelでtestしていない」としていた点は、これで展開側については解消した。

失敗・cancel時は展開先を`RemoveAll`する（§6）。中途半端なpayloadを残すと、次のinstallがそれを完成物と見分けられない。

### 3.6 `InspectResult`を1回の走査へ直した

1本目の記録が残した「検査後に同じ走査をもう一度行う」を、展開側の使い方が固まったので解消した。`InspectResult.Paths []string`を`Entries []InspectedEntry`（`Index`／`Path`／`Kind`／`Size`／`Executable`）へ変え、検査と結果組立てを同じloopで済ませる。`Index`が要るのは、`strip_components`で消えるtop-level directoryのぶん位置が連番にならず、formatごとのreaderと対応づけられなくなるためである。

## 4. 併せて直したもの

### 4.1 fakeのMkdirAllが既存entryを置き換えていた

**testが検出した欠陥である。** `fake.FileSystem`の`mkdirAllLocked`が「既存entryがdirectoryでなければ作り直す」という実装で、展開先に置いたsymlinkをdirectoryへ差し替えていた。実OSの`MkdirAll`はfileやsymlinkのある位置をdirectoryへ作り替えない。

このままだと**symlink raceのtestが素通りする**——防ごうとしている状態そのものをfakeが消してしまう。既存entryを置き換えないよう直し、`TestExtractRejectsSymlinkedDirectory`が実際に検出することを確かめた。

### 4.2 `IsContained`がWindowsで`/`区切りを見ていなかった

Windowsは`\`と`/`の両方をpath区切りとして扱い、`D:\gdtvm`と`D:/gdtvm/bin`は同じ位置を指す。片方だけで分けると同じ位置を別物と判定し、封じ込め検査が誤って失敗する。両方を区切りとして分けるよう直した。連続した区切りも1つとして扱う。

あわせてcomponent 0件のrootを拒否した。空文字列やfilesystem rootを封じ込め先として受けると、何を渡しても「配下」になりfail openする。

### 4.3 存在しない`VerifyContainment`へのgodoc参照

`security.Join`のコメントが`[VerifyContainment]`を指していたが、その関数は無い。realpathへの解決はfilesystemを触るため純計算の`internal/security`には置けず、実体は「呼出し側が解決してから`IsContained`」である。コメントを実態へ合わせた。

## 5. 検査が固定したこと

| 検査 | 対象 |
|---|---|
| `TestExtractZipCreatesTree` | 両OSで同じ構成・path順・件数・実bytesになること |
| `TestExtractTarGzCreatesTree` | formatの違いが展開後の構成へ出ないこと |
| `TestExtractStripComponentsZero` | top-level directoryなしlayout |
| `TestExtractNormalizesPermissions` | dir/file/executableの正規化とsetuid除去 |
| `TestExtractCutsOffLyingEntry` / `TestExtractRejectsShortEntry` | 宣言と実体が食い違うarchiveの両方向 |
| `TestExtractReaderCutsOffDeclaredSize` | 自前の宣言size打ち切り |
| `TestTreeWriterCutsOffTotalBytes` / `TestTreeWriterCutsOffSingleFileLimit` | 総展開と単一fileの実bytes上限 |
| `TestLimitedReaderStopsAtLimit` / `TestMaxExtractBytes` | 展開後streamの上限と分母の決め方（境界含む） |
| `TestExtractRejectsUnsafeEntry` | 違反1件で安全なentryも書かないこと |
| `TestExtractRejectsEntryAppearingAfterInspection` | 検査後に現れた実体 |
| `TestExtractRejectsSymlinkedDirectory` / `TestExtractRejectsDestOutsideStaging` | link差し替えと解決後の封じ込め違反 |
| `TestExtractRemovesDestOnCancel` / `TestExtractReaderStopsOnCancel` | cancelの伝播と後始末 |
| `TestExtractTarGzDetectsSwappedArchive` | 2 passの間の差し替え |
| `TestExtractRejectsCorruptArchive` / `TestExtractRejectsCorruptContent` | 壊れたarchiveとCRC不一致 |
| `TestExtractFailureInjection` | filesystem操作11経路の失敗注入と後始末 |
| `TestExtractTarGzFailureInjection` | tar 2 passそれぞれのopen失敗 |
| `TestExtractRejectsInvalidRequest` / `TestExtractRejectsUnknownFormat` / `TestExtractRejectsNonRegularArchive` | 要求の前提違反 |

## 6. 検証

すべてLinux containerで実行した（Go 1.26.6）。

| command | 結果 |
|---|---|
| `gofmt -l .` / `go build ./...` / `go vet ./...` | 出力なし・成功 |
| `go test ./... -race -shuffle=on -covermode=atomic` | 全package成功。`internal/install` 92.0% |
| `check_policy.py` / `check_imports.py` / `check_docs.py` / `check_licenses.py` / `check_messages.py` | すべて成功 |
| `check_pr_refs.py` / `git diff --check` | 成功・出力なし |

## 7. 未実施・制約

- **`port.FileSystem`のproduction実装がまだ無い。** `OpenAt`と`WriteStream`を実装したのはfakeだけで、実OS上のrename、cancel時削除、Windowsのpermission写像は未検証である（P8-01の`Ports`組立てと同じ範囲）。
- **`./`始まりのtar entryを拒否する。** §5が「空component、`.`/`..`」を拒否と定めるため`checkEntry`が`.`componentを通さない。GNU tarが`tar -c .`で作るarchiveはこの形になる。標準4 toolの実archiveを繋ぐのはP6以降であり、そこで実際に`./`始まりのものがあれば、仕様側でどう扱うか（正規化か拒否か）を先に決める必要がある。**推測で正規化を入れない。**
- **展開後の実bytesに対する圧縮比の分母はarchive file size**とした。§21が「圧縮比（entry/全体）1,000」としか書いておらず、「全体」の分母を一意に指定していない。事前検査（entry圧縮後sizeの合計）と展開中（archive file size）で分母が違うことを本記録で明示した。
- `Extractor`をまだ誰も使っていない。呼出し側はP6のinstall Plan/transactionである。
- probe、receipt、`command_targets`、commit時のread-only化とatomic renameは[08-install-runtime.md](../08-install-runtime.md)§7の範囲で、P6・P7である。本PRはstagingへ展開するところまでを担う。
- **仕様側の未決が2件継続**（§5第2項のlicense file size上限が§21の表に無い、§2の「license file名はASCII kebab grammar」が§2自身の例と食い違う）。`python.toml`の`lifecycle = "unknown"`はP3-04から継続。source error専用のerror code（`E_DEFINITION_INVALID`）、version数値要素の64 bit上限、`cache_ttl` 1分〜30日・pointer 255 byte・regex 1024 byte・SPDX expression 128 byte・hostname 253 byte、`logs/` file名規約と§11「専用lock」の解釈は未決である。
