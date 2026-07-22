# 標準定義レジストリ仕様

## 1. 配置

標準定義は次のGitHubリポジトリでクライアントコードと同居させる。

- Repository: `https://github.com/kznagamori/go_dev_tool_version_manager`
- Branch: orphan branch `registry`
- Release tag: `registry-v<SemVer>`

`registry` branchはクライアントソース履歴と親を共有しない。tagは同一repositoryのGit objectを指す。tag名prefixによりクライアントrelease tagと区別する。

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

## 5. 発行手順

registry release generatorは次を順に行う。

1. 全TOMLとschemaをstrict validationする。
2. 全tool ID、alias、command、helper、dependency DAGを横断検査する。
3. source URLがHTTPS、artifact checksum方針が適合することを検査する。
4. filesのsize/SHA-256を計算しmanifestをcanonical生成する。
5. manifestをEd25519署名する。
6. cleanなorphan branch commitを作る。
7. 保護された `registry-vX.Y.Z` tagをそのcommitへ作る。
8. tagから再取得して署名・hashを独立検証する。
9. smoke clientで4 client platformのparse/plan testを行う。

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
