# セキュリティ仕様

## 1. 信頼境界

信頼度を高い順に区別する。

1. client binaryへ埋込んだ公式repository identity、expected registry tree SHA-256、built-in schema
2. TLS検証済みHTTPSで公式GitHub repositoryから取得したpublished Release metadataと`checksums.txt`
3. `checksums.txt`とSHA-256が一致し、構造検証済みのrelease archive
4. release archiveへclientとともに同梱され、binary埋込みtree hashと一致する標準registry
5. SHA-256/upstream signature検証済みtool artifact
6. hashを表示し利用者が承認したlocal definition
7. 利用者が明示承認した未検証artifact
8. 親process環境、project file、remote response、archive内容、hook出力

下位の入力から上位trustを作らない。`checksums.txt`とarchiveは同じGitHub Releaseから取得するため、SHA-256照合は転送破損・asset取り違え・部分downloadを検出するが、GitHub account、release権限、workflowが侵害されchecksumとarchiveを同時に置換された場合の真正性は保証しない。

## 2. Release・registry security

- self-update先はbinaryと05章に固定した`kznagamori/go_dev_tool_version_manager`だけとし、owner/name、HTTPS scheme、GitHub API/download hostをredirectごとに再検査する。
- `checksums.txt`のcanonical形式と4 archiveの完全なentry集合を検査し、対象asset名に対応するSHA-256が1件だけ存在する場合に限りexpected digestとして使う。
- archive SHA-256一致前に展開内容を実行・active配置しない。
- release tag、`/VERSION`、archive名、archive root構造、binary埋込みversionの一致を必須にする。
- archive内のclient、config template、registry、README、USER_GUIDE、LICENSEを一つのtransactionで更新する。
- binaryへ同じreleaseのregistry tree SHA-256を埋め込み、archive検証後と通常起動時に同梱treeとの一致を確認する。不一致をarchive署名の代替として扱わず、改変・混在検出として使用する。
- checksum/digest/identity検査失敗時にlatest URL、branch内容、cache未検証assetへfallbackしない。
- standard registry file個別のmanifest・署名は作らない。同梱archiveの検証とsource CIを信頼境界とする。
- protected tag、branch protection、CODEOWNERS review、最小workflow permission、pinned action、GitHub environment approval、公開後asset再取得検査をrelease運用の必須防御とする。
- GitHub artifact attestation/provenanceは監査と利用者による追加検証に発行するが、schema 1のself-update成功条件にはしない。attestation未取得をSHA-256省略理由にしない。

## 3. artifact security

- SHA-256を検証済み最低線とする。
- tool提供元のchecksum fileがartifactと同一侵害domainだけでも、改ざん検知・誤配布検知として利用する。tool提供元がartifact署名も公開しており定義で検証を必須または任意指定した場合は、06章の上流artifact署名検証も併用する。この規則はgdtvm client releaseには適用しない。
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
- client release版、registry tree revision、definition hash

対話で明示確認する。`--yes` は入力を省略できるが表示とauditを省略しない。非対話では、artifactがSHA-256検証済みかつ検証済みrelease同梱standard definitionである場合に限り `--yes` で許可する。未検証third-partyは常に拒否する。

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

- portable rootは現在user書込みを要求する。他ユーザー書込み可能ならwarningとし、portable rootの共有利用は非対応。multi-user modeは管理者所有のread-only shared distributionとuser別data rootに限って対応する。
- user mode state/config/approvals/log backupはowner-onlyを原則とする。
- downloaded executableは検証前にexecute permissionを付けない。
- Linux展開時setuid/setgid/stickyを落とす。
- Windows alternate data streamsをartifactから作らない。

## 10. shell integration

- 変更範囲をmarkerで限定する。
- 元content/valueをbackupし、undoで第三者変更を上書きしない。
- HKCUだけ。HKLM禁止。
- PowerShell execution policyはscopeを問わず変更せず、Group Policyを回避しない。
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

security auditとして、self-updateのofficial repository/release/asset IDとchecksum照合、registry schema/tree検証、third-party承認、unverified承認/拒否、local definition承認、hook実行、profile変更、force削除を記録する。audit logもrotation対象だが通常info logより長く保持できる。

## 13. 脅威と必須対策

| 脅威 | 必須対策 |
|---|---|
| GitHub account/release/workflow改ざん | branch/tag protection、CODEOWNERS、最小workflow permission、pinned action、environment approval、attestation、公開後監視。client単体では同時改ざんを暗号学的に検出できない |
| tag/asset差替え | immutable運用、tag/version/release ID/asset ID/digest記録、公開済みassetの上書き禁止、公開後再取得比較 |
| tool artifactのCDN改ざん/誤配布 | SHA-256、定義で指定した任意の上流artifact署名 |
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

release archive digest、artifactの必須signature/digest、official repository identity、path containment、archive safety、unapproved definitionはfail closed。offline cacheや`--force`で回避できない。利用者が明示許可できる例外は「tool artifactにchecksumが元から提供されない場合」に限り、対話・警告・receipt/auditを必須とする。client release archiveは例外なくSHA-256照合を必須とする。
