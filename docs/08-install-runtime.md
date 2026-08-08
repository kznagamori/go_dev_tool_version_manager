# 導入・選択・実行仕様

## 1. 適用範囲

本章はGo、Node.js、Python、.NET SDKの完全version導入、選択、shim実行、削除、診断を規定する。v0.1はdefinition schema 1の固定install手順だけを実行し、helper、backend manager、hook、任意scriptを実行しない。

変更操作は`Resolve → Plan → Approve → Execute → Commit`を分離する。Plan後にversion、artifact、checksum、definition、selection、configが変わった場合は`E_PLAN_STALE`とし、新しいPlanを要求する。

## 2. operation状態と進捗

```text
created → resolved → planned → approved → downloading → verifying
        → staging → validating → committing → cleaning → succeeded
任意の実行状態 → cancelling → cancelled
任意の実行状態 → failed
```

- `committing`完了前は導入済みとして列挙しない。
- commit後の一時file清掃失敗は成功＋`W_CLEANUP_INCOMPLETE`とし、正常payloadを巻き戻さない。
- cancelはdownload、checksum取得、展開entry、probeの境界で確認する。commit開始後は整合状態まで完了またはrollbackしてからcancel結果を返す。
- CLIはdownload byte/総量、速度、残り時間、現在段階、対象tool/versionを表示する。総量不明なら処理byteと経過時間を表示する。

この状態はprogress表示とlogのためのものであり、専用のoperation journal fileへ永続化しない。中断復旧は§6のstaging構造とatomic renameで担保する。

公開timeoutと同時download数は開始時にconfig snapshotへ固定する。network retryは組込み3回、TTY progress emit間隔は組込み100 ms（phase変更と完了は即時）として開始時snapshotへ固定する。schema・archive・metadata・path等の安全上限も[04-storage-and-data.md](04-storage-and-data.md)§21の組込み値でありconfigから拡大できない。

## 3. version解決

### 3.1 完全version

1. tool ID/aliasを正規IDへ変換する。
2. 現platformのdefinitionと検証済みcatalogを選ぶ。
3. 入力をtrim、補完、range展開せず、catalogの正規version文字列とbyte完全一致で探す。
4. artifact、provider、channel、lifecycle、checksum、storage、runtime、probeをPlanへ固定する。

`22.18`を`22.18.0`へ補完しない。該当しなければ`E_VERSION_NOT_FOUND`とし、`gdtvm available <tool>`を案内するが、近似versionを自動提案・選択しない。platform artifactがないversionは理由付き`installable=false`として表示する。

channel=prereleaseまたはlifecycle=eolの完全versionは導入できる。Plan冒頭と確認直前に警告し、非対話では`--yes`を要求する。lifecycle=unknownは状態を明示するがEOLと断定しない。

### 3.2 `--latest`

`--latest`はchannel=stable、lifecycle!=eol、かつ現platformでinstallableなversionだけをtoolのversion schemeで比較し、最大の完全version 1件へ解決する。unknown lifecycleを選ぶ場合は状態をPlanへ表示する。解決した完全versionをPlanへ明示し、operation中にcatalogが変わっても別versionへ切り替えない。候補0件・比較不能・同順位複数は失敗する。

## 4. Planと承認

Plan冒頭に次の重要要約を目立つ形で表示し、詳細表示後の確認直前にも同じ値を再掲する。

- tool、完全version、channel、lifecycle、platform
- providerの`official|third-party`区分と名称
- artifact digestのalgorithmと取得元
- definitionが`license_notice`を宣言している場合はそのlicense識別子
- EOL/prerelease、その他warning数と、明示承認が必要なwarning code

詳細部にはartifact URL/host/file/size、provider repository/homepage/license、third-party採用理由、checksum source/value、definition hash、展開parameter、probeの実行file/完全版/取得元/digest/license/reason/argv/cwd/書込み先、payload/storage/cache/selectionの全読書きpath、rollbackをすべて表示する。setup/setup-removeでは`SetupPlan`のmode、旧/新root、filesystem能力/link方式、shim、integration対象、backup、再起動要否を表示する。provider metadataにsizeがなく`size=0`なら「不明」と表示し、0 byteと誤表示しない。長いURL等は折りたためる表示にしても省略しない。

利用者状態を変える`setup|setup --remove|install|use|uninstall`はPlan後に1回確認する。完全なno-opはPlan/確認を省略して変更なしを返せる。catalog cacheをatomic置換する`available --refresh`、log rotation、所有と期限を検証した失敗tmp/cacheのcleanupはpayload、selection、storage、config、setup/integrationを変更しない運用データ更新であり、Plan/確認を要求しない。

`--yes`は[04-storage-and-data.md](04-storage-and-data.md)§16.1で明示承認が必要な7件すべてを承認できるが、警告表示・結果記録を消さない。checksum mismatch、archive違反、registry破損、path逸脱は承認で回避できない。

## 5. downloadとchecksum

### 5.1 network

- HTTPSだけを許可し、TLS検証無効化optionを提供しない。
- Go標準HTTP clientのproxy環境変数とOS trust storeを使う。独自CA/proxy credential storeは持たない。
- redirectごとにscheme、host allowlist、credential混入を再検査する。
- status、Content-Length、実受信量、Content-Type、metadata/body上限を検査する。
- `.part`へstreamしながら、cache identity用の内部SHA-256とprovider指定algorithm（`sha256|sha512`）のdigestを同じ1 passで計算し、progressを送る。同じalgorithmなら計算器を共有する。
- Range再開は同一URL、ETag/Last-Modified、expected sizeが一致するときだけ。serverが無視したら0 byteから再開する。
- network timeout/5xx/429だけ初回後に最大3回retryする。`Retry-After`なしは1秒、2秒、4秒、delta-seconds/dateが0～30秒なら各backoffとの大きい方を待つ。30秒超または不正値は長時間sleep/capをせずretryable `E_NETWORK`で停止する。404、schema違反、checksum mismatchはretryしない。
- `Content-Disposition`のfilenameを保存pathに使わない。

### 5.2 checksum

definitionの`asset-field|text-file`からPlan前にdigestを確定する。algorithmはproviderが公開したもの（`sha256`または`sha512`）を使い、`<algorithm>:<hex>`へ正規化して保持する。artifact受信時に同じalgorithmで計算した値と一致しなければ隔離または削除し、`E_CHECKSUM_MISMATCH`。`--yes|--force|--offline`で迂回しない。gdtvmがalgorithmを弱い方へ読み替えない。

v0.1はchecksumを公開しないartifactを扱わない。以後同じcatalog identityで別digestが得られた場合は自動更新せず失敗し、定義・upstream・cacheを調査する。

## 6. 安全な展開とstaging

展開前に全entryを列挙し、次を拒否する。

- absolute、drive/UNC、空component、`.`/`..`、NUL/control、ADS
- UTF-8不正、非NFC、Windows予約名・末尾dot/space・case-fold衝突
- duplicate、file/directory種別衝突
- symlink、hardlink、reparse point、device、FIFO、socket
- entry数、単一file、総展開量、圧縮比、path/component長の組込み上限超過

標準4 toolはarchive内linkを必要としないため両OSで拒否する。target OSのpath規則で検査し、Linux上のCIでもWindows衝突を検出する。`strip_components`とinclude/exclude適用後にもcontainmentと衝突を再検査する。

operation tmpは完成先と同じvolumeへ作り、`tmp/operations/<operation-id>/`配下だけを書く。payload/storage/currentへ直接書かない。permissionを正規化し、Linux executableのowner executeを保持しsetuid/setgidを除去する。

中断・失敗・cancel時は`tmp/operations/<operation-id>/`をdirectory単位で削除すれば復旧する。このdirectoryには当該operationが作成したものしか存在しないため、root ID、owner、作成時刻を検査したうえでまとめて削除できる。

## 7. validationとcommit

1. staging payloadの全pathがroot内にあることを再検査する。
2. sanitized最小環境で、**probe専用のowner-only temp directoryをcwdとして**required probeを実行する。呼出し元のcurrent directoryを継承しないため、利用者のproject file（`global.json`等）がprobe結果へ影響しない。
3. probeが報告する完全version、pip/venv等の必須能力を検査する。
4. required runtime commandのtargetとfixed argsが指すpayload内fileについて、相対path、size、SHA-256を`command_targets`へ記録する。payload全fileのmanifestは作らない。
5. payloadを通常利用でread/execute onlyへ正規化する。Windowsは現在userのwrite ACEを除き、Linuxはdirectory 0555、executable 0555、その他0444を基本とする。
6. receiptをstagingへ書きflushする。
7. 完成先がないことを確認し、version directoryを同一volumeでatomic renameする。
8. receipt indexをatomic更新する。indexが古い状態で中断してもreceipt走査から再構築できる。
9. `install --use`の場合だけ、install commit後に別selection transactionを行う。

手順7のrenameが完了した時点でinstallは成功とみなす。rename前の中断は未導入、rename後の中断は導入成功でindexだけ古い状態であり、次回起動時の再構築で解消する。

完成先が競合して作られた場合、両receiptと`command_targets`が完全一致すれば後発stagingを破棄して成功、違えば`E_CONFLICT`。probe失敗やcommit前cancelはstagingを破棄し、既存install/selectionを変更しない。

required probeの起動後nonzero、timeout、output上限、version/root/path/能力不一致は`E_PROBE_FAILED`。実行file欠落やdefinition参照不正は`E_DEFINITION_INVALID`、permission/OS起動失敗は対応するplatform/filesystem errorとし、probe stderr末尾はmask/上限後だけhuman errorへ含める。

## 8. typed storage

storage pathはdefinitionの`kind`と`scope=tool|version`からengineが生成し、receiptへ固定する。tool IDを見たGo分岐で共有可否を決めない。

- `scope=tool`: 全managed versionで共有する設定、cache、global binary等。
- `scope=version`: ABI/API差がありversionごとに隔離するglobal package/runtime data等。
- 通常uninstallはversion scopeを対象version directoryと一緒に削除し、tool scopeを保持する。
- `--purge-shared`は最後のmanaged versionで参照がなく、`purge="explicit"`のtool scope領域だけを同じuninstall Planのdestructive warningへ追加し、全変更を1回確認して削除する。

標準toolの割当ては[07-registry-and-tools.md](07-registry-and-tools.md)を正とする。GoのGOBINはtool scope、Node.js global packagesはversion scope、Python site-packagesはversion scope、.NETの`DOTNET_CLI_HOME`はversion scopeでNuGet cache群はtool scopeとする。Python project dependencyはvenvを推奨する。

上流toolが管理rootへ隔離する公開手段を持たない領域は、typed storageとして宣言せず、公開文書で管理外と明示する。推測でstorageを宣言してPlanやreceiptに実態と異なるpathを記録しない。

## 9. selection

`gdtvm use <tool>@<version>`は導入済みでhealthyなreceiptだけを選ぶ。未導入の場合、configで許可されていれば同じ完全versionのinstall Planを提示できるが、近似version・latestへ変えない。

1. selection revisionとreceipt identityを検査する。
2. 新`selections.toml`をtempへ書きflushする。
3. 新current junction/symlinkをtemporary名で作り、targetを検査する。
4. stateとlinkをcommitし、shim indexを更新する。

Windows junction置換中の短い欠落は許容するが、shimはstate/receiptを正としてdirect targetを解決できるようにする。失敗時は旧selectionを復元するか、stateが新しくlinkだけ古い場合は成功＋`W_SELECTION_LINK_INCONSISTENT`とし、曖昧なtargetを実行しない。warningが出た場合は`use`の再実行で解消できる。

### 9.1 project selection

projectは`.gdtvm.toml [tools]`の完全versionだけを選ぶ。fileがなければ`schema=1`と最小`[tools]`を作り、元digestがPlan後に変われば上書きしない。project指定はuser selectionより優先する。解決順序はproject exact、user selection、noneとする。

## 10. shimとruntime

`gdtvm[.exe]`はCLIとnative shim resolverを兼ねる。起動basenameが`gdtvm`ならCLI、shim indexの公開commandならCLI/network初期化前にruntimeへ分岐する。command shimはWindowsでhardlinkまたは同release内蔵小型resolver、Linuxでrelative symlinkを使い、tool本体をcopyしない。

resolverは次を行う。

1. 呼出basenameを厳密に正規化してshim indexの所有toolを1件に決める。0件/複数件は失敗する。
2. nearest project fileを上向き探索し、project完全version、user selectionの順に選ぶ。
3. selection、receipt、`command_targets`、command target、storage pathを検査する。
4. 親環境をcopyし、receiptのenvironment profileをcase規則どおりmergeする。
5. targetと固定argsと利用者argsをargv配列として組み、shellを介さずstdio継承で起動する。
6. signal/console controlと終了codeを透過する。

targetを一般PATHから再探索せずreceiptのabsolute pathを使う。shim自身をtargetにしない。再帰markerはdepth 8で拒否する。shimはdownload、prompt、registry更新を行わない。

管理toolの環境はgdtvm管理home/cache/configへ隔離する。利用者はGo=`go env -w`、Node.js=`npm config`、Python=`pip config`/venv、.NET SDK=`dotnet nuget`/`dotnet tool`等の上流標準手段を使い、gdtvmはその保存先をprofileでredirectするだけで内容を独自解釈しない。上流が隔離手段を持たない領域は§8どおりtyped storageとして宣言せず、管理外として公開文書へ明示する。

## 11. current

`current`はscope、完全version、provider、target、health、project sourceを表示する。`--explain`は探索したproject file、候補、PATH resolution、inactive理由を追加する。

## 12. uninstall

1. 対象exact receipt、user/project selection、既知参照を調べる。
2. 参照があれば拒否する。`--force`は列挙済みuser selectionと明示project fileだけ解除でき、他installの依存や未知projectは壊さない。
3. Plan/確認後、selection解除を先にcommitする。
4. version directoryを同一rootのtrash名へatomic renameする。
5. receipt index、current、shim indexを更新する。
6. trashを削除する。削除失敗は非導入trashとして`doctor`対象にする。

OS全体を走査してprojectや使用中processを探さない。Planにはterminal、editor、language server終了の注意と削除するversion-scope storageを出す。Windows sharing violation時はprocessをkillせず、payload/receiptを変更せず停止する。削除時にpermission変更が必要ならlock取得後、`command_targets`一致を再確認した対象version directory内だけを一時的にowner write可能へ戻す。tool-scope shared storageとproject venv/sourceは§8どおり明示shared purge以外で削除しない。

## 13. doctor

`doctor`はread-onlyでregistry、config/state、receipt/payload、selection/current、shim、storage permission、root一致、stale tmpを10個の`Diagnostic.code`（[04-storage-and-data.md](04-storage-and-data.md)§17.1）として診断する。全体statusはerrorがあれば`unhealthy`、errorなしでwarnがあれば`degraded`、それ以外を`healthy`とする。次を変更しない。

- payload、storage、selection、config、setup/integration
- registry、receipt、catalog

診断項目は最低限、次を含む。

| 項目 | 検出する状態 |
|---|---|
| root | portable rootが移動している。data rootのownerが違う |
| state | strict parse失敗、revision不整合、root ID不一致 |
| registry | header/digest不一致、schema非互換 |
| receipt | 破損、index不一致、payload欠落 |
| payload | `command_targets`のsize/SHA-256不一致、permissionがread/execute onlyでない |
| selection | receiptがないselection、current linkの不一致 |
| shim | index不整合、shim実体の欠落 |
| PATH | shim directoryがeffective PATHに含まれない |
| storage | 他user/world書込み可能 |
| tmp | 期限切れのoperation directory、trash残留 |

`--report <path>`は診断結果と環境情報をMarkdown 1 fileへ書く。内容と除去規則は[10-security.md](10-security.md)§9を正とする。

自動修復commandはv0.1に持たない。`doctor`が示す状態に対しては、`setup`再実行、`use`再実行、対象versionの再installという既存commandで対処する。修復手順の案内は診断結果へmessage IDとして含める。

## 14. offline/cache

`--offline`はnetworkを一切使わない。exact catalog item、artifact、checksum metadataが検証済みcacheに揃う場合だけPlanを作る。期限切れcatalogはexact解決に必要な固定metadataが完全なら`W_CACHE_STALE`付きで利用できるが、`--latest`は期限切れ情報から黙って決めない。

cacheはdigest/identityで参照し、利用者指定filenameやURL basenameをpathにしない。削除はreceiptから参照されないentryだけを対象にする。

## 15. 長寿命process

selectionやcurrent linkの変更は既に動いているterminal、editor、language server、serviceのprocess環境や保持済み実体pathを変更しない。`use`成功時は必要に応じてterminal再起動またはVS Code Reload Windowを案内する。

serviceやCIは暗黙のuser selectionではなくproject完全versionを使う。project fileが置けない環境ではshim directoryを含まないPATHでabsolute pathを直接指定する。
