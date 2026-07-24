# 機械データ契約・既定制限

## 1. 目的

本章は永続状態、catalog、event/JSON、helper、message catalogの正規形を定める。フィールド名は永続化とWails bridgeで使用する規範名であり、Go内部の型名はこれに対応させる。

## 2. 共通表現

| 種類 | 表現 |
|---|---|
| 時刻 | UTC RFC 3339、必要時nano。TOML datetimeまたはJSON string |
| duration | 設定では単位付きstring、runtime resultではinteger milliseconds |
| byte数 | 符号なし64-bit範囲のinteger |
| digest | algorithm `sha256` と64桁小文字hex |
| ID | ASCII。operation/install/approvalはUUID形式を推奨 |
| path | state内は可能なら管理root相対POSIX slash、API resultはOS absolute pathも別field |
| URL | credentialを除いたabsolute HTTPS URL |
| nullable | TOMLではfield省略、JSONではschemaが認めるfieldだけnull |
| enum | 本仕様記載のASCII小文字・hyphen |
| client version | 日本時間release日の `YYYY.mm.DD.XX`。13章1.2節の検証・比較規則 |

TOML stateはunknown keyを拒否する。JSON APIは同一schema major内でunknown fieldを無視してよいが、required field欠落は拒否する。

## 3. `state/schema.toml`

```toml
schema = 1
state_schema = 1
created_at = 2026-07-22T00:00:00Z
last_migrated_at = 2026-07-22T00:00:00Z
client_version = "2026.07.23.00"
mode = "portable"
root_id = "<uuid>"
```

`root_id` はfolder移動後も保持し、同じportable rootを識別する。root absolute pathは保存しない。

## 4. `state/selections.toml`

```toml
schema = 1
revision = 12
updated_at = 2026-07-22T12:00:00Z

[[selections]]
tool_id = "node"
version = "22.18.0"
variant = "default"
install_id = "00000000-0000-4000-8000-000000000001"
```

最上位で許すkeyは`schema`, `revision`, `updated_at`, `selections`だけ。各selectionは`tool_id`, `version`, `variant`, `install_id`をすべて必須とし、正規tool ID、正規完全版、receiptと一致するvariant/install IDでなければならない。`version#variant`のような複合文字列やversionだけの短縮形は許さない。配列はtool IDのUTF-8 byte順、tool IDは一意とする。

user選択は一つの管理ルートにつきtoolごとに1件である。同じportable rootを別OS/archから共用した場合、receipt platformが現在platformと一致しない選択は自動で別artifactへ読み替えずinactiveとして扱う。別platformで`use`すると同toolのuser選択を置換する。複数platformのuser選択を同時保持する機能はschema 1の非目標とする。

## 5. `state/update.toml`

```toml
schema = 1
revision = 4
current_client_version = "2026.07.23.00"
current_registry_tree_sha256 = "<64 lowercase hex>"
last_checked_at = 2026-07-22T00:00:00Z
available_client_version = "2026.07.24.00"
last_success_at = 2026-07-22T00:00:00Z
last_release_tag = "v2026.07.23.00"
last_release_id = 123456789
last_asset_name = "gdtvm_2026.07.23.00_windows_amd64.zip"
last_asset_id = 987654321
last_archive_sha256 = "<64 lowercase hex>"
rollback_relative_path = ".gdtvm-update/previous/2026.07.22.00"
```

最上位で許すkeyは上例のfieldだけとする。このfileはsetup前または一度も更新確認していないuserでは存在しなくてよい。`current_client_version`と`current_registry_tree_sha256`はfile作成時に必須とする。

実行中binaryや同梱registryと値が一致しない場合、multi-user配置を管理者が更新したか、利用者がrelease archiveを手動再展開した可能性があるため、それだけで全state corruptionにしない。binary埋込みversion、registry compatibility、registry tree strict validation、binary埋込みexpected registry tree SHA-256との一致がすべて成功した場合はstale update metadataとしてwarning/auditし、現在値へatomic再同期する。どれかが失敗した場合だけ`E_REGISTRY_INVALID`または`E_STATE_CORRUPT`として通常の変更操作を拒否する。

`last_checked_at`と`available_client_version`はcheck成功時だけ、`last_success_at`から`last_archive_sha256`までは更新成功時だけ組として存在する。更新候補がないcheck成功時は`last_checked_at`だけを更新し、`available_client_version`を削除する。release IDとasset IDはGitHub APIが返した正の64 bit整数、tag/name/digestとともにauditへ記録する。`rollback_relative_path`はdistribution root内の自己更新backupが存在する場合だけ記録し、absolute pathや`..`を禁止する。自己更新のcommit完了後にatomic更新し、失敗時は旧fileを維持する。

## 6. `state/approvals.toml`

```toml
schema = 1
revision = 3

[[local_definitions]]
tool_id = "my-tool"
origin_path = "D:/definitions/my-tool.toml"
bundle_sha256 = "<64 lowercase hex>"
approved_at = 2026-07-22T00:00:00Z
last_used_at = 2026-07-22T00:00:00Z
client_version = "2026.07.23.00"
decision = "allow"

[[artifacts]]
tool_id = "legacy-tool"
version = "1.2.3"
platform_id = "windows-amd64"
url_sha256 = "<hash of sanitized final URL>"
reason = "upstream-checksum-unavailable"
approved_at = 2026-07-22T00:00:00Z
```

unverified artifactのapprovalはinstall operation 1回限りであり、永続approvalを次回自動利用しない。上表はaudit記録である。

checksumが存在しないため、`artifacts` audit entryに `artifact_sha256` を記載しない。download後に参考digestを計算して監査へ残す場合は `observed_sha256` として保存できるが、検証済み判定には使わない。

## 7. `state/setup.toml`

```toml
schema = 1
revision = 2

[[changes]]
id = "<uuid>"
kind = "powershell-profile"
target = "<absolute path or registry key>"
before_exists = true
before_type = "file"
before_sha256 = "<digest>"
after_sha256 = "<digest>"
backup = "setup-backups/<timestamp>/<file>"
applied_at = 2026-07-22T00:00:00Z
client_version = "2026.07.23.00"
active = true
```

registry値ではtypeに`reg-sz`等、backupに値type/contentを安全にencodeしたTOMLを使う。secretを含む可能性があればowner-onlyとしconsoleへ全内容を出さない。

## 8. `state/shim-index.toml`

```toml
schema = 1
revision = 8
client_version = "2026.07.23.00"
registry_schema = 1
registry_tree_sha256 = "<digest>"
generated_at = 2026-07-22T00:00:00Z

[[commands]]
name = "node"
tool_id = "node"
runtime_command_id = "node"
definition_sha256 = "<digest>"
```

commandsはname、tool_id、runtime_command_idのbyte順。同じnameを異なるtoolが公開するentryは許すが、3項目が同一の重複は拒否する。local definition originの場合はregistry tree hashでなくapproval bundle hashもentryへ持つ。target version/payload absolute pathは保存しない。shimは同名候補の有効selectionを解決し、複数候補が有効なら `E_COMMAND_AMBIGUOUS` とする。

## 9. receipt完全形

`.gdtvm-install.toml` の正規field:

```toml
schema = 1
install_id = "<uuid>"
tool_id = "node"
version = "22.18.0"
variant = "default"
platform_id = "windows-amd64"
os = "windows"
arch = "amd64"
libc = "none"
installed_at = 2026-07-22T00:00:00Z
payload_root = "payload"
relocation = "portable"
mutable_payload = false
selection_strategy = "link"

[definition]
origin = "registry"
client_version = "2026.07.23.00"
registry_tree_sha256 = "<digest>"
path = "tools/node.toml"
sha256 = "<digest>"

[[artifacts]]
id = "windows-amd64-archive"
role = "primary"
recipe_id = "windows-amd64-archive"
kind = "official"
source = "https://nodejs.org/"
repository = "https://github.com/nodejs/node"
license = "MIT"
url = "<sanitized final URL>"
file = "node-v22.18.0-win-x64.zip"
size = 12345678
digest_algorithm = "sha256"
digest = "<digest>"
verification = "verified"
upstream_signature = "not-provided"

[[commands]]
name = "node"
launcher = "native"
target = "node.exe"
args = []
codepage = "inherit"
working_directory = "inherit"
environment_profile = "default"
passthrough_signals = true

[[environment_profiles]]
id = "default"
shell_export_path = true

[[environment_profiles.path_prepend]]
root = "payload"
path = "."

[[dependencies]]
kind = "managed-tool"
tool_id = "example"
version = "1.0.0"
variant = "default"
install_id = "<uuid>"

[[steps]]
id = "extract"
kind = "extract"
status = "succeeded"
exit_code = 0

[[approvals]]
kind = "normal"
approval_id = "<uuid>"
decision = "allow"
evidence_sha256 = "<digest>"
approved_at = 2026-07-22T00:00:00Z

[[important_files]]
path = "payload/node.exe"
size = 123456
sha256 = "<digest>"
```

`verification`: `verified`, `verified-signed`, `unverified-approved`。`upstream_signature`: `verified`, `not-provided`, `not-applicable`。Hook/script本文やsecret envはreceiptへ保存しない。

command `target` はlink方式では`payload_root`、backend方式では`backend.shared_root`からの相対pathであり、`payload/`や`shared/`を重ねて含めない。script launcherは`interpreter` IDとinstall時に検証した`interpreter_path`も持つ。`args`、launcher、codepage、working directory、signal方針もreceiptを正とし、shim実行のためにactive definitionを必須にしない。

`important_files.path`はreceipt directoryからの相対pathであるため、link方式では通常`payload/...`から始まる。command targetとは基準が異なる。どちらもabsolute path、`..`によるroot逸脱、OS separator混在を拒否する。

`omitted_commands`は`name`, `reason`を持つarray of tableで、definitionの`required=false` commandがtarget欠落のため公開されなかった場合だけ記録する。targetが存在するrequired/optional commandは`commands`へ入り、同じnameを両方へ持たせない。

environment profile（定義側の`[[platforms.runtime.environment]]`とcommandの`env_profile`に対応し、receiptでは`[[environment_profiles]]`および`environment_profile`として確定する）は次の正規契約を持つ。

- `path_prepend`/`path_append`要素: `root`（`payload`または`shared`）とroot内相対`path`。
- `variables`要素: `name`, `operation`（`set`/`unset`）, `value_kind`（set時の`literal`/`path`）。literalは`value`、pathは`root`と相対`path`を持つ。
- variableの`override_allowed`, `shell_export`はbool。profileの`shell_export_path`もbool。
- Windowsではvariable nameをcase-insensitiveに一意化する。root外へ出るpath、absolute `path`、未定義rootを拒否する。

空配列は省略できる。receiptに保存するruntime契約はinstall時に検証済みdefinitionから確定し、後のself-updateで書き換えない。definition origin/hashは説明、監査、repair、uninstall hook照合に使う。自己更新で標準definitionが変わってもreceipt内runtime契約を保持する。local definitionの旧hookが取得不能またはhash不一致なら実行せず、安全な管理root内削除だけをplanへ示す。

`approvals`は実際に承認を要した項目だけを持つ。通常の確認省略installでは空または省略する。third-party、unverified、local definition、hook等ではkind、approval ID、decision、evidence hash、時刻を残すが、prompt本文や秘密値は保存しない。

backend方式のreceiptは `payload_root` を省略し、次を持つ。

```toml
selection_strategy = "backend"

[backend]
kind = "rustup"
selector = "1.88.0-x86_64-pc-windows-gnu"
shared_root = "../../../shared"
```

`shared_root` もreceiptから管理tool root内を指す相対pathである。backend selector、shared receipt、required probeが一致しなければ導入済みと見なさない。

## 10. catalog JSON

fileはUTF-8 JSON、top-level object:

```json
{
  "schema": 1,
  "tool_id": "node",
  "platform_id": "windows-amd64",
  "definition_sha256": "<digest>",
  "fetched_at": "2026-07-22T00:00:00Z",
  "expires_at": "2026-07-23T00:00:00Z",
  "source_url": "https://nodejs.org/dist/index.json",
  "etag": "<optional>",
  "last_modified": "<optional>",
  "versions": []
}
```

version要素のrequired fields:

| field | 型/内容 |
|---|---|
| version | exact string |
| raw_version | source表記 |
| comparison_key | string arrayまたはinteger array。schemeに応じ一方 |
| channel | enum |
| published_at | timestampまたはnull |
| metadata | string→stringだけ。definitionで宣言済みkey |
| artifacts | array |

artifact要素:

`id`, `role`, `priority`, `platform_id`, `recipe_id`, `source`（template/asset）, `asset_name` nullable, `url`, `file`, `format`, `kind`, `size` nullable, `license`, `homepage`, `checksum` object, `signature` object nullable, `availability` object, `metadata`。同一version/platform/roleはexactly 1件で、primary roleを必須とする。補助roleはcatalogへ確定保存してよいが、install definitionから未参照ならdownloadしない。

checksum objectは `algorithm`, `value` nullable, `source`, `required`, `status`（`resolved`, `pending`, `unavailable`）を持つ。`pending`は検証済みdefinitionにchecksum取得URL/asset/pointerと一意なparse規則があり、refresh時に値だけを遅延取得した状態である。catalogには保存してよいが、Install Plan前のResolve段階でchecksumを取得・cacheして`resolved`にしなければdownloadへ進めない。offlineでは検証済みchecksum cacheがなければ`E_CATALOG_MISSING`。`unavailable`か取得規則自体がないrequired checksumはinstallableにしない。これによりNode等の全過去版についてrefresh時にchecksum fileを一括downloadせず、安全性を落とさず遅延解決できる。JSON object keyはgeneratorで辞書順、versionsはversion降順、artifactsはrole順＋priority降順＋ID順。

## 11. operation journal JSON Lines

1行1object、flush境界は重要state transitionごと。共通field:

`schema`, `operation_id`, `sequence`, `timestamp`, `record_type`, `state`, `step_id`, `data`。

record_typeは `created`, `transition`, `step`, `commit`, `cleanup`, `terminal`。最後のterminalがなければcrash中断とみなす。dataへsecret、raw Authorization、全環境mapを書かない。

回復時はjournalだけを信頼して削除せず、operation IDとstaging receipt/path containmentを照合する。

## 12. JSON CLI envelope

短い読取りcommandの `--json` は単一object:

```json
{
  "schema": 1,
  "command": "current",
  "operation_id": "<uuid>",
  "ok": true,
  "result": {},
  "warnings": [],
  "error": null
}
```

error object:

`code`, `category`, `message_id`, `args`, `retryable`, `remediations`, `context`。localized proseは任意field `message` とし、機械処理はcode/message_idを使う。

長時間commandのNDJSONは最初が`operation-started`、最後が必ず`operation-completed`または`operation-failed` event。event契約は [10-internal-api.md](10-internal-api.md)。子processがraw stdoutを使用する`exec --json`は、gdtvm event JSONと混在するため禁止し `E_USAGE` とする。将来別fd/protocolを設けるまで通常stdioを使う。

## 13. Plan data

Action required fields:

- `id`, `kind`, `depends_on`, `tool_id`, `install_key`
- `summary_message_id`, `summary_args`
- `reads`, `writes`（sanitized logical paths）
- `network` nullable（method, host, expected bytes）
- `process` nullable（executable, redacted args, cwd, timeout）
- `security`（trust level, digest/signature, approval IDs）
- `reversible`, `rollback_action`

Warning required fields: `id`, `severity`, `kind`, `message_id`, `args`, `approval_required`, `evidence`。

Plan fingerprintは、global security関連設定、project file bytes、selection revision、definition bundle hash、catalog entry hash、同梱registry tree hashのSHA-256集合からcanonical生成する。

## 14. helper definition

helper fileは `helpers/<helper-id>.toml`、schema 1。helperはuser選択やshim公開を持たない。次の`example.invalid`と全ゼロdigestは構造説明用であり、registry受入れ可能な実artifactを表さない。

```toml
schema = 1

[helper]
id = "example-extractor"
name = "Example Extractor"
homepage = "https://example.invalid/"
license = "Apache-2.0"
version = "1.2.3"

[[platforms]]
id = "windows-amd64"
os = "windows"
arch = "amd64"
entrypoint = "bin/extract.exe"

[[platforms.artifacts]]
id = "main-zip"
role = "primary"
priority = 100
source = "template"
url = "https://example.invalid/releases/1.2.3/example-windows-amd64.zip"
file = "example-windows-amd64.zip"
format = "zip"

[platforms.artifacts.checksum]
algorithm = "sha256"
kind = "literal"
value = "0000000000000000000000000000000000000000000000000000000000000000"
required = true

[[platforms.install.steps]]
id = "download"
kind = "download"
artifact = "primary"
output = "helper-archive"

[[platforms.install.steps]]
id = "verify"
kind = "verify-digest"
depends_on = ["download"]
input = "{{outputs.helper-archive}}"
artifact = "primary"

[[platforms.install.steps]]
id = "extract"
kind = "extract"
depends_on = ["verify"]
input = "{{outputs.helper-archive}}"
destination = "{{helper.staging}}/payload"
format = "artifact"
output = "helper-payload"

[[platforms.validation.probes]]
id = "version"
executable = "{{helper.payload}}/bin/extract.exe"
args = ["--version"]
stdout_regex = "(?P<version>1[.]2[.]3)"
version_capture = "stdout"
expected_version = "1.2.3"
timeout = "30s"
required = true
```

許可key:

- helper: id, name, homepage, license, version, source_repository, notes
- platform: id, os, arch, libc, entrypoint, artifacts, dependencies, install, validation
- artifacts/checksum/signature、dependencies、install、validationはtool definitionの同名schema subset

helperはversion sourceを持たずclient releaseごとに完全版固定する。platformは現在OS/arch/libcにexact matchする1件だけを選ぶ。同一条件の複数variantやpriority選択はhelper schema 1では許さない。artifactは`source="template"`だけを許し、primary roleを必須とする。補助roleも定義できるため、7-Zipのbootstrap実体と完全版installerを別々に検証できる。すべてのartifactでSHA-256必須、checksum `none`は禁止する。

install stepは`ensure-dir`, `download`, `verify-digest`, `verify-signature`, `extract`, `move`, `rename`, `copy-file`, `remove`, `run`, `probe`, `discover-files`, `select-files`, `set-output`だけを許す。`write-file`, `patch-text`, `extract-msi-admin`, shell hookはhelper自身の導入では禁止する。`run`で起動できるのは同operationでdigest検証済みのartifact、先行stepで展開済みのhelper staging実体、または明示したsystem dependencyだけである。helper-specific templateは`{{helper.version}}`, `{{helper.staging}}`, `{{helper.payload}}`で、toolの`{{version}}`, `{{staging}}`, `{{payload}}`を混在させない。

entrypointは完成helper payloadからのPOSIX slash相対pathで、absolute path、`..`、symlink/reparse point、16 MiB超の実体を拒否する。ただし外部program本体が16 MiBを超える場合はplatformに`entrypoint_max_bytes`を明示し、built-in artifact上限以内で拡大できる。required validation probe成功後、helperを`cache/helpers/<id>/<version>/<platform-id>/payload`へatomic commitし、artifact digest、entrypoint digest、license、definition hashをhelper receiptへ保存する。

`run-helper`はhelper IDとregistryが固定したversion/platformを依存planから解決し、receiptのentrypoint digestを毎回検査して絶対path起動する。OSやPATHに偶然存在する同名commandへfallbackしない。複数toolが同じhelperを要求した場合はhelper install lockで一度だけ導入し共有する。tool uninstallでhelperを削除せず、参照receiptがなくcache policyが削除可能と判断した場合だけ清掃する。

helper同士の依存はDAGで許可するが、user選択、shim公開、system変更hook、shell integration、runtime command公開は禁止する。SPDX `LicenseRef-*` を使う場合は対応license textをrelease archive内の`registry/licenses/`へ同梱する。

Windows `msiexec.exe`のようなOS componentはdownload helperに偽装しない。`system-command` dependencyとしてsystem directory APIから絶対pathを解決し、署名/所有元、非昇格probe、version情報をreceipt/journalへ記録する。見つからない場合にネットワークから同名exeを取得してはならない。

## 15. message catalog

`messages/ja.toml`, `messages/en.toml`:

```toml
schema = 1
language = "ja"

[messages]
"error.tool_unknown" = "ツール {tool_id} は見つかりません。"
```

message IDはASCII lower dot path。placeholderはASCII identifierで、ja/en間の集合が完全一致しなければregistry/client CI失敗。format specifierや任意template実行を許さず、値を文字列化して挿入する。security evidenceのURL/path/hashは翻訳messageへ埋め込まずstructured表示する。

client本体の基本messageはbinary同梱。registry messageはtool固有notesだけで、基本errorを上書きできない。

## 16. built-in制限

設定既定とhard maximum:

| 対象 | 既定 | hard maximum |
|---|---:|---:|
| `registry/registry.toml` bytes | 2 MiB | 16 MiB |
| 1 definition bytes | 2 MiB | 16 MiB |
| release metadata response/file | 2 MiB | 16 MiB |
| release client archive | 1 GiB | 4 GiB |
| catalog response | 32 MiB | 256 MiB |
| artifact download | 64 GiB | 256 GiB |
| download cache total | 10 GiB | 1 TiB |
| archive entries | 1,000,000 | 2,000,000 |
| total extracted | 128 GiB | 512 GiB |
| single extracted file | 64 GiB | 256 GiB |
| compression ratio | 1000 | 2000 |
| external stdout+stderr capture | 8 MiB | 64 MiB |
| redirect count | 10 | 20 |
| definition step count | 512 | 2048 |
| dependency nodes/plan | 128 | 512 |
| template rendered string | 32 KiB | 1 MiB |
| path components | 256 | OS上限以下 |

definitionはこれらを増加できない。05章の`gdtvm.toml`に対応fieldが公開されている対象だけをhard maximum以内で変更でき、対応fieldのない`registry/registry.toml`、definition step count、dependency nodes、template rendered string、path components等は表のbuilt-in既定値に固定する。expected artifact sizeが設定済み上限を超えるtoolはinstall前に、変更可能なfieldであれば上限変更を案内し、変更不能なbuilt-in上限またはhard maximumを超える場合は非対応理由を示す。いずれも途中までdownloadしない。

## 17. migration

state schema migrationは `N → N+1` の逐次関数だけを実装し、skip migrationをしない。

1. 全state lock。
2. current clientが元schemaと先schemaを理解することを確認。
3. `state/migration-backups/<timestamp>/`へ対象fileをcopyしdigest記録。
4. 一時領域で全変換・全validation。
5. 同一volume renameでcommit。
6. `schema.toml`を最後に更新。
7. failure時は全fileを元へ戻す。

tool payloadをstate migrationで変更しない。receipt migrationが必要なら個別fileをbackupし、payload digestと対応を維持する。新clientが未知の将来schemaを見た場合、downgrade書込みをせずread-only errorにする。
