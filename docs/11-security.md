# セキュリティ仕様

## 1. 信頼境界

信頼度を高い順に区別する。

1. client binaryへ埋込んだregistry公開鍵とbuilt-in schema
2. その鍵で検証したregistry manifest/file
3. SHA-256/upstream signature検証済みartifact
4. hashを表示し利用者が承認したlocal definition
5. 利用者が明示承認した未検証artifact
6. 親process環境、project file、remote response、archive内容、hook出力

下位の入力から上位trustを作らない。HTTPSだけでstandard definitionやartifactを信頼済みにしない。

## 2. Registry security

- Ed25519 public keyをbinaryへ埋込み、manifest raw bytesを検証する。
- manifest検証前にTOML内容を意思決定へ使わない。
- manifest後に全file SHA-256/size/pathを検証する。
- tag version、manifest version、snapshot directory versionの一致を必須にする。
- active snapshot変更は完全検証後のatomic commit。
- signature失敗時にbranch先端やcache未検証版へfallbackしない。
- key rotation/revocationは [07-registry.md](07-registry.md) に従う。

## 3. artifact security

- SHA-256を検証済み最低線とする。
- checksum file自体のsourceがartifactと同一侵害domainだけでも、改ざん検知・誤配布検知として利用する。署名があれば併用する。
- URL host、redirect先、最終URLをreceiptへ記録する。
- digestはstream計算し、検証前fileを実行・展開しない。
- executable installer/helperを検証前に起動しない。
- Windows AuthenticodeやLinux package signatureを将来adapterで検証できるが、初期版でもSHA-256を省略しない。

## 4. third-party portable build

公式portable binaryが存在しないplatformでだけ標準定義に採用できる。各導入前に必ず次を表示する。

- 「公式配布物ではない」明示
- build提供者名とsource/repository URL
- license/SPDX
- 対象tool完全版、OS、arch、libc/variant
- checksum/signature状態
- 公式portable artifactを採用できない理由
- definition registry版とhash

対話で明示確認する。`--yes` は入力を省略できるが表示とauditを省略しない。非対話では、artifactがSHA-256検証済みかつstandard signed definitionである場合に限り `--yes` で許可する。未検証third-partyは常に拒否する。

初期のLinux Python portable buildが代表例である。sourceからgdtvmがビルドする方式はsystem dependencyと時間が大きいため、初期版では標準経路にしない。

## 5. local definition

### 5.1 projectからの隔離

`.gdtvm.toml` はversion選択/disabledだけで、definition path、URL、hook、environment任意値を持てない。単にrepositoryをclone/cdしただけで任意コードが実行されることを防ぐ。

### 5.2 承認fingerprint

local definition承認hashは次のcanonical bundleに対するSHA-256:

- definition raw bytes
- 参照script raw bytes
- 参照helper definition raw bytes
- canonical absolute origin path
- schema major

初回またはどれかの変更時に再承認する。承認画面はtool ID、origin path、bundle hash、artifact hosts、全hook command/script hash、予定write roots、security warningを示す。

`approvals.toml` はhash、path、approved time、client version、policy、last-usedを保存する。file timestampだけで承認しない。

### 5.3 policy

- `deny`: local definitionをparseして一覧へ出せるが実行しない。
- `prompt`（既定）: 対話承認。非対話拒否。
- `allow`: hash変更後も自動許可できるが、毎回warning/audit。projectから変更不可。

## 6. hook security

- 組込みstepを優先し、shell hookを最終手段とする。
- planにないhookは実行しない。
- argv実行を既定にし、shell使用をdefinitionで明示する。
- cwdと宣言write rootsは管理root内。
- sanitized環境を渡し、token、SSH agent、cloud credential、Git credential等を既定で除外する。tool導入に必要なproxy/GitHub tokenはHTTPClientだけが扱いhookへ渡さない。
- stdinを閉じ、対話installerを禁止する。
- timeout、process tree kill、output上限を強制する。
- hook script/commandとdefinition hashをreceiptへ記録する。
- 管理者権限要求を検出したら失敗しmanual prerequisiteを案内する。

## 7. process execution

WindowsはCreateProcess相当の正しいargv quoting、Linuxはexec argvを使う。shell文字列連結を通常processで使わない。

実行対象pathは:

- helper receipt
- tool receiptのpayload相対target
- 許可listされたOS component絶対path

のいずれか。現在PATHから同名実行物を曖昧探索しない。system-command probeだけはPATH探索可だが、解決absolute pathをplanへ固定する。

## 8. path/traversal

- lexical cleanとrealpathの両方でroot containmentを検査する。
- symlink/reparse pointを辿った先がroot外なら拒否する。
- Windows case-insensitive、短い8.3名、UNC、device pathを考慮する。
- `\\?\` prefixは内部canonicalizationだけに用い、definition入力で許さない。
- archive entryのUnicode normalization/case collisionを検査する。
- 削除対象はreceipt/stateから導出し、利用者文字列を直接再帰削除へ渡さない。

## 9. filesystem permission

- portable rootは現在user書込みを要求する。他ユーザー書込み可能ならwarning、shared/multi-user用途は初期非対応。
- user mode state/config/approvals/log backupはowner-onlyを原則とする。
- downloaded executableは検証前にexecute permissionを付けない。
- Linux展開時setuid/setgid/stickyを落とす。
- Windows alternate data streamsをartifactから作らない。

## 10. shell integration

- 変更範囲をmarkerで限定する。
- 元content/valueをbackupし、undoで第三者変更を上書きしない。
- HKCUだけ。HKLM禁止。
- PowerShell execution policyは別承認、CurrentUserだけ、Group Policy回避禁止。
- generated startup fileはnetworkやhookを実行しない。
- profile pathやAutoRun contentをログへ出す場合、user homeを短縮表示できる。

## 11. network

- TLS certificate validation必須。独自CAはOS/Go trust storeの通常設定を使う。
- HTTPはlocal definitionかつ明示policy時だけ。standard registry/artifactでは禁止。
- DNS rebinding対策としてredirectごとにURL再検査する。private address自体は企業mirror用途で一律禁止しない。
- response body、header、page数、時間、展開sizeに上限。
- GitHub/Proxy tokenをURL queryへ入れずAuthorization/headerで扱う。

## 12. logs/audit

mask対象:

- Authorization、Cookie、Proxy-Authorization
- URL userinfo、既知token query
- `*_TOKEN`, `*_PASSWORD`, `*_SECRET`, `*_KEY`環境値
- userがsecret指定した追加header

security auditとして、registry検証、third-party承認、unverified承認/拒否、local definition承認、hook実行、profile変更、force削除を記録する。audit logもrotation対象だが通常info logより長く保持できる。

## 13. 脅威と必須対策

| 脅威 | 必須対策 |
|---|---|
| registry account takeover | embedded key signature |
| tag差替え | tag/commit/manifest hash記録、signature |
| CDN改ざん/誤配布 | SHA-256、任意upstream signature |
| zip slip/archive bomb | preflight path/size/count/ratio |
| malicious project | project schemaをselection限定 |
| local TOML変更 | bundle hash再承認 |
| command injection | argv分離、限定template、shell明示 |
| junction/symlink attack | realpath/root/reparse検査、payload/stateのatomic commit、Windows junction置換中のshim fallback |
| concurrent corruption | process lock、journal、revision |
| UAC/sudo誘導 | elevation禁止、manual prerequisite |
| shim hijack/loop | receipt absolute target、PATH不使用、depth marker |
| credential leakage | sanitized hook env、log mask |

## 14. security failure方針

signature、digest、path containment、archive safety、unapproved definitionはfail closed。offline cacheや`--force`で回避できない。利用者が明示許可できる例外は「checksumが元から提供されないartifact」に限り、対話・警告・receipt/auditを必須とする。
