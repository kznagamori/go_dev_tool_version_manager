# アーキテクチャ仕様

## 1. 基本方針

実装はヘキサゴナル構成に準じ、ドメインとユースケースをOS、CLI、HTTP、TOMLライブラリから分離する。将来のWails v3 GUIはCLIを呼び出さず、CLIと同じアプリケーションサービスを直接利用する。

依存方向は次の一方向とする。

```text
CLI / 将来GUI
      ↓
Application Service（ユースケース）
      ↓
Domain（値、規則、計画、結果）
      ↓ 抽象ポート
Infrastructure（FS、HTTP、Process、Windows/Linux、TOML）
```

DomainからCLI、Wails、具体的OS API、具体的HTTPクライアントを参照することを禁止する。

## 2. Goモジュール内の論理構成

実装時の正確なディレクトリ名は次を標準とする。すべて同一Goモジュール内とし、`internal` 境界を使う。

| 論理領域 | 責務 |
|---|---|
| `cmd/gdtvm` | 引数解析、表示形式、終了コード、サービス呼出し |
| `internal/app` | ユースケースの公開窓口、要求検証、トランザクション境界 |
| `internal/domain` | ToolID、Version、Platform、Plan、Selection、Receipt、Error |
| `internal/config` | 実行file隣接設定、project設定、許可された環境変数の読込みと統合 |
| `internal/definition` | ツール定義の解析、スキーマ検証、テンプレート評価 |
| `internal/registry` | 同梱registryの読込み、schema検証、definition revision構築 |
| `internal/update` | 公式GitHub Release探索、checksum照合、self-update transaction |
| `internal/catalog` | 配布元照会、版正規化、stable判定、カタログキャッシュ |
| `internal/install` | 依存計画、ダウンロード、検証、展開、hook、receipt |
| `internal/selection` | user/project選択、currentリンク、優先順位 |
| `internal/runtime` | 実行環境生成、コマンド解決、子プロセス起動 |
| `internal/shim` | shim metadata生成、呼出名解決、実体委譲 |
| `internal/shell` | setup、profile marker、completion、undo |
| `internal/store` | 状態、カタログ、receipt、承認、journal、atomic write |
| `internal/platform` | Windows/Linux固有のリンク、プロセス、権限、パス |
| `internal/security` | SHA-256、承認、パス検査、マスク |
| `internal/doctor` | 診断規則と修復計画 |
| `internal/events` | 進捗、警告、確認要求、監査イベント |
| `internal/i18n` | メッセージIDとja/enカタログ |

CLIフレームワークやTOMLライブラリの型を `internal/app` の境界に露出させてはならない。

## 3. 主要ドメイン値

| 値 | 必須条件 |
|---|---|
| ToolID | 正規化済みkebab-case、aliasではない |
| Version | 空でない完全版。元文字列と比較用キーを保持 |
| Platform | OS、arch、必要時libc、実行形式suffix |
| Scope | `user` または `project` |
| Mode | `portable`, `user`, または `multi-user` |
| Channel | `stable`, `prerelease`, `nightly`, `eol` |
| Digest | algorithmと小文字hex値。初期実装の必須algorithmはSHA-256 |
| DefinitionOrigin | release同梱registry、approved local、built-in emergencyのいずれか |
| InstallKey | ToolID＋Version＋Platform＋variant |
| EffectiveSelection | 選択値、由来、設定ファイル、導入状態 |

Versionの比較規則はツール定義が指定する。`semver`、数値ドット、JEP 223形式、日付、辞書式、カスタム正規表現抽出を組み込みで提供する。入力一致は比較用キーではなく、カタログに保存された正規完全版の完全一致とする。

## 4. アプリケーションサービス

アプリケーションサービスは [10-internal-api.md](10-internal-api.md) の操作群を提供する。各操作は次の共通規則に従う。

- `context` によるキャンセルと期限を受け取る。
- 要求値、結果値、型付きエラーだけを境界に出す。
- 進捗は戻り値に溜めずイベントsinkへ送る。
- 確認はUIポートへ要求し、CLIかGUIが回答する。
- 非対話モードではUIポートがポリシーに基づいて即答し、入力待ちしない。
- ファイルやプロセスの具体型を戻さない。
- 一つの操作に一つのoperation IDを付与し、ログ、journal、イベントを関連付ける。

## 5. 抽象ポート

最低限、次の抽象を定義する。名称は概念名であり、Goの具体的な宣言そのものではない。

| ポート | 操作 |
|---|---|
| FileSystem | stat、read、atomic write、mkdir、rename、remove、walk、permission、realpath |
| LinkManager | junction/symlink/hardlink作成、リンク種別取得、安全な除去、能力検査 |
| HTTPClient | GET、HEAD、range再開、redirect、proxy、TLS、response limit |
| ProcessRunner | argv実行、環境、cwd、stdio、signal、exit code、timeout |
| ArchiveExtractor | list、安全検査、選択展開、進捗、形式判定 |
| ReleaseIntegrityVerifier | release asset名・size・SHA-256、archive内容、埋込みregistry hashの照合 |
| SignatureVerifier | 上流tool artifactのPGP-detached/Minisign署名検証（[06-tool-definition-schema.md](06-tool-definition-schema.md)6.1節）。client release検証には用いない |
| HashCalculator | streaming digest計算 |
| LockManager | process間共有/排他ロック、所有情報、timeout |
| Clock | 現在時刻、単調時間 |
| UI | event表示、確認、秘密入力不可、対話可否 |
| Logger | 構造化level、operation ID、秘密値マスク |
| LocaleProvider | OSロケールと端末能力 |

UIポートが担うevent出力と確認は、[10-internal-api.md](10-internal-api.md)の生成契約ではEventSinkとPrompt/Approval providerへ分離して注入してよい。抽象境界の粒度は変えても、翻訳文やUI固有型を境界へ出さない点は同じである。

テストでは全ポートをメモリまたは一時ディレクトリ実装へ差し替え可能にする。

## 6. 処理計画と実行の分離

状態を変更する操作は、必ず `Resolve → Plan → Approve → Execute → Commit → Cleanup` の段階を通る。

1. **Resolve**: alias、platform、version、definition、依存を確定する。
2. **Plan**: ダウンロード、検証、外部コマンド、ディスク変更、警告を列挙する。
3. **Approve**: 危険度と対話ポリシーに従って承認を得る。
4. **Execute**: staging領域だけを変更し、journalへ進捗を書く。
5. **Commit**: 完成receiptを記録してから導入先へ原子的に公開する。
6. **Cleanup**: 一時物を除去し、保持ポリシーに従いdownload cacheを整理する。

GUIはPlanを表形式で表示し、CLIは要約して表示できる。Planに含まれない外部コマンドや管理ルート外書込みを実行してはならない。

## 7. 並行処理とロック

ロック順序を固定し、デッドロックを防ぐ。

1. 管理ルートmigration lock
2. registry lock
3. tool catalog lock（ToolID順）
4. backend/shared store lock（ToolID順）
5. install lock（InstallKey辞書順）
6. selection lock（Scope、ToolID順）
7. shim lock

同一InstallKeyの同時導入は後発が待機し、先発成功後に整合性検査だけを行う。rustup等が複数InstallKeyで同じshared storeを変更する場合はToolID単位のbackend lockで直列化する。異なるInstallKeyのダウンロードは `downloads.max_parallel` まで並行可能だが、commitとshim更新は短い排他区間にする。

ロックファイルにはPID、開始時刻、operation ID、hostnameを記録する。PID不在だけで即時破棄せず、OS上のプロセス存在、開始時刻、ロックハンドルを確認する。強制解除は `repair` の明示計画に含める。

## 8. 失敗と回復

- 外部処理失敗は標準エラー末尾、終了コード、step IDを型付きエラーへ含める。
- staging失敗は完成版へ影響させない。
- commit後に選択リンク更新が失敗した場合、導入成功・選択失敗として区別し、`repair` 可能にする。
- self-update失敗時は現在の検証済みdistributionを維持し、commit途中ならbackupからrollbackする。
- 状態ファイル破損時は `.bak`、receipt、実ディレクトリから再構築する。
- cleanup失敗は主操作成功を覆さず警告とdoctor項目にする。
- キャンセルは子プロセスを終了し、journalを `cancelled` にして安全な一時物だけを除去する。

## 9. 外部依存方針

Go標準ライブラリを優先する。外部モジュールは、CLI解析、TOML、Windows API、process間lock、標準ライブラリにないarchive形式など、保守上の利益が明確なものに限定する。

採用時は次を記録する。

- SPDXライセンスと再配布条件
- 最終更新、既知脆弱性、maintainer状況
- transitive dependency一覧
- コアの抽象ポートで置換可能であること
- lockfile相当の `go.sum` 固定と定期更新方針

CLIフレームワークのコマンドオブジェクトからドメイン処理を直接呼び分けず、要求値へ変換してApplication Serviceを1回呼び出す。

## 10. 将来GUIの境界

Wails v3用UIは別のentry pointとして同一モジュールに置き、`internal/app` を利用する。GUI固有要件は次のとおり。

- 長時間操作はoperation IDを返し、event streamで進捗を受ける。
- 確認ダイアログはUIポートの実装とする。
- キャンセルは同じcontextへ伝播する。
- GUI終了後も半端な操作を残さないjournal方式をCLIと共有する。
- API要求/応答に端末制御文字、ANSI、翻訳済み文章を含めず、メッセージIDと構造化値を使う。
- GUIのためにコアへCGOやWails型を導入しない。GUIビルドだけが必要なCGO/WebView依存を持つ。
