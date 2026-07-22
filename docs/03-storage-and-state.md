# 保存領域・状態仕様

## 1. モード決定

ポータブルモードを既定とする。管理ルート決定の優先順位は高い順に次とする。

1. CLIグローバルオプション `--home <absolute-path>`
2. 環境変数 `GDTVM_HOME`
3. CLIグローバルオプション `--mode portable|user`
4. 環境変数 `GDTVM_MODE`
5. 実行ファイルと同じ場所の `gdtvm.bootstrap.toml`
6. 実行ファイル親が書込み不能で、既知のOSユーザー領域に整合する初期化済み`mode="user"` stateが1件ある場合はユーザーモード
7. ポータブルモード

`--home` と `GDTVM_HOME` は明示パスのポータブル相当として扱う。相対パス、空パス、管理ルート自身を含むsymlink loopを拒否する。

`--home`/`GDTVM_HOME` と `--mode user`/`GDTVM_MODE=user` を同時指定した場合は意味が衝突するため `E_USAGE` とする。CLIと環境変数で異なるmodeを指定した場合は通常の優先順位でCLIを採用するが、debug logにoverrideを記録する。

`gdtvm setup --mode user` は、書込み可能なら実行ファイルの隣に次のlocatorだけを保存する。

```toml
schema = 1
mode = "user"
```

実行ファイル隣へlocatorを書けない場合でも、ユーザーモードのglobal config/stateに`mode="user"`とroot IDを記録し、setupしたshell initには`GDTVM_MODE=user`を設定する。shell外の直接起動では上記6の既存state検出を使う。検出は既知の固定ユーザー領域だけを読み、複数候補、schema不正、owner不一致なら採用しない。

実行ファイルのフォルダーが書込み不能で、初期化済みuser stateもなくポータブルモードが暗黙選択された場合、対話時はユーザーモードへの切替を提案する。非対話時は `E_HOME_NOT_WRITABLE` で終了し、未初期化の別領域へ暗黙作成しない。

## 2. ポータブルモード

管理ルートは、symlinkを解決した `gdtvm` / `gdtvm.exe` 実体の親フォルダーとする。最低限の構造は次のとおり。

```text
<root>/
├─ gdtvm[.exe]
├─ gdtvm.bootstrap.toml       # 任意
├─ config/
│  └─ gdtvm.toml
├─ tools/
├─ registry/
├─ cache/
├─ state/
├─ shims/
├─ logs/
├─ tmp/
└─ locks/
```

フォルダー全体を移動した後は `gdtvm repair` を実行する。receiptと状態には管理ルート相対パスを優先して保存し、移動後に絶対パスを一括修復できるようにする。ツール自身が絶対パスを内部ファイルへ埋める場合は、定義に `relocation = "repair-required"` と修復stepを記載する。

## 3. ユーザーモード

### 3.1 Windows

| 種類 | パス |
|---|---|
| 設定 | `%APPDATA%\gdtvm\gdtvm.toml` |
| data root | `%LOCALAPPDATA%\gdtvm` |
| tools等 | data root直下の同名フォルダー |
| logs | `%LOCALAPPDATA%\gdtvm\logs` |

環境変数が欠ける場合は既知フォルダーAPIで取得する。文字列連結だけで推測しない。

### 3.2 Linux

| 種類 | パス |
|---|---|
| 設定 | `${XDG_CONFIG_HOME:-$HOME/.config}/gdtvm/gdtvm.toml` |
| data | `${XDG_DATA_HOME:-$HOME/.local/share}/gdtvm` |
| cache | `${XDG_CACHE_HOME:-$HOME/.cache}/gdtvm` |
| state/locks | `${XDG_STATE_HOME:-$HOME/.local/state}/gdtvm` |

論理上の管理ルートはdata pathとし、cache/state/locks/logsだけがXDG実体へ分離する。PathResolverが論理名から実パスを返し、他コンポーネントはXDG分離を意識しない。

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
registry/
├─ snapshots/<registry-version>/
└─ bootstrap.toml
cache/
├─ downloads/
├─ catalogs/<tool-id>/
├─ registry/
└─ helpers/
state/
├─ schema.toml
├─ selections.toml
├─ registry.toml
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

- `config/gdtvm.toml`、ユーザーモード設定、`.gdtvm.toml`、ローカルツール定義は利用者が編集可能なTOMLである。
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
| definition | registry版、definition相対パス、SHA-256 |
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

receiptはshimがactive registryやnetworkなしで起動先と子環境を再現できる自己完結runtime契約とする。絶対管理rootを埋め込まず、`payload`、`shared`等のlogical rootと相対pathを保存する。receiptがないディレクトリは導入済みと見なさない。`repair` は安全な内容からreceiptを再生成できる場合だけ採用し、それ以外はorphanとして報告する。

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
- registry archiveは最新2世代を既定保持する。検証済みsnapshotは最新2世代に加え、active/previous、導入receipt、進行中operationのいずれかがdefinition hashで参照する版を保持する。参照がなくなったsnapshotだけをLRU削除できる。
- `repair` は参照中snapshot、選択中version、進行中operationのデータを削除しない。
- cache上限超過時はLRUだが、未検証downloadを優先削除する。

## 11. 管理ルート境界

削除、rename、link作成、展開の前に、lexical pathとrealpathの双方が許可root配下であることを検査する。存在しない末尾については最も近い既存親のrealpathを検査する。case-insensitiveなWindowsではcase-fold後も比較する。UNC、mapped drive、ネットワークfilesystemは導入rootとして設定で明示許可しない限り拒否し、Windows junctionはローカルvolumeだけに作成する。
