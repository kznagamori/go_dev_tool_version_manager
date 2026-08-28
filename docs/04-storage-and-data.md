# 保存先・状態・データ契約仕様

## 1. modeとroot決定

modeは`portable|user`だけを許す。優先順位は、CLIの一時`--mode`、有効なsetup state、導入経路の既定の順とする。global `gdtvm.toml`にmode keyはない。

- 手動archiveの既定は`portable`。
- `install.ps1`/`install.sh`の既定は`user`。
- mode変更で既存tool/stateを自動移動・削除しない。新rootをsetupし、旧rootを明示する。
- `portable`と`--home`は排他。`user --home`はその実行だけdata rootを上書きし、永続shell/PATH変更には使わない。

### 1.1 portable

`gdtvm[.exe]`が存在するcanonical directoryをdistribution root兼data rootとする。filesystem root、symlink/reparse loop、owner不明、現在userが書けないrootをsetup先として拒否する。

```text
<portable-root>/
  gdtvm[.exe]
  gdtvm.toml             # 任意
  registry/
  README.md
  USER_GUIDE.md
  LICENSE
  tools/
  state/
  cache/
  logs/
  tmp/
  locks/
  shims/
```

setup後にこのdirectoryを移動する運用はv0.1で非対応とする。移動した場合はlink、shim、shell/PATH integrationが旧pathを指すため、`gdtvm setup`を再実行して作り直す。`doctor`はroot不一致を検出して再setupを案内する。

### 1.2 user mode

- Windows: Known Folder APIの`LocalAppData`直下`gdtvm`。
- Linux: OS user lookup APIで得たhome直下`.local/share/gdtvm`。

Linuxは通常のpath決定に`HOME`、`XDG_DATA_HOME`、`XDG_STATE_HOME`、`XDG_CACHE_HOME`を使わない。ownerが現在userと一致し、既存親chainに他user/world書込み可能な不安全要素がないことを検査する。

bootstrapによるactive distribution rootは`<data-root>/distribution/current`とする。client/registry/任意configはdistribution root、tool/state/cacheはdata rootへ置く。

`--home`は既存または現在userが作成可能なabsolute directoryだけを許す。filesystem root、distribution rootそのもの、他user所有、network share、symlink/reparse loopを拒否する。永続PATH/profileは既定data rootを指すため、`--home`実行のshimを永続作成しない。

## 2. data root layout

```text
<data-root>/
  distribution/                 # user modeだけ
  tools/
    <tool-id>/
      versions/
        <exact-version>/
          <platform-id>/
            payload/
            version-data/
            .gdtvm-install.toml
      shared/
        <storage-id>/
      current                    # managed user selectionだけ
  state/
    schema.toml
    selections.toml
    setup.toml
    shim-index.toml
    receipt-index.toml
    shell/
    backups/
  cache/
    catalogs/
    downloads/
  logs/
  tmp/
    operations/<operation-id>/
    trash/<operation-id>/
  locks/
  shims/
```

`platform-id`は`windows-amd64|linux-amd64-glibc`。versionとpath componentは定義のgrammarを通し、raw upstream文字列をdirectory名に使わない。

`tmp/operations/<operation-id>/`配下には当該operationが作成したものしか存在しない。中断した操作の後始末は、root ID・owner・作成時刻を検査したうえでこのdirectoryをまとめて削除する。

## 3. 編集可能範囲

| 対象 | 利用者編集 |
|---|---|
| distribution隣接`gdtvm.toml` | 任意。strict schema |
| project `.gdtvm.toml` | 可。strict schema |
| toolの公式設定file | tool definitionが宣言したstorage内で可 |
| registry、receipt、state、catalog、index | 不可。gdtvmだけがatomic更新 |
| typed storage内のcache/global bin/package | 対応toolの公式commandから変更可 |
| payload | 不可。通常permissionでread/execute only |

内部TOML/JSONのunknown key、重複、型違い、上限超過は破損として扱い、黙って修復・再生成しない。index/cacheだけは正本receiptから安全に再構築できる。

## 4. atomic write

state更新は同一volumeで次を守る。

1. 対象lock取得。
2. 現在revision/digestを再確認し、revision fieldを持つfileはnext=current+1（新規は1）を計算する。
3. next revisionを含む全内容をsibling temporary fileへ書き、flushする。
4. strict再parseする。
5. 正本stateの既存fileがあれば同じdirectoryの`<basename>.bak`へ最新1世代をatomic保持してから、temporaryをatomic replaceする。
6. directory metadataを可能なOSでflushする。
7. 公開fileを再読してexpected digest/revisionと一致させる。不一致なら検証済みbackupへrollbackする。

Windowsでreplace APIの制約がある場合も、旧fileを失った状態で新fileを書き始めない。`.bak`は元fileと同じowner-only permission、raw schema、root IDを保ち、strict parse/digest/root IDが一致する場合だけ復元候補にする。receipt、catalog、再構築可能indexはこのgeneric backup対象外。setupのOS integration raw backupは§10の別契約を使う。

正本stateは`state/schema.toml`, `state/selections.toml`, `state/setup.toml`の3件とする。

## 5. lockとcleanup

lock順序の正本は[02-architecture.md](02-architecture.md)§12とする。本章は保存先とmetadata形式（§19）だけを規定し、順序をここで再掲しない。

- catalog cacheはdefinition hash、platform ID、取得時刻、期限を持つ。
- 中断したdownloadのpartial fileは再利用せず、所有と作成時刻を検査して破棄する。download再開は[15-deferred.md](15-deferred.md) D-24へ延期する。
- install成功後も共有content cacheは上限内で保持できる。
- failed/cancelled operationのtmpはroot ID、owner、作成時刻を検査してcleanする。
- cleanup失敗は完成installをrollbackせず`W_CLEANUP_INCOMPLETE`にし、`doctor`へ残す。

## 6. root境界

すべての書込み前にlogical rootからabsolute pathを組み立て、canonical parent containmentを検査する。absolute component、`..`、空component、予約名、ADS、NUL、separator混在、symlink/reparse point経由の逸脱を拒否する。削除はreceiptまたはsetup stateで所有を証明できるregular file/directory/linkだけを対象にする。

## 7. 共通表現

schema revisionはすべて`1`。TOMLはUTF-8 BOMなしTOML 1.0、JSONはUTF-8 BOMなしRFC 8259。永続fileは末尾LFちょうど1つ。unknown/duplicate key、型違い、enum外、上限超過、trailing dataを拒否する。

| 型 | 表現 |
|---|---|
| timestamp | UTC RFC 3339、秒精度以上、offset `Z` |
| SHA-256 | 64 lowercase hex |
| upstream digest | `<algorithm>:<lowercase hex>`。algorithmは`sha256`（64 hex）または`sha512`（128 hex） |
| byte count/revision | 非負integer、JSONは2^53-1以下 |
| operation/install/root ID | 128 bit randomの32 lowercase hex |
| duration | config/definitionはGo duration文字列、機械resultはmillisecond integer |
| path | TOML/JSONではlogical role＋POSIX relativeを基本。absoluteは契約が明記したfieldだけ |
| URL | HTTPS、userinfoなし、最大8 KiB |
| enum/ID | ASCII lowercase、各章のgrammar |
| scalar parameter key | `^[a-z][a-z0-9_]*$`、1～64文字 |
| message ID | `^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$`、segment 2件以上、全体1～128文字 |

scalar parameter keyは、typed errorの`parameters`（[02-architecture.md](02-architecture.md)§14）、progressの`parameters`（同§10）、Plan warningとresult warningの`parameters`（§16.1・§16.2）、structured logの`fields`（§18）で共通とする。同じ値を指すkeyが箇所ごとに`tool_id`と`toolId`へ分かれると、message templateのplaceholder（§20）とCLI JSONの突き合わせができなくなるためである。permissiveな受理はせず、grammar外のkeyを持つ値は公開境界へ出す前に拒否する。

message IDのsegmentを2件以上とするのは、先頭segmentを`error`、`warning`、`install`のような分類として使い、catalogの網羅性をtoolで検査できるようにするためである。

digestは2種類あり、混同しない。

| 種別 | 表現 | 該当field |
|---|---|---|
| **upstream由来**（providerが公開した値） | `<algorithm>:<hex>` | definitionのchecksum/asset digest、catalogの`artifact_digest`、receiptの`[artifact].digest`、Planの`downloads[].expected_digest` |
| **gdtvm自身が計算** | 64 lowercase hex（SHA-256固定） | receiptの`command_targets[].sha256`、`receipt-index.toml`の`receipt_sha256`、setup backupの`sha256`と`integration_identity`、`registry.toml`のtool entry `sha256`、Planの`inputs`各digest、release `checksums.txt` |

gdtvm自身が計算するdigestにalgorithmを選ぶ理由がないため、SHA-256固定とし`<algorithm>:`を付けない。upstream digestのalgorithmは`<algorithm>:`部分だけを正本とし、別fieldへ複製しない。algorithmとhex長が一致しない値を拒否する。

mapを永続化するときはkey UTF-8 byte順、tool/versionは定義したcomparison＋ID順、pathはPOSIX relative UTF-8 byte順で出力する。parserは順序に意味を持たせない。

## 8. `state/schema.toml`

```toml
schema = 1
revision = 1
root_id = "0123456789abcdef0123456789abcdef"
mode = "user"
created_at = 2026-08-07T09:00:00Z
updated_at = 2026-08-07T09:00:00Z
client_version = "2026.08.07.00"
state_schema = 1
receipt_schema = 1
catalog_schema = 1
```

許可keyは上記だけで全件必須。mode=`portable|user`。root IDは作成後不変。別root IDのstate、junction/symlink、receiptを混在させない。development buildはclient_version=`devel`を許す。

## 9. `state/setup.toml`

```toml
schema = 1
revision = 3
root_id = "0123456789abcdef0123456789abcdef"
mode = "user"
path_integration = "user-path"
shell = ""
shim_path = "shims"
backup_id = "abcdef0123456789abcdef0123456789"
updated_at = 2026-08-07T09:00:00Z

[integration_identity]
kind = "windows-registry-value"
location = "HKCU\\Environment"
name = "Path"
before_sha256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
after_sha256 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
```

top-levelは`schema`, `revision`, `root_id`, `mode`, `path_integration`, `shell`, `shim_path`, `backup_id`, `updated_at`, `integration_identity`。全件必須。path integration=`user-path|shell-profile|none`。shellはnone/user-pathなら空、shell-profileなら`bash|zsh|fish`。shim pathはdata root相対`shims`固定。

integration identity keyは`kind`, `location`, `name`, `before_sha256`, `after_sha256`。kind=`windows-registry-value|shell-profile-file|none`。noneではkind=`none`、他string空、digestは64 zero。registry/profileのraw内容をsetup stateへ入れない。

## 10. setup backup

`state/backups/setup-<backup-id>.toml`はlatest 1世代だけ保持しowner-onlyとする。

```toml
schema = 1
backup_id = "abcdef0123456789abcdef0123456789"
root_id = "0123456789abcdef0123456789abcdef"
kind = "windows-user-path"
created_at = 2026-08-07T09:00:00Z
target = "HKCU\\Environment\\Path"
existed = true
value_type = "REG_EXPAND_SZ"
raw_bytes_base64 = "VABFAFMAVAA="
sha256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
```

許可keyは上記だけ。kind=`windows-user-path|shell-profile`。Windows raw bytesはregistry valueのUTF-16LEそのもの、profileはfile raw bytes。不存在は`existed=false`, raw空、digest 64 zero。Base64 decode後のsize上限を適用し、log/JSON/reportへrawを出さない。backupは即時rollback/diagnose用で、remove時に利用者の後続変更全体を巻き戻す用途にしない。

## 11. `state/selections.toml`

```toml
schema = 1
revision = 8
root_id = "0123456789abcdef0123456789abcdef"
updated_at = 2026-08-07T09:00:00Z

[[selections]]
tool_id = "node"
version = "22.18.0"
platform_id = "windows-amd64"
install_id = "11111111111111111111111111111111"
selected_at = 2026-08-07T09:00:00Z
```

top-level keyは`schema`, `revision`, `root_id`, `updated_at`, `selections`。selection entryの許可keyは`tool_id`, `version`, `platform_id`, `install_id`, `selected_at`で全件必須。toolごとに最大1件、tool ID byte順で一意。

v0.1のuser selectionはmanaged版だけを表す。selectionはreceiptと`install_id`で一致させ、receiptが欠落・破損していればinactiveとして扱い、fileを自動更新しない。

## 12. `state/shim-index.toml`

```toml
schema = 1
revision = 4
root_id = "0123456789abcdef0123456789abcdef"
client_version = "2026.08.07.00"
receipt_index_revision = 5
updated_at = 2026-08-07T09:00:00Z

[[commands]]
name = "node"
tool_id = "node"

[[commands]]
name = "npm"
tool_id = "node"
```

許可keyは上記だけ。commandsはname byte順で一意。`tool_id`は正規IDで、registry全体のrequired command衝突検査によりcommandごとにexactly 1 toolへ対応する。version targetを固定せず、runtimeがproject/user selectionとreceiptから解決する。receipt revision不一致時は正本receiptから再生成できるが、未知commandをPATH探索しない。

## 13. `state/receipt-index.toml`

```toml
schema = 1
revision = 5
root_id = "0123456789abcdef0123456789abcdef"
updated_at = 2026-08-07T09:00:00Z

[[receipts]]
tool_id = "node"
version = "22.18.0"
platform_id = "windows-amd64"
install_id = "11111111111111111111111111111111"
path = "tools/node/versions/22.18.0/windows-amd64/.gdtvm-install.toml"
receipt_sha256 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
health = "healthy"
```

許可keyは上記だけ。health=`healthy|unhealthy|unknown`。tupleで一意・sort。indexはcacheでありreceipt走査から再構築できるが、破損receiptを健康扱いしない。

## 14. install receipt

保存pathは`.gdtvm-install.toml`。

```toml
schema = 1
install_id = "11111111111111111111111111111111"
root_id = "0123456789abcdef0123456789abcdef"
tool_id = "node"
version = "22.18.0"
platform_id = "windows-amd64"
installed_at = 2026-08-07T09:00:00Z
client_version = "2026.08.07.00"
client_commit = "0123456789abcdef0123456789abcdef01234567"
definition_path = "tools/node.toml"
definition_sha256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
payload_path = "payload"

[artifact]
provider_kind = "official"
provider_name = "Node.js project"
provider_release = "v22.18.0"
url = "https://nodejs.org/dist/v22.18.0/node-v22.18.0-win-x64.zip"
file = "node-v22.18.0-win-x64.zip"
size = 1
digest = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
checksum_source = "text-file"
third_party_approved = false
license_notice_approved = false

[[storage]]
id = "global-packages"
kind = "global-packages"
scope = "version"
path = "version-data/global-packages"
purge = "with-version"

[[commands]]
name = "node"
target = "{{payload}}/node.exe"
fixed_args = []
environment_profile = "default"
working_directory = "inherit"
passthrough_signals = true

[[environment_profiles]]
id = "default"
path_prepend = ["{{storage.global-packages}}"]
path_append = []
unset = []
override_allowed = []
shell_export = ["NPM_CONFIG_PREFIX"]

[environment_profiles.set]
NPM_CONFIG_PREFIX = "{{storage.global-packages}}"

[[probes]]
id = "version"
runtime_command = "node"
args = ["--version"]
stream = "stdout"
expect = "version"
regex = "^v(?P<version>[0-9]+[.][0-9]+[.][0-9]+)$"
expected_version = "22.18.0"
expected_root = ""
required_paths = []
timeout_ms = 30000
required = true
status = "passed"
reported_version = "22.18.0"
finished_at = 2026-08-07T09:00:00Z

[[command_targets]]
path = "payload/node.exe"
size = 1
sha256 = "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
```

top-level keyは`schema`, `install_id`, `root_id`, `tool_id`, `version`, `platform_id`, `installed_at`, `client_version`, `client_commit`, `definition_path`, `definition_sha256`, `payload_path`, `artifact`, `storage`, `commands`, `environment_profiles`, `probes`, `command_targets`。全件必須、arrayはstorageだけ空可。payload path=`payload`固定。

`command_targets`は**required runtime commandのtargetとfixed argsが参照するpayload内fileだけ**を、payload相対path byte順・一意で持つ。payload全fileのmanifestは作らない。keyは`path`, `size`, `sha256`。`doctor`はここを照合してpayload破損を検出する。

artifact keyは`provider_kind`, `provider_name`, `provider_release`, `url`, `file`, `size`, `digest`, `checksum_source`, `third_party_approved`, `license_notice_approved`。provider kind=`official|third-party`。`digest`は§7のupstream digest形式。v0.1は全artifactでupstream checksumとの一致を必須とするため、検証状態fieldを持たない。third-partyなら`third_party_approved=true`必須、definitionが`license_notice`を持つplatformなら`license_notice_approved=true`必須。

storage keyは`id`, `kind`, `scope`, `path`, `purge`。tool scope pathはreceiptからdata rootの`tools/<tool>/shared/<id>`へ解決しpurge=`retain|explicit`、version scopeはreceipt directoryの`version-data/<id>`へ解決しpurge=`with-version`固定。receiptにabsolute rootを書かない。

command keyは`name`, `target`, `fixed_args`, `environment_profile`, `working_directory`, `passthrough_signals`。targetはpayload regular executableまたは別required command実体で、fixed JS/module argsを許す。working directory=`inherit|payload`。profile keyは`id`, `path_prepend`, `path_append`, `set`, `unset`, `override_allowed`, `shell_export`。env map keyはplatform規則で一意。target/fixed args/path/setで許すtemplateは`{{payload}}`とreceipt内に存在する`{{storage.<id>}}`およびその子pathだけで、metadata/version/staging/outputや再帰展開は禁止する。runtimeはreceiptの現在位置とstorage scopeからabsolute化し、管理rootのabsolute pathをreceiptへ固定しない。

probe keyは`id`, `runtime_command`, `args`, `stream`, `expect`, `regex`, `expected_version`, `expected_root`, `required_paths`, `timeout_ms`, `required`, `status`, `reported_version`, `finished_at`。全key必須で非該当string/arrayは空。stream/expect/regex/root/pathの意味は[06-tool-definition.md](06-tool-definition.md)、timeoutは1～120,000 ms。status=`passed|skipped`でrequired=trueはpassed必須。version非対象probeはreported version空。

既存receiptはactive registry definitionで再解釈しない。runtimeはreceiptのcommand/profile/storageを正とし、registryはschema互換だけを追加判断する。

## 15. catalog JSON

保存pathは`cache/catalogs/<tool-id>/<platform-id>.json`。

```json
{
  "schema": 1,
  "tool_id": "node",
  "platform_id": "windows-amd64",
  "definition_sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "source_identity": "https://nodejs.org/dist/index.json",
  "fetched_at": "2026-08-07T09:00:00Z",
  "expires_at": "2026-08-08T09:00:00Z",
  "items": [
    {
      "version": "22.18.0",
      "channel": "stable",
      "lifecycle": "supported",
      "lifecycle_evidence": "https://github.com/nodejs/Release",
      "lifecycle_assessed_at": "2026-08-07T00:00:00Z",
      "published_at": "2026-07-01T00:00:00Z",
      "installable": true,
      "unavailable_reason": "",
      "provider_kind": "official",
      "provider_release": "v22.18.0",
      "artifact_file": "node-v22.18.0-win-x64.zip",
      "artifact_url": "https://nodejs.org/dist/v22.18.0/node-v22.18.0-win-x64.zip",
      "artifact_size": 1,
      "artifact_digest": "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
      "checksum_source": "text-file"
    }
  ]
}
```

top-level/entry keyは例の集合だけ。static sourceは`expires_at=null`を許す。channel=`stable|prerelease`、lifecycle=`supported|eol|unknown`。lifecycle evidenceは公式/providerのHTTPS URL、assessmentはUTC RFC 3339で全状態に必須。source fieldならsource URL/fetch時刻、override/staticならdefinition記録を使う。**上流がlifecycleを示さず既定の`unknown`になったitemもsource URL/fetch時刻を使う。**「この公式sourceをこの時刻に取得した時点でlifecycle情報が公開されていなかった」ことを記録する。取得元を持たない根拠不明のitemを作らない。`published_at`はUTC RFC 3339とし、providerがitem公開日時を提供せずdefinitionもpointerを宣言しない場合だけ空文字を許す。catalog取得時刻で代用しない。`json-index` sourceの`source_identity`はindex文書のURLとする。unavailable reasonはinstallable=trueなら空、falseならmessage ID。`artifact_size=0`はprovider上でunknown、正値はexpected size。`artifact_digest`は§7のupstream digest形式で、Plan前に未解決のitemは`installable=true`にしない。itemsはversion comparison降順、同値ならversion byte順。

catalogはcacheでありdefinition/platform不一致時に利用しない。offline exactはidentity/digestが完全なら期限切れを`W_CACHE_STALE`付きで利用できる。`--latest`は期限切れを黙って使わない。

## 16. Plan data

Planは永続fileではないがhuman表示とapprovalの正本となるtyped data。

```json
{
  "schema": 1,
  "client_version": "2026.08.07.00",
  "invocation_id": "33333333333333333333333333333333",
  "operation_id": "22222222222222222222222222222222",
  "operation": "install",
  "created_at": "2026-08-07T09:00:00Z",
  "summary": {
    "tool_id": "python",
    "version": "3.13.7",
    "platform_id": "windows-amd64",
    "provider_kind": "third-party",
    "provider_name": "Astral",
    "provider_repository": "https://github.com/astral-sh/python-build-standalone",
    "provider_homepage": "https://github.com/astral-sh/python-build-standalone",
    "provider_license": "MPL-2.0",
    "provider_release": "20250814",
    "license_notice": "",
    "channel": "stable",
    "lifecycle": "supported",
    "expected_digest": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    "checksum_source": "asset-field",
    "warning_count": 1
  },
  "setup": null,
  "inputs": {
    "root_id": "0123456789abcdef0123456789abcdef",
    "config_sha256": "",
    "project_sha256": "",
    "definition_sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    "catalog_sha256": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
    "registry_sha256": "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
    "selections_revision": 8,
    "setup_revision": 3,
    "receipt_index_revision": 5
  },
  "downloads": [],
  "extracts": [],
  "probes": [],
  "writes": [],
  "storage": [],
  "warnings": [
    {
      "code": "W_THIRD_PARTY",
      "message_id": "warning.third_party",
      "parameters": {},
      "requires_explicit_approval": true
    }
  ]
}
```

上のJSONはtop-level、summary、setup、inputs、配列のkey形状を示す**構造例**であり、記載量を抑えるためoperation entryを空にしている。そのままでは`operation=install`に必要なdownload/extract/probeがないため、semantic validatorは実行可能なPlanとして拒否しなければならない。実行可能なpositive fixtureは各definitionの全probeと全作用を展開し、[11-quality-and-ci.md](11-quality-and-ci.md)§6および[13-progress.md](13-progress.md) P6-02で検査する。

top-level keyは例の集合だけで全件必須。`client_version`はPlanを作ったclientの完全versionまたは`devel`、`invocation_id`はrequest、`operation_id`は変更transactionの128 bit lowercase hex IDとする。`operation`は`setup|setup-remove|install|use|uninstall`。`summary`と`inputs`のkeyも例の集合だけで全件必須とし、対象外のstringは空、数値は0とする。`provider_kind=official|third-party|none`、対象toolがないoperationのprovider/channel/lifecycle/checksum/license fieldは空または`none`とする。`warning_count`は`warnings`の件数と一致させる。catalog refresh、log rotation、検証済みtmp/cache cleanupはPlan operationに含めない。

`setup`は`operation=setup|setup-remove`のときだけ`SetupPlan` object、それ以外は`null`とする。`SetupPlan`のexact keyは次の15件で全件必須。

| key | type/規則 |
|---|---|
| `mode` | `portable\|user` |
| `previous_mode` | 初回setupまたはmode/root不変なら空、それ以外は変更前の`portable\|user` |
| `data_root` | role=`data-root`の`PathValue` |
| `distribution_root` | role=`distribution-root`の`PathValue` |
| `previous_data_root` | role=`data-root`の`PathValue`。初回setupまたはmode/root不変ならpath空 |
| `previous_distribution_root` | role=`distribution-root`の`PathValue`。初回setupまたはmode/root不変ならpath空 |
| `filesystem_capabilities` | §17.1の値をASCII byte順・重複なしで1～7件 |
| `current_link_strategy` | `junction\|symlink` |
| `shim_strategy` | `hardlink\|symlink\|fallback-resolver` |
| `shim_directory` | role=`shim`の`PathValue` |
| `path_integration` | `user-path\|shell-profile\|none` |
| `shell` | `shell-profile`なら`bash\|zsh\|fish`、それ以外は空 |
| `integration_target` | role=`config`の`PathValue`。`none`ならpath空 |
| `backup_path` | role=`state-backup`の`PathValue`。integrationを変更しない場合はpath空 |
| `restart_required` | bool。trueと`W_RESTART_REQUIRED` exactly 1件を同値にする |

Windowsはcapabilityに`atomic-replace|directory-rename|file-identity|owner-enforcement|junction`を必須とし、`current_link_strategy=junction`、`shim_strategy=hardlink|fallback-resolver`とする。hardlinkを使う場合だけcapabilityへ`hardlink`を含める。Linuxは`atomic-replace|directory-rename|file-identity|owner-enforcement|symlink`を必須とし、`current_link_strategy=symlink`、`shim_strategy=symlink|fallback-resolver`とする。必須capabilityを確認できない場合はPlanを作らず`E_PLATFORM_UNSUPPORTED`にする。

`previous_mode`、`previous_data_root`、`previous_distribution_root`は同時に空または同時に非空とする。非空と`W_MODE_CHANGE` exactly 1件を同値にし、旧rootを読書き対象へ暗黙追加しない。`setup-remove`も現在のsetup stateから同じobjectを作り、除去するintegration targetと変更前backup先を明示する。

`inputs`はroot identity、config/project/definition/catalog/registryのSHA-256、selection/setup/receipt-index revisionを固定する。ExecuteはPlanのschema/client/invocationと全inputを、対応する実体から再取得して承認前とlock取得後に再検査する。

各配列entryのexact keyは次のとおり。全key必須で、対象外のstring/digestは空、配列は空、sizeは0とする。IDはPlan内で種類をまたいで一意なASCII lowercase kebab、各配列はIDのASCII byte順とする。pathは§17.2の`PathValue`を使い、entryを作る場合は空pathを許さない。

| 配列 | exact key | enum/規則 |
|---|---|---|
| `downloads` | `id`, `provider_kind`, `provider_name`, `provider_repository`, `provider_homepage`, `provider_release`, `url`, `file_name`, `size`, `expected_digest`, `checksum_source`, `license`, `adoption_reason_message_id`, `destination` | HTTPS完全URL。`expected_digest`は§7のupstream digest形式。`size=0`はprovider上でunknownとして表示する。`destination.role=download-cache\|staging`。officialのadoption reasonだけ空 |
| `extracts` | `id`, `source_download_id`, `format`, `strip_components`, `destination` | `source_download_id`は同じPlanのdownload ID。`format=zip\|tar.gz`、stripは`0\|1`、`destination.role=staging` |
| `probes` | `id`, `runtime_command`, `executable`, `version`, `source`, `artifact_digest`, `license`, `reason_message_id`, `args`, `working_directory`, `write_paths`, `stream`, `expect`, `regex`, `expected_version`, `expected_root`, `required_paths`, `timeout_ms`, `required` | definition probeを完全展開した値。executable/cwd/write pathは`PathValue`。完全version、artifact URL/digest、provider license、理由を空にしない。`args`は下記`PlanArg[]`、`expected_root`は`PathValue\|null`、`required_paths` entryは`kind`, `path`（`PathValue`）。stream/expect/kindは§17.1。Plan外probeを実行しない |
| `writes` | `id`, `action`, `target` | `target`は`PathValue`。`action=create\|replace\|remove\|junction\|symlink\|registry-value` |
| `storage` | `id`, `kind`, `scope`, `target`, `purge`, `action` | `target`は`PathValue`。kind/scope/purgeは[06-tool-definition.md](06-tool-definition.md)。`action=create\|retain\|purge` |
| `warnings` | `code`, `message_id`, `parameters`, `requires_explicit_approval` | parametersはstring/bool/integer/nullだけのmap。codeは§16.1の閉じた集合 |

`PlanArg`のexact keyは`kind`, `value`, `path`。`kind=literal`では`value`をそのままargv 1要素とし`path=null`、`kind=path`では`value`を空、`path`を非空の`PathValue`とし、そのnative pathをargv 1要素とする。definitionの1個のargs entryを複数argvへ分割せず、pathをliteralやwarning parameterへ埋め込まない。

`writes[]`は利用者可視の変更だけを列挙する。対象は、setup/setup-removeのintegration対象（Windows user PATHの`action=registry-value`、shell profileの`create|replace|remove`）、project fileの作成・更新、current linkの`junction|symlink|remove`とする。staging、download cache、state、receipt、index、shim、storageなどdata root内部の書込みはPlanへ列挙せず、Executeの封じ込め検査（全書込みがdata root、distribution root、宣言済みintegration対象、project fileの中にあること）と[11-quality-and-ci.md](11-quality-and-ci.md)§7.2の書込み範囲検査で保証する（[15-deferred.md](15-deferred.md) D-23）。rollbackはengine内部動作でありPlan dataとして公開しない。失敗時回復は[11-quality-and-ci.md](11-quality-and-ci.md)§6のfailure injection testで検証する。

Approvalは`requires_explicit_approval=true`のwarning `code`集合そのものであり、Executeは同じPlan objectのcode集合をApprovalが満たすことを検査する。Plan全体のcanonical digestは持たない。

Executeは`inputs`の各値を実体から再取得して一致を確認する。lock取得後にも同じ確認を行い、不一致なら`E_PLAN_STALE`とする。v0.1の変更operation中に起動できる子processは列挙済みprobeだけで、任意helper/backendを起動しない。Plan外のdownload、extract、probeを実行せず、書込みを封じ込め範囲の外へ出さない。human簡略表示の都合でtyped fieldを削らない。

### 16.1 `PlanWarningCode`

v0.1の事前表示・承認用`PlanWarningCode`は次のexactly 8件とする。未定義codeをPlanへ出さない。

| code | 条件 | 明示承認 |
|---|---|---|
| `W_THIRD_PARTY` | artifact providerが`third-party` | 必要 |
| `W_RESTRICTIVE_LICENSE` | definitionが当該platformへ`license_notice`を宣言している（OSI承認OSS licenseでない配布物） | 必要 |
| `W_PRERELEASE` | 対象versionの`channel=prerelease` | 必要 |
| `W_EOL` | 対象versionの`lifecycle=eol` | 必要 |
| `W_DESTRUCTIVE` | tool-scope shared storageの削除（`--purge-shared`） | 必要 |
| `W_SHELL_MODIFICATION` | user PATHまたはshell profileの変更・除去 | 必要 |
| `W_MODE_CHANGE` | setup済みのeffective mode、data root、distribution rootのいずれかを変更 | 必要 |
| `W_RESTART_REQUIRED` | 既存terminal/GUI processへ変更が反映されない | 不要 |

`requires_explicit_approval=true`のcode集合がApprovalの単位である。上表では`W_RESTART_REQUIRED`を除く**7件**が該当する。Approvalが満たさないcodeが1件でもあれば`E_APPROVAL_REQUIRED`とする。`--yes`はこの7件すべてを承認できるが、警告表示自体は消さない。`W_RESTART_REQUIRED`は情報提供であり承認の対象にしない。security failureをwarning codeにしない。

### 16.2 `ResultWarningCode`

処理結果用`ResultWarningCode`は次のexactly 5件とし、Plan approvalには使わない。

| code | 条件 |
|---|---|
| `W_CACHE_STALE` | offline exact解決で期限切れだがidentity/digestが完全なcatalogを利用した |
| `W_CLEANUP_INCOMPLETE` | 主操作成功後に所有を確認した一時物のcleanupが完了しなかった |
| `W_SELECTION_LINK_INCONSISTENT` | selection stateはcommit済みだがcurrent link更新が失敗した |
| `W_ENVIRONMENT_NOTIFICATION_FAILED` | Windows user PATH更新後の環境変更通知だけが失敗した |
| `W_LIFECYCLE_OVERRIDE_UNUSED` | definitionのexact version lifecycle overrideに対応するsource itemがなかった |

human表示、`ResultMeta.Warnings`、CLI JSON envelopeの`warnings`は同じ`ResultWarningCode`、message ID、scalar parametersを使う。security failureをresult warningへ格下げしない。

## 17. CLI JSON envelope

`--json`は読取り専用5 command（`available|installed|current|doctor|version`）だけが持ち、stdoutは完了時のexactly 1 JSON documentとする。

成功:

```json
{
  "schema": 1,
  "ok": true,
  "command": "installed",
  "invocation_id": "33333333333333333333333333333333",
  "data": {},
  "warnings": []
}
```

失敗:

```json
{
  "schema": 1,
  "ok": false,
  "command": "installed",
  "invocation_id": "33333333333333333333333333333333",
  "error": {
    "code": "E_STATE_CORRUPT",
    "message_id": "error.state_corrupt",
    "parameters": {},
    "retryable": false
  },
  "warnings": []
}
```

top-levelは成功/失敗例のkey集合。data/errorを排他にする。warnings entryは`code`, `message_id`, `parameters`で、codeは§16.2の`ResultWarningCode`だけ。parametersはstring/bool/integer/nullのmapだけ。表示済みmessage、Go error、stack、secretをJSONへ入れない。human/JSONは同じtyped Resultから生成する。

成功時`data`のexact keyとvalue typeを次に固定する。

| command | `data`のexact key |
|---|---|
| `available` | `tool_id: string`, `platform_id: string`, `items: CatalogItem[]` |
| `installed` | `installs: InstallSummary[]` |
| `current` | `selections: SelectionSummary[]` |
| `doctor` | `status: string`, `diagnostics: Diagnostic[]`, `report_path: PathValue` |
| `version` | `build: BuildResult` |

named objectのexact keyは次のとおり。全key必須。対象外のstringは空、配列は空、integerは0とする。

| type | exact key |
|---|---|
| `CatalogItem` | §15 itemのexact key集合 |
| `InstallSummary` | `tool_id`, `version`, `platform_id`, `install_id`, `installed_at`, `health`, `receipt_path`, `disk_size`, `provider_kind`, `selected` |
| `SelectionSummary` | `source`, `project_file`, `tool_id`, `version`, `install_id`, `payload_path`, `health` |
| `Diagnostic` | `code`, `severity`, `message_id`, `parameters`, `paths` |
| `BuildResult` | `version`, `commit`, `build_time`, `go_version`, `platform_id`, `state_schema`, `definition_schema`, `registry_schema`, `development` |
| `PathValue` | `role`, `path` |

`selected`, `development`はbool、count/sizeは0以上のinteger、時刻は空またはUTC RFC 3339、parametersはpathを含まないscalar map。`receipt_path`, `project_file`, `payload_path`, `report_path`と`Diagnostic.paths[]`は§17.2の`PathValue`。listはtool ID、version比較、diagnostic codeの各規則で決定的にsortし、unknown keyを拒否する。

`doctor`の`report_path.role`は常に`report`、`path`は`--report`指定時だけ非空とする。`SelectionSummary.project_file.role`は`project-file`、sourceがproject以外ならpathを空にする。sourceがnoneの`payload_path`はrole=`payload`かつpath空とする。progress/log/promptはstderrでありstdout documentへ混ぜない。

### 17.1 機械契約のenum

本章のTOML/JSONに現れるenumは次で閉じる。`unknown`という値を持つenumだけが「不明」を表現でき、それ以外のenumで未定義値を受理しない。

| enum | 出現先 | 値 |
|---|---|---|
| `mode` | `schema.toml`, `setup.toml`, Plan `SetupPlan`, `SetupResult` | `portable\|user` |
| `path_integration` | `setup.toml`, Plan `SetupPlan` | `user-path\|shell-profile\|none` |
| `shell` | `setup.toml`, Plan `SetupPlan` | 空（none/user-path時）または`bash\|zsh\|fish` |
| `integration_identity.kind` | `setup.toml` | `windows-registry-value\|shell-profile-file\|none` |
| backup `kind` | setup backup | `windows-user-path\|shell-profile` |
| `health` | `receipt-index.toml`, `InstallSummary`, `SelectionSummary` | `healthy\|unhealthy\|unknown` |
| `provider_kind` | receipt, catalog, Plan, `InstallSummary` | `official\|third-party`。対象toolがないPlan operationだけ`none` |
| **`checksum_source`** | receipt, catalog, Plan | `asset-field\|text-file`（[06-tool-definition.md](06-tool-definition.md)§7.2のkindと一致） |
| `channel` | catalog, Plan | `stable\|prerelease`。対象toolがないoperationは空 |
| `lifecycle` | catalog, Plan | `supported\|eol\|unknown`。対象toolがないoperationは空 |
| storage `kind` | receipt, Plan | [06-tool-definition.md](06-tool-definition.md)§8のkind |
| storage `scope` | receipt, Plan | `tool\|version` |
| storage `purge` | receipt, Plan | `retain\|explicit\|with-version` |
| probe `status` | receipt | `passed\|skipped` |
| probe `stream` | receipt, definition | `stdout\|stderr\|combined` |
| probe `expect` | receipt, definition | `version\|success\|path-within` |
| probe path `kind` | Plan probe | `file\|directory` |
| archive `format` | definition, Plan extract | `zip\|tar.gz` |
| `working_directory` | receipt, definition | `inherit\|payload` |
| Plan `operation` | Plan | `setup\|setup-remove\|install\|use\|uninstall` |
| setup filesystem capability | Plan `SetupPlan` | `atomic-replace\|directory-rename\|file-identity\|owner-enforcement\|junction\|symlink\|hardlink` |
| setup current link strategy | Plan `SetupPlan` | `junction\|symlink` |
| setup shim strategy | Plan `SetupPlan` | `hardlink\|symlink\|fallback-resolver` |
| Plan arg `kind` | Plan `PlanArg` | `literal\|path` |
| `PlanWarningCode` | Plan `warnings` | §16.1の8値 |
| `ResultWarningCode` | ResultMeta、CLI JSON `warnings` | §16.2の5値 |
| `writes[].action` | Plan | `create\|replace\|remove\|junction\|symlink\|registry-value` |
| `storage[].action` | Plan | `create\|retain\|purge` |
| **`path_role`** | `PathValue`、typed error、`doctor --report` | §17.2の22値 |
| **`severity`** | `Diagnostic` | `error\|warn\|info` |
| **`source`** | `SelectionSummary` | `project\|user\|none` |
| **`doctor_status`** | doctor result | `healthy\|degraded\|unhealthy` |
| **`diagnostic_code`** | `Diagnostic` | `D_ROOT\|D_STATE\|D_REGISTRY\|D_RECEIPT\|D_PAYLOAD\|D_SELECTION\|D_SHIM\|D_PATH\|D_STORAGE\|D_TMP` |
| log `level` | structured log | `error\|warn\|info\|debug\|trace` |

`source`は「effective selectionがどこから来たか」を表し、requestの`scope`（`user\|project`）とは別のenumである。同じ語を両方に使わない。

`Diagnostic.code`は上表の10件をcode順にexactly 1件ずつ返す。各項目が正常なら`severity=info`、継続利用できるが対処を推奨する場合は`warn`、完全性・安全性・必須機能を満たさない場合は`error`とし、具体的な状態はmessage ID、parameters、0件以上のtyped pathsで表す。doctorの`status`はerrorが1件以上なら`unhealthy`、errorなしでwarnが1件以上なら`degraded`、それ以外は`healthy`とする。

### 17.2 `path_role`

永続内部fileのrelative pathを除き、Plan、CLI JSON、typed error、doctor reportへ出すすべてのpathは`PathValue`としてlogical roleと一体にする。`PathValue`のexact keyは`role`, `path`で、roleは次のexactly 22値で閉じる。

```text
data-root            distribution-root    registry             tool-definition
payload              version-data         shared-storage       receipt
receipt-index        catalog              state                state-backup
shim                 shim-index           current-link         project-file
config               download-cache       staging              trash
log                  report
```

roleと対象の対応は次に固定する。同じabsolute pathへ複数roleを恣意的に付けず、最も具体的なroleを使う。

| role | 対象 |
|---|---|
| `data-root` | active data root directory |
| `distribution-root` | 実行中clientのactive distribution root directory |
| `registry` | 同梱registry directoryまたは`registry.toml` header |
| `tool-definition` | 対象tool definition TOML |
| `payload` | 完成またはstaging内のtool payloadとその子path |
| `version-data` | version scope storageとその子path |
| `shared-storage` | tool scope storageとその子path |
| `receipt` | `.gdtvm-install.toml` |
| `receipt-index` | `state/receipt-index.toml` |
| `catalog` | tool/platform catalog JSON |
| `state` | schema/setup/selection等の正本state file |
| `state-backup` | state `.bak`またはsetup integration backup |
| `shim` | shim directory、command shim、fallback resolver |
| `shim-index` | `state/shim-index.toml` |
| `current-link` | toolのmanaged current junction/symlink |
| `project-file` | project `.gdtvm.toml` |
| `config` | distribution隣接`gdtvm.toml`、shell profile、Windows user PATH locator |
| `download-cache` | download cache fileまたはそのpartial metadata |
| `staging` | operation staging/probe-temp directoryとその子path（payloadとして扱う展開後内容を除く） |
| `trash` | operation trash directoryとその子path |
| `log` | structured log file |
| `report` | `doctor --report`出力file |

PlanとCLI JSONの`path`はOS nativeの正規absolute pathとする。`SetupPlan`のprevious root/integration target/backup pathと§17のCLI resultで明示したoptional fieldだけ空を許す。doctor reportでは[10-security.md](10-security.md)§9の規則で`<HOME>`等へmaskした表示pathを同じroleと組にする。typed errorは秘密値や個人pathを露出させず、exact keyを保ったまま`path`を空にしてroleだけを伝えられる。

Windows user PATHのregistry valueはfilesystem pathではないが変更対象の識別が必要なため、`SetupPlan.integration_target`と`writes[]`ではrole=`config`、`action=registry-value`とし、`path`はexact locator `HKCU\Environment\Path`とする。これはPlan `PathValue.path`をabsolute filesystem pathとしない唯一の例外である。Linux shell profileはrole=`config`の正規absolute filesystem pathを使う。

roleを付ける目的は2つある。`doctor --report`（[10-security.md](10-security.md)§9.1）がhome配下のpathをrole単位で確実に置換できること、CIの書込み範囲検査（[11-quality-and-ci.md](11-quality-and-ci.md)§7.2）が実際の書込み先の封じ込めをroleで判定できることである。role未定義のpathを公開境界へ出さない。

## 18. structured log JSON Lines

```jsonl
{"schema":1,"time":"2026-08-07T09:00:00Z","level":"info","invocation_id":"33333333333333333333333333333333","operation_id":"22222222222222222222222222222222","component":"installer","message_id":"install.started","fields":{"tool_id":"node","version":"22.18.0"}}
```

keyは例の集合だけ。level=`error|warn|info|debug|trace`。fields scalar map、keyは§7のscalar parameter key grammar、最大64件。mask後だけserializeし、credential/file content/registry rawを入れない。専用audit logは存在しない。

## 19. lock metadata

`locks/<role>.lock`はOS lockを正本とし、file contentは診断metadataだけ。

```json
{"schema":1,"lock_id":"44444444444444444444444444444444","role":"state","operation_id":"22222222222222222222222222222222","pid":1234,"created_at":"2026-08-07T09:00:00Z"}
```

PID/file ageだけでactive lockを破棄しない。role grammarとlock順は§5と[02-architecture.md](02-architecture.md)§12。lock待ち既定30秒、cancel可能。

## 20. message catalog

`registry/messages/ja.toml`は§7のmessage ID grammarに従うASCII dotted key集合を持ち、値はUTF-8 template string。placeholderは`{name}`、literal braceは`{{`/`}}`。template内ANSI、terminal control、秘密値展開を禁止する。1 message 8 KiB、全体2 MiB。

v0.1の言語は日本語だけだが、message ID機構はそのまま保持する。言語追加時はcatalog fileを増やし、key/parameter集合の一致をtestで担保する。

## 21. 組込み上限

利用者configで拡大できない。

| 対象 | 上限 |
|---|---:|
| global/project/setup/selection TOML各file | 1 MiB |
| tool definition各file | 2 MiB |
| definition platform / alias | 2 / 16 |
| lifecycle override / static version / static asset per version | 10,000 / 10,000 / 16 |
| platform内storage / runtime command / environment profile / probe | 32 / 64 / 16 / 64 |
| その他definition array | 256 |
| version source `cache_ttl` | 1分〜30日 |
| JSON pointer | 255 byte |
| `version_regex` | 1024 byte |
| SPDX expression | 128 byte |
| URL hostname | 253 byte |
| registry manifest各file | 2 MiB |
| receipt各file | 1 MiB |
| catalog JSON各file | 64 MiB |
| upstream metadata response各文書 | 16 MiB |
| checksum text | 2 MiB |
| version source子文書 / catalog item | 32文書 / 10,000 item |
| redirect / network retry | 10 / 初回後3回。backoff 1/2/4秒、Retry-After最大30秒 |
| TTY progress emit間隔 | 100 ms。phase変更/完了は即時 |
| cancel後process graceful猶予 | 5秒 |
| URL / rendered string | 8 KiB / 32 KiB |
| path component / logical path | 255 byte / 32 KiB |
| Windows user PATH | 24,576 UTF-16 code unit |
| archive entry | 200,000 |
| archive単一file / 総展開 | 4 GiB / 20 GiB |
| 圧縮比（entry/全体） | 1,000 |
| artifact download | 20 GiB |
| captureするprocess stdout/stderr | 各16 MiB、超過は末尾1 MiB保持して失敗。shim経由のstdio直接透過は対象外 |
| log 1 line | 256 KiB |
| diagnostic/error集約 | 100件 |
| environment entry / block | 4,096 / OS上限内 |
| `doctor --report` 出力 | 1 MiB |
| lock wait | 30秒 |
| stale backup | 1世代 |

configのcache容量等がこれより大きくてもsecurity hard limitを越えない。加算/multiplication overflow、宣言size不明、圧縮stream途中超過をfail closedで扱う。

## 22. schema互換

v0.1のclientはschema 1だけを作成・読込みする。未知future schemaを推測して読まない。schema revisionを増やす手順は[15-deferred.md](15-deferred.md)へ延期しており、v0.1にmigration機構を実装しない。将来schemaを増やす場合は[14-maintenance.md](14-maintenance.md)の拡張手順でmigration契約を先に仕様化する。
