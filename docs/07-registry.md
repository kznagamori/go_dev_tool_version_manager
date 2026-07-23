# 標準定義レジストリ仕様

## 1. 配置

標準定義は次のGitHubリポジトリでクライアントコードと同居させる。

- Repository: `https://github.com/kznagamori/go_dev_tool_version_manager`
- Branch: orphan branch `registry`
- Release tag: `registry-v<SemVer>`

`registry` branchはクライアントソース履歴と親を共有しない。tagは同一repositoryのGit objectを指す。tag名prefixによりクライアントrelease tagと区別する。

`registry` 用のorphan branchを初めて作成する場合は、作業ツリーに未コミットの変更がないことを確認したうえで、次の順序で作成する。標準のブランチ名を使用する場合、`<空ブランチ名>` は `registry` とする。

```text
git checkout --orphan <空ブランチ名>
git rm -rf .
```

`git rm -rf .` は切り替え元の追跡対象ファイルを、新しいorphan branchのインデックスと作業ツリーから除去する操作である。未保存の作業を失わないよう、実行前に作業ツリーがcleanであることを必ず確認する。実行後は未追跡ファイルも確認し、後述のregistry構成に含まれるファイルだけを登録する。この手順はorphan branchの初回作成時だけに使用し、既存の `registry` branchを更新するときは再実行しない。

クライアントはGit実行ファイルを前提にせず、HTTPSとGitHub REST/codeload/raw endpointで取得する。GitHub API仕様の参照先は以下とする。

- Git references: `https://docs.github.com/en/rest/git/refs`
- Repository contents: `https://docs.github.com/en/rest/repos/contents`

## 2. branch構造

```text
/
├─ manifest.toml
├─ manifest.sig
├─ keys.toml
├─ schemas/
│  ├─ tool-definition-v1.json
│  ├─ registry-manifest-v1.json
│  └─ helper-definition-v1.json
├─ tools/
│  ├─ android-sdk.toml
│  └─ ...
├─ helpers/
│  ├─ seven-zip.toml
│  └─ wix.toml
├─ scripts/
│  └─ <tool-id>/...
├─ messages/
│  ├─ ja.toml
│  └─ en.toml
├─ licenses/
│  └─ LicenseRef-7zip-unRAR.txt
├─ upstream-keys/
│  └─ <provider-key-file>
└─ revoked.toml
```

schemaの規範は本仕様であり、registry内JSON SchemaはCI・editor補助とする。食い違い時は本仕様を優先し、修正版registryを発行する。

### 2.1 初期registryに登録する内容

初回公開するregistry snapshotには、旧 `anyvm_win` が対応していた全17ツールを標準定義として登録する。各tool fileは[12-standard-tools.md](12-standard-tools.md)の同名節を規範とし、[06-tool-definition-schema.md](06-tool-definition-schema.md)の完全なtool definition TOMLとして作成する。単なるtool名一覧やURL一覧では受入れない。

| registry path | 正規tool ID | 入力alias | 旧 `anyvm_win` 対応 |
|---|---|---|---|
| `tools/android-sdk.toml` | `android-sdk` | `androidsdk` | `AndroidSDKVm` |
| `tools/bazel.toml` | `bazel` | なし | `BazelVm` |
| `tools/cmake.toml` | `cmake` | なし | `CMakeVm` |
| `tools/dart.toml` | `dart` | なし | `DartVm` |
| `tools/dotnet.toml` | `dotnet` | `.net` | `dotnetVm` |
| `tools/flutter.toml` | `flutter` | なし | `FlutterVm` |
| `tools/go.toml` | `go` | なし | `GoVm` |
| `tools/gradle.toml` | `gradle` | なし | `GradleVm` |
| `tools/jdk.toml` | `jdk` | `java` | `JDKVm` |
| `tools/kotlin.toml` | `kotlin` | なし | `KotlinVm` |
| `tools/llvm.toml` | `llvm` | なし | `LLVMVm` |
| `tools/mingw.toml` | `mingw` | なし | `MinGWVm` |
| `tools/ninja.toml` | `ninja` | なし | `NinjaVm` |
| `tools/node.toml` | `node` | `nodejs` | `NodejsVm` |
| `tools/python.toml` | `python` | なし | `PythonVm` |
| `tools/rust.toml` | `rust` | なし | `RustVm` |
| `tools/winlibs.toml` | `winlibs` | なし | `WinLibsVm` |

各 `tools/<id>.toml` は、対応platformごとに次の該当情報をすべて登録する。schema上そのtoolに適用されない項目は省略できるが、値が未決定であることを理由に省略してはならない。

1. 正規ID、alias、表示名、説明、homepage、license、version scheme、default channel、manager。
2. OS、architecture、libc、variant、priority、対応可否、artifact種別、relocation、payload可変性、選択方式。
3. version discovery source、完全versionへの正規化規則、stable/prerelease/EOL分類、version sort、catalog更新方式。
4. artifactの取得元、asset選択条件、redirect許可host、file形式、size上限、checksum取得・検証規則。
5. download、digest検証、展開、配置、helper実行、backend実行からなる依存順付きinstall step。
6. 公開commandごとのlauncher、payload相対target、固定引数、codepage、interpreter、signal伝播。
7. PATH、環境変数、共有cache/home、version間で保持するdirectoryとuninstall時の扱い。
8. required/optional dependency、system prerequisite、競合tool、利用者へ表示する注意・第三者artifact警告。
9. version probe、command probe、file/directory probe、期待完全version、timeoutおよび失敗条件。
10. Windows標準ユーザーおよびLinux非rootでのinstall/use/uninstall可否と、対応外platformの明示理由。

外部programを必要とする処理はbinaryをregistryへ直接登録せず、次のhelper definitionを必須登録する。helper TOMLには完全版、platform、公式取得元、artifact名、SHA-256、展開方法、公開entrypoint、license、利用toolを記載する。

| registry path | helper ID | 利用tool | 用途 |
|---|---|---|---|
| `helpers/seven-zip.toml` | `seven-zip` | `llvm`, `mingw`, `winlibs` | 7z archiveおよびself-extracting installerの管理root内展開 |
| `helpers/wix.toml` | `wix` | `python`（Windows） | Python installerからMSIを抽出する `dark.exe` |

初期snapshotには上記に加え、次の該当fileをすべて登録する。

- `schemas/tool-definition-v1.json`, `schemas/helper-definition-v1.json`, `schemas/registry-manifest-v1.json`
- `messages/ja.toml`, `messages/en.toml`
- `keys.toml`, `revoked.toml`
- 7-Zipを収録する場合の `licenses/LicenseRef-7zip-unRAR.txt` と、helper/tool定義が要求するその他のlicense text
- 上流signatureを検証する定義がある場合、その検証に必要な `upstream-keys/<provider-key-file>`
- hookが必要な定義に限る `scripts/<tool-id>/...`。組込みstepだけで表現できるtoolにはscriptを登録しない。

`manifest.toml` と `manifest.sig` はこれらの登録内容から発行時に生成する成果物であり、手書きの初期入力ではない。`manifest.toml` の `files` には両者自身を除く全登録fileを漏れなく列挙する。17件のtool definitionまたは必須helperが欠けたsnapshotは初期registryとして発行してはならない。

tool節に「公開commandは実在するもの」等の条件付き記述がある場合でも、発行する個々のplatform recipeではcommand名、launcher、target、固定引数を有限のTOML entryとして完全列挙する。clientがpayloadを走査して未定義commandを自動公開したり、file名から環境変数や導入stepを推測したりしてはならない。発行時CIは列挙した全required command targetとprobeの実在を検査する。

## 3. manifest

`manifest.toml` はUTF-8 BOMなし、LF、canonical TOML profileで生成する。最低項目:

```toml
schema = 1
registry_version = "1.0.0"
created_at = 2026-07-22T00:00:00Z
min_client_version = "1.0.0"
tool_definition_schema = 1
helper_definition_schema = 1
key_id = "registry-root-2026"

[[files]]
path = "tools/node.toml"
size = 12345
sha256 = "<64 lowercase hex>"
role = "tool"
```

`files` はUTF-8 byte順のpath昇順、重複なしとする。`manifest.toml` と `manifest.sig` 自身はfilesへ含めない。directory、symlink、submoduleを禁止し、通常fileだけを列挙する。pathは相対POSIX形式で、`.`、`..`、空segment、backslash、control characterを禁止する。

`role` は `tool`, `helper`, `script`, `schema`, `message`, `key-set`, `revocation`, `license`, `upstream-key` のいずれかで、pathの既定directoryと一致させる。実装が知らないroleは同じschema majorでは拒否する。

`max_client_version` は任意fieldであり、上限なしの場合はfield自体を省略する。空文字を特別値として使わない。

### 3.1 canonical profile

署名対象の曖昧さを避けるため、署名対象はGit blobの `manifest.toml` raw bytesそのものとする。生成側が次を保証する。

- UTF-8 BOMなし、Unicode NFC
- LFのみ、末尾LFちょうど1つ
- key順はschemaで固定、filesはpath順
- integer/date/string表記をgeneratorで統一
- commentなし

クライアントはbytesを正規化せず、そのままEd25519検証する。TOML parseは署名成功後に行う。

## 4. 署名

- algorithmはEd25519。
- 公開鍵はクライアントへ最低1本埋め込む。
- `manifest.sig` はASCIIで `key_id:base64_signature` の1行、末尾LF任意。
- signatureはmanifest raw bytes全体に対する64-byte Ed25519 signature。
- key ID不一致、base64不正、署名不正はsnapshot全体を拒否する。
- HTTPS/TLS、GitHub branch protection、tag protectionは多層防御であり、署名の代替ではない。

`keys.toml` に次期公開鍵を載せる場合、現在信頼鍵で署名されたmanifestのfilesに含まれる必要がある。鍵rotationは、旧鍵と新鍵の重複期間を1 client release以上設け、client binaryにも新鍵を追加してからregistry signing keyを切り替える。

秘密鍵はrepositoryへ置かない。CIの保護secretまたはoffline signing環境に置く。

### 4.1 初回の信頼鍵準備からmanifest生成まで

本仕様でいう「証明書の作成」は、registry署名用Ed25519鍵pairの生成を指す。X.509証明書は使用しない。初回構築時は、次の順序を変更してはならない。

1. client source branch上で、offline環境または秘密情報を扱える管理環境を使い、registry署名専用のEd25519公開鍵と秘密鍵を生成する。既存サービスの鍵やSSH署名鍵を流用しない。
2. 秘密鍵をrepositoryの作業ツリー外へ移動し、所有者だけが読み取れる権限を設定する。復旧用backupも暗号化したrepository外の保管先へ置く。秘密鍵、そのseed、生成時の一時fileをcommit、registry、release asset、logへ含めてはならない。
3. 公開鍵に一意な `key_id` を割り当て、公開鍵と `key_id` をclientの埋込み信頼鍵一覧へ登録する。manifestの `key_id` と完全一致する値を使用する。
4. clientの署名検証testを実行し、登録した公開鍵で正しい署名を受理し、別の鍵または改変されたmanifestを拒否することを確認する。
5. 公開鍵の登録と関連testをclient source branchへcommitする。commit前に、秘密鍵および秘密情報が追跡対象にも未追跡fileにも存在しないことを確認する。
6. commit完了後、既存のregistry branchへ `git switch registry` で切り替える。registry branchがまだ存在しない場合だけ、1章に記載した `git checkout --orphan <空ブランチ名>` と `git rm -rf .` の手順で作成する。
7. registry構成fileを配置または更新してから、5章の発行手順に従って `manifest.toml` を生成し、repository外の秘密鍵で `manifest.sig` を作成する。

manifest generatorには秘密鍵fileのpathを明示的に渡す。既定pathをrepository内に設けたり、秘密鍵をregistry branchへcopyしたりしてはならない。generatorとCIは秘密鍵の内容およびpathをlogへ出力しない。

## 5. 発行手順

registry release generatorは次を順に行う。

1. 初回発行では4.1節の手順1から5までを完了する。通常の更新では、client source branchの必要な変更を先にcommitする。
2. 初回発行では1章の手順でorphan branchを作成し、既存branchの更新では `git switch registry` でそのbranchへ切り替える。
3. registry構成fileを配置または更新する。
4. 全TOMLとschemaをstrict validationする。
5. 全tool ID、alias、command、helper、dependency DAGを横断検査する。
6. source URLがHTTPS、artifact checksum方針が適合することを検査する。
7. filesのsize/SHA-256を計算しmanifestをcanonical生成する。
8. repository外に保存した秘密鍵でmanifestをEd25519署名する。
9. registry構成に含まれるfileだけを登録し、cleanなorphan branch commitを作る。
10. 保護された `registry-vX.Y.Z` tagをそのcommitへ作る。
11. tagから再取得して署名・hashを独立検証する。
12. smoke clientで4 client platformのparse/plan testを行う。

公開済みtagを移動・上書きしてはならない。修正は新SemVerを発行する。

## 6. client bootstrap/update

### 6.1 最新版探索

1. GitHub APIで `refs/tags/registry-v` を列挙する。
2. prefix以降が正しいSemVerであるtagだけを採用する。
3. client互換情報は取得後manifestで最終判断するため、候補をversion降順に試す。
4. annotated tagは循環とdepth上限を検査しながらtag objectをcommitへ解決し、lightweight tagは直接commitを使う。候補はtag名と不変なcommit SHAの組として固定する。
5. tag名だけでbranch先端を信頼しない。

API rate limit、proxy、offlineに備え、最後に成功したtag候補をcacheする。`GITHUB_TOKEN` 等の設定済み環境変数があればAuthorization headerへ使うが、ログには出さない。

active registry版がある場合、自動更新とversion省略updateはそれ未満の候補を採用しない。API/cacheが新しいtagを返さないだけで旧snapshotへ戻してはならない。利用者が`--version`で古い完全版を明示したrollbackだけを警告・承認・audit後に許すが、signature/manifest/client互換検証は省略しない。

### 6.2 downloadと検証

1. 候補tagで解決済みのcommit SHAを使い、そのcommitのmanifestとsignatureを小容量で取得する。以後の全取得を同じcommit SHAへpinし、可変tag名をdownload対象に使わない。
2. embedded keyでsignature検証する。
3. manifestをstrict parseし、client/schema互換を検査する。
4. 同じcommit SHAのarchiveを `cache/registry/*.part` へ取得する。
5. GitHub archive固有の単一top-level directoryを検証して除き、archiveを安全に列挙する。`manifest.toml` と `manifest.sig` 以外のmanifest外file、symlink、path traversal、重複path、case衝突を拒否する。
6. archive内のmanifest/signatureが先に検証したraw bytesと一致すること、および各列挙fileのsizeとSHA-256がmanifestと一致することを照合する。
7. `registry/snapshots/<version>.staging-<id>` へ展開する。
8. snapshot metadataを追加し、完成名へrenameする。
9. `state/registry.toml` のactive versionをatomic更新する。
10. 前snapshotをrollback候補として保持する。

archive取得を使わずfilesを個別取得してもよいが、同じcommit SHAへpinし、すべてのhashを照合する。tagが探索後に移動・削除されても、固定したcommitが取得できる限りoperationの入力を変更しない。commit取得不能なら別候補へ移り、branch先端へfallbackしない。

### 6.3 初回

標準snapshotがなく、`auto_bootstrap=true` かつオンラインなら、通常コマンド実行前に上記bootstrapを行う。利用者には「署名済み標準定義を取得しています」とsource repositoryを表示するが、公式registryの通常取得に追加確認は不要。offlineまたは取得失敗なら、ツール処理を行わず `registry update` の案内を出す。

### 6.4 自動更新

`auto_update=true` でも通常コマンドのcritical pathを長く止めない。interval超過時に更新確認し、短いmetadata requestで最新版があれば、その操作開始前に同期更新するか、設定されたtimeout内に完了しなければ現snapshotで継続し次回案内する。install計画開始後にdefinitionを差し替えない。

## 7. SemVer方針

- PATCH: URL、checksum、version filter、message、同じ意味のstep修正。
- MINOR: 後方互換なtool/helper追加、platform追加、optional field利用。
- MAJOR: schema major、既存tool ID意味、信頼方針、必須client能力の破壊変更。

artifactの緊急revocationはPATCHを発行し `revoked.toml` へinstall key/digest/reason/advisoryを追加する。clientはregistry update後、導入済みreceiptと照合してdoctor warning/errorを出す。利用者のtoolを黙って削除しない。

## 8. rollback

新snapshotを有効化後、definition parseやruntime問題が判明した場合に `repair` は前の検証済みsnapshotへ戻す計画を提示できる。明示的な公開CLI `registry rollback` は初期版に設けず、概念増加を避ける。

rollbackしても既に作成したreceiptは元definition hashを保持する。新旧definitionが同じinstallを解釈できない場合は、導入済みpayloadを維持して管理操作だけを停止し、registry更新を案内する。

## 9. branch protection

repository側で次を必須とする。

- `registry` branchへのforce-push/delete禁止
- pull requestとCI validation必須
- maintainer review必須
- `registry-v*` tagの作成権限制限と更新/delete禁止
- signing workflowへのsecret accessをprotected environmentで制限

ただしclient trust rootはembedded Ed25519 keyであり、GitHub account compromiseだけでは正しいsignatureを作れない構成とする。

## 10. offline

- active検証済みsnapshotがあれば、registry通信なしで全ローカル操作を行う。
- catalog cacheとartifact download cacheが揃えばoffline installを許可する。
- offline中はsignature期限を設けないが、snapshot ageとrevocation未取得のriskをwarning表示できる。
- `registry update --offline` 相当はusage errorではなく、明確なoffline network prohibited errorとする。
- cacheへ手動配置したarchiveはmanifest digest一致時だけ使う。

## 11. mirror/enterprise override

global設定でrepositoryを変更できるが、標準embedded keyはそのまま使う。別署名鍵を追加する機能は初期版では提供せず、独自repositoryはローカル定義として扱う。これにより初心者がURL変更だけで新しいtrust rootを暗黙導入することを防ぐ。

## 12. 監査情報

stateへactive tag、commit SHA、manifest SHA-256、signature key ID、verified time、source URL、previous versionを保存する。`gdtvm version` と `doctor --deep` から確認可能にする。
