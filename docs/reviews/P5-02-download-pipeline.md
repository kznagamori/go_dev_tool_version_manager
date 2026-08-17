# P5-02 決定記録（2/2）: download pipeline（P5-02完了）

対象タスク: `docs/13-progress.md` P5-02の2本目。規範仕様は[10-security.md](../10-security.md)§7.2・§9.2・§10、[04-storage-and-data.md](../04-storage-and-data.md)§7・§17.2・§21、[05-configuration.md](../05-configuration.md)§3.5、[02-architecture.md](../02-architecture.md)§2・§10・§14、[15-deferred.md](../15-deferred.md) D-24。

## 1. 着手時の確認事項（1本目の停止記録より）

2件とも既存のもので足りた。

| 確認項目 | 結果 |
|---|---|
| §17.2の`download-cache`／`staging` path role | `domain.RoleDownloadCache`／`RoleStaging`として実装済み。`download-cache`の定義が「download cache fileまたはその**partial metadata**」であり、`.part`の存在を仕様が想定している |
| `internal/progress`のProgress型 | `PhaseDownload`／`PhaseVerify`、`Current`、`Total *int64`、`Unit`（`UnitBytes`）、`Rate`を持ち、byte数の通知にそのまま使える。**変更不要** |

## 2. 利用者判断: `port.FileSystem`へ`WriteStream`を追加した

**`port.FileSystem`が書込みを`AtomicWrite(path, data []byte, perm)`だけに絞っていた。** portのコメントが理由を明記している。

> 書込みをAtomicWriteだけに絞っているのは、docs/10-security.md が要求する「中断しても壊れた状態を残さない」を型で強制するためである。**部分書込みが観測できるAPIを置かない。**

一方でP5-02は`.part` streamを要求し、artifactは最大20 GiB（§21）、§3.5は「全量memoryへ載せない」と定める。`AtomicWrite`は全量bytesを受け取るため使えない。

| 選択肢 | 結果 |
|---|---|
| **`WriteStream(path, perm, src io.Reader)`を追加** | **採用** |
| `CreatePart` で`io.WriteCloser`を返す | 不採用（部分書込みが観測できるhandleをportが配ることになり、設計意図から最も遠い） |
| `AtomicWrite`のまま進めない | 不採用（20 GiBをmemoryへ載せることは§3.5が禁じ、代替手段が無い） |

`WriteStream`はreaderを受け取るため、**書込みhandleを呼出し側へ渡さない**というportの性質を保てる。progressとdigestはreader側をwrapして得る（`io.TeeReader`＋1本目の`StreamHasher`）。

portのコメントを「書込みhandleを呼出し側へ渡さない」を主眼に書き直し、`AtomicWrite`と`WriteStream`の使い分けと、**原子性は単一呼出しではなく`.part`書込み→digest照合→renameという手順が担保する**ことを明記した。§4.1のFileSystem行へ`stream write`を追加した。

`WriteStream`は**renameしない**。digest照合がrenameをgateする契約（§7.2）を呼出し側が持つためで、port側でrenameすると未検証のbytesが最終pathへ現れる。

## 3. 判断

### 3.1 cache再利用はupstream digestを再計算して判定する

内部SHA-256だけを見ると、providerがsha512を公開しているartifactでalgorithmの一致を確かめられない。既存cacheを開いて**期待upstream digestと同じalgorithmで再計算**し、一致した場合だけ再利用する（§10「URL identityとdigestが一致するcomplete fileだけを再利用」）。

`.part`はcacheとして扱わない。完成fileと途中のfileをbasenameで区別する理由がこれで、partial fileのRange再開は行わない（D-24）。

### 3.2 sizeが宣言と食い違う応答を通さない

digestが一致してもsize宣言と違う場合は拒否する。catalogとPlanが表示するsizeが実体と違うことになるためである。`ExpectedSize=0`はproviderがsizeを公開していない状態（§16）なので照合しない。

progressの総量は「宣言size → 応答の`Content-Length` → 通知しない」の順で決める。0を総量として出すと進捗率が常に100%か0%になる。

### 3.3 offlineと一時障害を分ける

DNS解決失敗と経路不達（`ENETUNREACH`／`EHOSTUNREACH`）を`E_OFFLINE`、それ以外のnetwork失敗を`E_NETWORK`にした。利用者が取るべき行動が違い、`E_NETWORK`は再実行で直りうる一時障害、`E_OFFLINE`は接続そのものが無い状態を指す。connection refusedは到達できたうえでの失敗なので一時障害側である。

checksum不一致は`Retryable=false`にした。同じbytesを取り直しても同じ結果になる（§14）。

## 4. 実装中に見つけた欠陥1件

`TestDownloadMasksCredentialInError`が失敗した。**`fake.HTTPClient`が未登録URLのerror messageへ生URLを載せていた。**

```text
install: https://…?access_token=<redacted> の取得に失敗した:
  fake: no stub registered for https://…?access_token=SECRETVALUE
```

P5-01で`internal/platform`側の同種の漏れ（`*url.Error`）を直したが、fakeは別経路である。fakeのerrorもtest logやtest failureの出力へ出るため、credential付きURLを扱うtestがfake経由でsecretを漏らす。

fakeは§1の依存方向により`internal/domain`配下しかimportできず`security.MaskURL`を使えないため、userinfo・query・fragmentを落とす`safeURL`をfake内へ置いた。maskの正本が`internal/security`であることはコメントへ明記した。

なお`TestDownloadOmitsTotalWhenSizeUnknown`も失敗したが、こちらは**testの期待が誤り**だった。宣言sizeが0でも応答が`Content-Length`を返せば総量として使えるのが正しい挙動である。testを`TestDownloadFallsBackToContentLength`へ直し、総量が本当に不明な場合は`countingReader`の直接testで検査した。

## 5. 検査が固定したこと

| 検査 | 対象 |
|---|---|
| `TestDownloadWritesVerifiedFile` | `.part`書込み→digest照合→renameの手順、`.part`が残らないこと |
| `TestDownloadReportsByteProgress` | byte単位のprogress、Currentの単調非減少、最終値 |
| `TestDownloadFallsBackToContentLength` / `TestCountingReaderOmitsUnknownTotal` | 総量の決め方3段 |
| `TestDownloadDiscardsPartialOnDigestMismatch` | digest不一致で`.part`も最終pathも残さないこと、`Retryable=false` |
| `TestDownloadRejectsSizeMismatch` | 宣言sizeと違う応答 |
| `TestDownloadReusesCache` / `RefetchesCorruptedCache` / `IgnoresPartialAsCache` | cache再利用の3経路 |
| `TestDownloadSupportsSHA512` | sha512 providerでも内部digestはSHA-256 |
| `TestDownloadReportsNetworkFailure` / `MasksCredentialInError` | typed error、secretを載せないこと |
| `TestDownloadRejectsInvalidRequest` | URL空、digest未設定、path空、role違い、`.part` path、size負、redirect負（7件） |
| `TestDownloadDiscardsPartialOnWriteFailure` | 書込み失敗時のpartial破棄（failure injection） |
| `TestIsOfflineDistinguishesUnreachableNetwork` / `TestNetworkErrorSelectsCode` | offlineと一時障害の区別（6件） |
| `TestFileSystemWriteStream*`（fake） | 読取り失敗で書きかけを残さないこと、読み切った内容を書くこと |
| `TestHTTPClientErrorOmitsCredential`（fake） | fakeのerrorへcredentialを載せないこと |

## 6. 検証

すべてLinux containerで実行した（Go 1.26.6）。

| command | 結果 |
|---|---|
| `gofmt -l .` / `go build ./...` / `go vet ./...` | 出力なし・成功 |
| `go test ./... -race -shuffle=on -covermode=atomic` | 全package成功。`internal/install` 88.7%、`internal/domain/port/fake` 88.1% |
| `check_policy.py` / `check_imports.py` / `check_docs.py` / `check_licenses.py` / `check_messages.py` | すべて成功 |
| `check_pr_refs.py` / `git diff --check` | 成功・出力なし |

## 7. 未実施・制約

- **`port.FileSystem`のproduction実装がまだ無い。** `WriteStream`を実装したのはfakeだけで、実OS上の挙動（同一filesystem上のrename、cancel時の削除）は未検証である。production adapterはP8-01の`Ports`組立てと同じ範囲になる。
- **cancel経路を実際のcancelでtestしていない。** `WriteStream`が`context.Canceled`を返した場合の分岐は書いたが、fakeのWriteStreamはcontextを受け取らない。P5-04のProcessRunnerでcancel境界を扱うときに、port全体のcancel伝播をまとめて見直す必要がある。
- **`Rate`を通知していない。** `progress.Progress`は`Rate *float64`を持つが、算出には経過時間が要り`port.Clock`をDownloaderへ持ち込むことになる。§10は`Rate`を必須としていないため、TTY表示を実装するP8-05でまとめて扱う方が確実である。
- **offline判定はsyscall errnoに依存する。** `ENETUNREACH`／`EHOSTUNREACH`はWindowsでも`syscall`packageに定義があるが、実際にその値が返るかは両OSのCI unit testでは検証できていない（fakeがsyscall errorを作れない）。実挙動は§9の利用者確認チェックリストの範囲である。
- extract、receipt、transactionはP5-03以降の範囲である。
