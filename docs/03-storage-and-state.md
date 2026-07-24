# 保存領域・状態仕様

## 1. モード決定

ポータブルモードをrelease同梱`gdtvm.toml`の既定とする。modeは高い順にCLI `--mode portable|user|multi-user`、実行file隣`gdtvm.toml`、built-in portable既定で決める。

別bootstrap fileや`GDTVM_MODE`を使用しない。`--home`は開発・一時実行用にuser data rootだけをabsolute pathでoverrideし、distribution root、config、registryの場所は変えない。portable modeで`--home`を指定すると配置が分裂するためusage errorとする。

multi-userかつconfigが明示許可した場合だけ`GDTVM_USER_HOME`をuser data rootに使用する。user modeのdata rootは高い順にCLI `--home`、非空の`paths.user_home`、OS既定値、multi-userはCLI `--home`、許可された`GDTVM_USER_HOME`、OS既定値で決める。それ以外のgdtvm固有環境変数はmode/path決定に使用しない。

実行fileと同じdirectoryの`gdtvm.toml`または`registry/`がない場合、別directoryやnetworkを探索せず`E_CONFIG_SCHEMA`または`E_REGISTRY_INVALID`とし、同じrelease archiveの再展開を案内する。

## 2. ポータブルモード

管理ルートは、symlinkを解決した `gdtvm` / `gdtvm.exe` 実体の親フォルダーとする。最低限の構造は次のとおり。

```text
<root>/
├─ gdtvm[.exe]
├─ gdtvm.toml
├─ README.md
├─ USER_GUIDE.md
├─ LICENSE
├─ registry/                  # release同梱、通常read-only
├─ tools/
├─ cache/
├─ state/
├─ shims/
├─ logs/
├─ tmp/
└─ locks/
```

フォルダー全体を移動した後は `gdtvm repair` を実行する。receiptと状態には管理ルート相対パスを優先して保存し、移動後に絶対パスを一括修復できるようにする。ツール自身が絶対パスを内部ファイルへ埋める場合は、定義に `relocation = "repair-required"` と修復stepを記載する。

## 3. user・multi-user mode

### 3.1 Windows

| 種類 | パス |
|---|---|
| 実行file/config/registry | distribution root |
| data root | `%LOCALAPPDATA%\gdtvm` |
| tools等 | data root直下の同名フォルダー |
| logs | `%LOCALAPPDATA%\gdtvm\logs` |

Known Folder APIでuser pathを取得し、文字列連結だけで推測しない。multi-userで明示許可された`GDTVM_USER_HOME`があればdata rootを置換する。共有distributionがread-onlyでも各user data rootへ書き込める。

### 3.2 Linux

| 種類 | パス |
|---|---|
| 実行file/config/registry | distribution root |
| data | `<OS user home>/.local/share/gdtvm` |
| cache | `<OS user home>/.cache/gdtvm` |
| state/locks | `<OS user home>/.local/state/gdtvm` |

OS user homeはOS APIから取得し、`HOME`やXDG環境変数を通常のpath決定に使用しない。論理上のuser data rootはdata pathとし、cache/state/locks/logsは上表へ分離する。multi-userで明示許可された`GDTVM_USER_HOME`があれば全user可変dataをそのroot下へまとめる。共有distributionのconfig/registryをuser別にcopyしない。

## 4. 詳細ディレクトリ

```text
tools/
└─ <tool-id>/
   ├─ versions/
   │  └─ <encoded-version>/
   │     └─ <encoded-variant>/
   │        ├─ payload/              # link方式のツール実体。backend方式では存在しない
   │        └─ .gdtvm-install.toml   # receipt
   ├─ current                        # link方式だけのuser選択link
   ├─ shared/                        # GOPATH、PUB_CACHE等
   └─ helpers/                       # tool専用の検証済み小型helper
cache/
├─ downloads/
├─ catalogs/<tool-id>/
├─ updates/
└─ helpers/
state/
├─ schema.toml
├─ selections.toml
├─ update.toml
├─ approvals.toml
├─ setup.toml
├─ shim-index.toml
├─ shell/                         # shell別の安全な環境snapshot/init
└─ operations/<operation-id>.jsonl
shims/
├─ gdtvm-shim[.exe]              # gdtvm本体へのhardlink/symlink、または内蔵fallback shimの検証済み展開
└─ <exposed-command>[.exe]
tmp/
└─ operations/<operation-id>/
locks/
└─ *.lock
```

`encoded-version` と `encoded-variant` は元文字列のUTF-8 bytesをpercent-encodingしたものとし、ASCII英数字、dot、hyphen、underscoreは保持する。Windows予約名、末尾dot/space、`/`、`\`、colon、NULを直接使わない。variant省略時もdirectory名は `default` とする。receiptが表示用の元version/variantを保持する。

## 5. ユーザー編集可能ファイルと内部状態

- distribution rootの`gdtvm.toml`、`.gdtvm.toml`、ローカルツール定義は利用者または共有配置管理者が編集可能なTOMLである。
- distribution rootの`registry/`はrelease所有であり直接編集をサポートしない。変更はlocal definitionを使う。
- `state/*.toml` とreceiptもTOMLだが、gdtvm所有であり直接編集をサポートしない。
- operation journalは追記効率と回復性のためJSON Linesとする。これは設定形式ではない。
- available catalogはサイズとストリーミング処理のため正規化JSONとする。元レスポンスを永続保存しない。

すべてのファイル先頭に整数 `schema` を持たせる。未知のmajor schemaは読込みを拒否し、古いschemaはバックアップ後にmigrationする。

## 6. 選択状態

`state/selections.toml` はuser選択だけを保持する。各選択は、definition更新後も同じ導入物を指せるよう完全版だけでなくvariantとinstall IDを必ず保持する。

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

`selections` はtool IDのUTF-8 byte順で保存し、同じtool IDを重複させない。参照receiptが現在のOS/arch/libcで利用できなければ選択済みとして実行せず、`doctor`と`use`による再選択を案内する。プロジェクト選択は `.gdtvm.toml` 自体が唯一の正とし、stateへ複製しない。`revision` は楽観的競合検出に用いる。

## 7. 導入receipt

各導入先の `.gdtvm-install.toml` は最低限次を保持する。

| 項目 | 内容 |
|---|---|
| schema | receipt schema |
| install_id | UUIDまたは同等の衝突しないID |
| tool_id/version/variant | 正規値 |
| platform | OS、arch、libc |
| definition | client版、registry tree SHA-256、definition相対パス、definition SHA-256 |
| artifacts | roleごとの最終URL、ファイル名、サイズ、digest、検証方式、source/license |
| installed_at | UTC |
| payload_root | link方式ではreceiptからの相対パス。backend方式では省略 |
| exposed_commands | command名、launcher、固定引数、link方式のpayload内またはbackend方式のshared内相対target |
| omitted_commands | optional commandを公開しなかったnameと理由 |
| environment | path entryと変数操作をlogical root＋相対pathまたはliteralへ確定したruntime profile |
| dependencies | install key一覧 |
| files_manifest | 任意。重要ファイルの相対パスとdigest |
| relocation | portable、repair-required、fixed |
| selection_strategy/backend | link、またはbackend kindと完全selector |
| hooks | 実行step IDと終了状態。秘密引数は保存しない |

receiptはshimが同梱registryやnetworkなしで起動先と子環境を再現できる自己完結runtime契約とする。絶対管理rootを埋め込まず、`payload`、`shared`等のlogical rootと相対pathを保存する。receiptがないディレクトリは導入済みと見なさない。`repair` は安全な内容からreceiptを再生成できる場合だけ採用し、それ以外はorphanとして報告する。

## 8. 原子的書込み

状態更新は次の手順とする。

1. 同じディレクトリにランダムsuffix付き一時ファイルを作る。
2. 所有者だけが読書きできるpermissionで開く。
3. 全内容を書き、flushする。
4. 既存ファイルがあれば `.bak` を一世代だけ更新する。
5. 同一ボリューム上のrename/replaceで置換する。
6. 可能なら親ディレクトリもflushする。

Windowsでantivirus等によりreplaceが一時失敗した場合だけ短い有限再試行を行う。別ボリュームへのrenameをatomicと見なさない。

## 9. currentリンク

- 本節はtool platformの `selection_strategy="link"` にだけ適用する。`backend` は状態とbackend selectorで解決する。
- link自身は `tools/<id>/current` に固定する。
- targetは同じtool root内の `versions/<encoded-version>/<encoded-variant>/payload` でなければならない。
- Windowsはディレクトリ・ジャンクション、Linuxは相対symlinkを使う。
- linkを削除する際、targetを再帰削除してはならない。reparse point/symlinkそのものだけを除去する。
- link targetと `selections.toml` の不一致はstateを正として `repair` する。
- linkの作成失敗はshim direct-target modeへフォールバックできるが、診断warningを残す。shell環境snapshotはその場合に選択payloadの絶対pathを使うため、次回切替時にshell/VS Code再起動が必要になる。

## 10. キャッシュ保持

- 成功した導入のarchive/installerは `downloads.retain = false` の既定で削除する。
- 失敗した`.part`は再開可能な場合だけ期限内保持する。
- catalogは取得時刻、ETag、Last-Modified、definition hash、platformを持つ。
- `updates.retain_previous=true`（既定）の場合、self-update backupは直前releaseを1世代保持する。falseでもtransaction完了までは一時backupを必須とし、成功確認後に削除する。現在distribution、rollback対象、進行中operationが参照するbackupは削除しない。
- `repair` は現在distribution、rollback対象、選択中version、進行中operationのデータを削除しない。
- cache上限超過時はLRUだが、未検証downloadを優先削除する。

## 11. 管理ルート境界

削除、rename、link作成、展開の前に、lexical pathとrealpathの双方が許可root配下であることを検査する。存在しない末尾については最も近い既存親のrealpathを検査する。case-insensitiveなWindowsではcase-fold後も比較する。UNC、mapped drive、ネットワークfilesystemは導入rootとして設定で明示許可しない限り拒否し、Windows junctionはローカルvolumeだけに作成する。
