# セキュリティ仕様

## 1. 方針と信頼境界

v0.1は個人・小規模開発で安全に完全versionを導入するための最低限を必須とし、企業向けpolicy engine、中央承認、専用audit、custom CA/credential管理を作らない。企業は開発toolを公式のinstall文書に従って導入すると想定し、gdtvmはその代替や監査基盤を目指さない。

一方、archive traversal、管理root逸脱、checksum不一致、command injection、credential漏えい、他user書込みは利用形態を問わずfail closedとする。これらは企業向け機能ではなく、個人利用でも壊れると実害が出る境界である。

信頼境界は次のとおり。

1. tag commitからbuildされたgdtvm clientと同梱registry。
2. 公式GitHub repositoryのclient release metadata、canonical `checksums.txt`、archive。
3. standard definitionが指定するupstream/third-party metadataとartifact。
4. 利用者のconfig/project file、environment、PATH、filesystem、network response。
5. managed payload。

1〜3も無検査で信頼せず、各契約のschema、digest、path、versionを検査する。4〜5はuntrusted inputとして扱う。

## 2. client release

client release検証は次に固定する。

- binaryに公式GitHub repository owner/name、client version、commit、targetを埋め込む。
- release tag/name、asset filename/sizeを検査する。
- 同releaseのcanonical `checksums.txt`から対象archiveのSHA-256をexactly 1件解決する。
- download archive SHA-256、許可されたarchive構造、binary version/targetを照合する。
- release後に公開assetを再downloadしてdigest/構造/versionを再検査する。公開済みassetを上書きしない。

署名検証（Ed25519、PGP、Minisign等）、署名manifest、SBOM/provenance/attestationはv0.1に含めない。したがって上記は転送破損・asset取り違え・不整合を検出するが、公式repository/account/build環境の侵害まで独立に証明するものではない。**この保証範囲をREADME/USER_GUIDEへ明記する。**

bootstrap script自体は同releaseからHTTPSで取得する。scriptはarchiveとchecksumsを取得して照合するが、自分自身の真正性を自己証明できない。公開文書は直接pipe実行を唯一の手段にせず、downloadして内容/公開元を確認してから実行する方法と、archiveを手動検証する方法を併記する。

client自身を更新するcommandはv0.1に存在しない。更新はbootstrap scriptの再実行または手動archive展開で行う。bootstrapは新releaseをstagingで完全検証してからactive `distribution/current`をdirectory単位で置換するが、開発版であるv0.1では旧distributionのbackupと自動rollbackを持たない。失敗時や旧版へ戻す場合は、開発利用者が完全client versionを指定して再取得する。tool payload、state、storageはdistribution置換の対象外とする。distribution隣接の任意`gdtvm.toml`は利用者dataなので、owner/regular file/sizeを検査してraw bytesを新stagingへ引き継ぎ、旧binaryと一緒に削除しない。引継ぎ完了前にcurrentを除去せず、commit失敗時は所有するoperation tempの設定copyを復旧可能位置として報告する。

## 3. registry

runtimeはdistribution同梱registryだけを読む。local/network registry、override、pluginを探索しない。`registry.toml`の各file digest、schema、client互換、許可entry集合を検査し、不一致なら通常のcatalog/install/useを拒否する。

registry treeに対する独自のcanonical hashをbinaryへ埋め込まない。archive全体の完全性はrelease archiveのSHA-256で担保する。

問題のあるartifactが判明した場合の対応は、当該entryを外した新しいclient releaseの公開と告知である。実行時のrevocation listはv0.1に存在しない（[15-deferred.md](15-deferred.md) D-05）。

## 4. tool artifact

公式portable artifactを優先する。公式が非root、複数version、portableの製品要件を満たせない場合だけthird-party artifactを標準definitionへ採用でき、provider、repository、license、採用理由をPlanに表示する。PythonはWindows/LinuxともAstral `python-build-standalone`であり、この区分を隠さない。

公式providerの配布物でも、licenseがOSI承認OSS licenseでないplatformには`license_notice`を宣言し、Planの重要要約で明示承認を求める。.NET SDKのWindows配布物（.NET Library License）が該当する。`official`区分であることが「利用条件が緩い」ことを意味しないため、区分表示だけに委ねない。

**upstream checksumが公開されているartifactだけを採用する。** Plan前にproviderが公開したalgorithm（`sha256`または`sha512`）でdigestを確定し、download直後に同じalgorithmで必須照合する。値の欠落・重複・algorithm違い・hex長不一致・mismatchは承認で回避できない。gdtvmがalgorithmを弱い方へ読み替えない。

checksumを公開しないproviderのartifactはv0.1で扱わない。将来必要になった場合は、Plan表示、承認category、receiptの検証状態表現、offline再利用条件を[15-deferred.md](15-deferred.md) D-06のgateで先に仕様化する。

## 5. archiveとpath

archiveは展開前に全entryを検査し、absolute/drive/UNC、`..`、NUL/control、ADS、invalid/non-NFC Unicode、Windows予約名/case衝突、duplicate、symlink/hardlink/reparse、特殊file、size/count/ratio超過を拒否する。検査と実書込みの間にもparent identity/containmentを確認し、symlink raceを防ぐ。

すべてのwrite/move/delete/link targetはlogical管理rootから組み立て、canonical parent containmentとownerを検査する。削除はreceiptまたはsetup stateで所有を証明できるentry、および`tmp/operations/<operation-id>/`のようにoperation専用と構造上保証されたdirectoryだけ。filesystem root、home全体、workspace root、管理外pathを再帰対象にしない。

receiptの`command_targets`が指すfileのsize/SHA-256が変化した場合はunhealthyとしてruntimeを拒否する。commit前にpayloadを通常利用でread/execute onlyへし、mutableなtool config/cache/global packageはtyped storageへ分離してpayload完全性と混同しない。permission変更だけを完全な防御とみなさない。

## 6. permissionとmode

Windowsは現在user所有かつ他user書込み不可のACL、Linuxは現在UID所有かつgroup/other書込み不可を基本とする。root、Administrator、SYSTEM所有の既存rootを一般userが黙って採用しない。shared multi-user配置、HKLM、system PATH、system tool削除、自動昇格を実装しない。

state、receipt、cache metadata、setup backupは利用者だけが書けるようにする。tool payload/storageが他user/world書込み可能ならruntimeを拒否または診断し、警告だけで実行しない。Windows reparse pointとLinux symlinkを各open/move/delete境界で再検査する。

## 7. process実行

install engineが起動できる外部processはstaging payloadのdefinition-declared validation probeだけ。helper、hook、backend、shell scriptを実行しない。runtimeはreceiptのabsolute targetだけを起動する。

- executableとargvを分離し、shellへ再結合しない。
- install/probeはsanitized allowlist環境、runtimeは親環境＋receipt profileを使う。
- sanitized環境へは、**OSが起動に要求する最小変数だけ**をprocess adapterが補う。補う変数はWindowsの`SystemRoot`だけとし、Linuxでは何も補わない。呼出し側が同名（Windowsはcase非依存）を宣言していればその値を優先する。これ以外の変数を親環境から引き継がない。この集合は固定であり、Planと[11-quality-and-ci.md](11-quality-and-ci.md)§7.2の記録照合はこの差分を既知として扱う。
- cwd、stdin/stdout/stderr、timeout、cancel、process tree終了を明示する。
- executable containmentと完全versionを実行直前に再検査する。
- install/probeでcaptureするstdout/stderrを組込み上限で打ち切り、secretをmaskする。shim経由の直接stdio透過はgdtvmが内容を保存・maskせず、利用者processへそのまま渡す。
- Plan `probes[]`にないexternal executableをExecute中に発見して起動しない。

## 8. shell/PATH

shell profileは対象1 file・gdtvm markerだけを変更し、literalをshell別にescapeする。利用者fileをsource/evaluateして編集判断しない。before digest、owner、symlink、同時変更を検査し、backup→temp→atomic replaceを行う。

Windows user PATHはWin32 Registry APIでHKCUの既存Path値1件だけを扱う。raw値と型を保持し、長さ上限、before digest、再読検証、rollbackを行う。`setx`、HKLM、system PATHを使わない。removeはgdtvm所有entryだけを除き、利用者の後続変更を古い全値で上書きしない。

PATH/profileへ追加するのはshim directory 1件だけ。個別tool/version path、credential、download URLを入れない。

## 9. `doctor --report` の内容と除去規則

`doctor --report <path>`はドッグフーディングの一次資料である。利用者がそのままissueや会話へ貼れることを前提とし、貼る前に目視確認できるMarkdown 1 fileとする。出力先fileが既に存在する場合は上書き前に確認する。

### 9.1 含める内容

| 節 | 内容 |
|---|---|
| client | version、commit、build time、Go version、OS、arch、mode、development bool |
| root | data root / distribution rootの`path_role`（[04-storage-and-data.md](04-storage-and-data.md)§17.2）。**home配下のpathは`<HOME>/…`へ置換**。置換はroleごとに行い、文字列一致に依存しない |
| registry | schema version、tool ID一覧、各definitionのSHA-256 |
| installed | tool / version / platform / health / install時刻 / receipt SHA-256 / disk size |
| selection | `source`（`project`/`user`/`none`）、tool、version、project fileのlogical path |
| config | 有効値。存在しないkeyは既定値であることを示す |
| diagnostics | `doctor`の全診断項目とseverity、message ID、parameters |
| PATH | **全体を出さず、shim directoryが含まれるかのbooleanだけ** |
| 直近のerror | error code、message ID、parameters、発生時刻 |
| 直近log | 末尾N行。既定100行、上限は出力全体で1 MiB |
| 失敗した外部process | executableのlogical path、argv要約、exit code、stderr末尾（mask後・上限付き） |

### 9.2 除去する内容

- `*_TOKEN`, `*_PASSWORD`, `*_SECRET`, `*_KEY`にmatchする環境値
- URL userinfo、既知のtoken query key、HTTP authorization/cookie/proxy header
- 環境変数の全量dump（definitionが宣言したkeyの有無だけを示す）
- registry PATH値のraw全体、config/project fileのraw content
- user home、user名、hostname、SIDを含むabsolute path

expected/actual digestはsecretではないため記録する。report生成時のmask漏れを防ぐnegative testを必須とし、maskを解除するoptionを作らない。

### 9.3 生成の制約

report生成はread-onlyとする。payload、storage、selection、config、setup stateを変更しない。生成中に読めなかった項目は「読めなかった理由」を書き、推測値で埋めない。

## 10. network

- HTTPS必須。TLS verification disable optionなし。
- Go標準proxy環境動作とOS trust storeを使う。
- URL userinfo、HTTP Authorization/Cookieのdefinition指定を禁止する。
- redirectごとにscheme/host/credentialを検査し、wildcard hostを使わない。
- connect/header/overall timeout、redirect/page/body/download上限を適用する。
- 429/5xx/一時networkだけ有限retry。checksum/schema/404/security errorをretryしない。
- download cacheはURL identityとdigestが一致するcomplete fileだけを再利用し、partial fileのRange再開は行わない（[15-deferred.md](15-deferred.md) D-24）。

proxy環境変数にcredentialが含まれてもPlan/log/error/reportへ出さない。独自proxy credential保存、interactive login、certificate installはv0.1対象外。

client bootstrapの組込みGitHub host集合は`api.github.com`, `github.com`, `release-assets.githubusercontent.com`だけ。tool artifactはdefinitionの元host＋exact `redirect_hosts`だけ。host集合の変更はlive smoke失敗として調査し、redirect responseの任意hostをその場で信頼しない。

## 11. config・project・data

config/state/definition parserはunknown key、duplicate、型違い、enum外、上限超過を拒否し、暗黙fallbackしない。project fileはversion selectionだけで、registry、URL、hook、command、path rootを指定できない。

managed tool設定はtyped storageへ隔離し、上流標準command/fileで変更する。gdtvmは設定内容をlog/receipt/reportへcopyしない。project dependencyやsource codeを自動走査・変更しない。

## 12. logと診断

専用audit logは作らず、通常structured log、receipt、resultを使う。通常logの既定levelはinfo、rotation/保持上限を適用する。§9.2のmask規則をlogにも同じく適用する。

log masking失敗を防ぐnegative testを必須とし、debugでもcredentialを解除表示するoptionを作らない。

## 13. failure方針

次は常にfail closedとする。

- client/registry/checksum/receipt identity不一致
- archive/path/owner/permission/link検査失敗
- 封じ込め範囲（管理root、宣言integration対象、project file）外への書込み、Plan外のprobe process/download、Planの`inputs`変化
- rollback後の整合状態を証明できないtransaction

network unavailable、cache期限切れ、unsupported platform、必要system prerequisite欠落は対象operationを失敗させ、安全な再実行方法を示す。security errorを初心者向けの利便性のためにwarningへ格下げしない。

## 14. security test

最低限、次をCI matrixの両OSで自動検証する。

- traversal、ADS、reserved name、case collision、archive bomb、link race
- checksum欠落/重複/mismatch、algorithm不一致、hex長不一致、SHA-512 artifactをSHA-256として照合しようとする不正definition
- `license_notice`宣言platformで明示承認なしにExecuteへ進めないこと
- registry digest改変、receipt/payload改変
- shell marker/registry PATH同時変更、長さ、型、rollback、remove ownership
- argv分離、environment masking、redirect/credential漏えい、timeout/cancel
- junction/symlink/reparse root逸脱、他user/world-writable root
- `doctor --report`のmask漏れ（secretを含む環境・log・error fixtureで検証）

実行できないOS固有testは未実施理由と再現commandを進捗台帳へ残し、該当gateを完了にしない。
