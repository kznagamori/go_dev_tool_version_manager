# Codex CLI 作業指示

## 1. 適用範囲

このファイルはrepository全体に適用する。`go_dev_tool_version_manager`（CLI名`gdtvm`）の実装、標準registry、テスト、公開文書、release工程は、必ず`docs/`以下の仕様書に従う。

作業開始時は最初に`docs/README.md`を全文読み、文書の規範領域、矛盾時の扱い、固定された製品判断を確認する。その後、対象タスクに関係する番号付き仕様書と`docs/16-implementation-progress.md`を読む。要約や本ファイルは仕様書の代替ではない。

## 2. 仕様の優先順位

1. 利用者から与えられた現在の指示
2. `docs/README.md`が定める規範領域と、該当する番号付き仕様書
3. 本ファイル
4. 実装済みコード、テスト、既存の慣例

仕様書間に矛盾がある、またはenum、既定値、上限、失敗時動作、platform差を一意に決められない場合は、実装を開始または継続しない。矛盾箇所と影響範囲を報告し、仕様、schema、テスト、進捗項目を先に同期修正する。Go標準ライブラリ、外部library、OS、既存コードの暗黙既定値で仕様不足を補わない。

`anyvm_win/`は移植元の保管物であり、Go実装の規範入力ではない。仕様書が明示的に要求する比較・監査を除き、`anyvm_win`のsource、binary、生成script、version cacheから未記載挙動を推測または移植しない。不足が見つかった場合は仕様を先に改訂する。

## 3. 作業開始・停止・再開

実装と評価の単一進捗台帳は`docs/16-implementation-progress.md`とする。

作業開始時:

1. 進捗スナップショットと最新の停止・再開記録を読む。
2. 現在のbranch、commit、作業tree、OS、architecture、shell、Go versionを確認する。
3. 「次に開始するタスクID」と依存gateを確認する。
4. 同時に進行中とするタスクは1件だけにし、対象を`[-]`へ変更してスナップショットを更新する。
5. タスクの規範仕様と受入条件を読んでからファイルを変更する。

タスク完了時は、仕様で要求されたテストを実行し、command、結果、report pathまたは判断記録を「証跡」へ記載してから`[x]`にする。テスト未実施、失敗、証跡未記録のタスクを完了扱いにしない。

途中停止時は`docs/16-implementation-progress.md`の停止手順に従い、未完了タスクを`[ ]`へ戻し、完了済み部分、残作業、blocker、再開時の最初の具体操作を記録する。`[-]`のまま作業を終了しない。

## 4. 実装順序

- Windowsを先行する。W00～W12と`G-WIN-E2E`を完了する前に、Linux固有コード、Linux固有registry recipe、Linux E2Eへ進まない。
- Windows段階でもplatform-neutralなinterface、fake、fixture、testは実装してよい。
- Linux作業は`G-LINUX-START`合格後にL01から順に進める。
- cross-platform評価、release評価はそれぞれX01、R01のgateに従う。
- 依存未完了のタスクを、実装しやすさだけを理由に先行させない。

## 5. アーキテクチャ原則

- Goコードは`docs/02-architecture.md`と`docs/10-internal-api.md`の責務・依存方向に従う。
- コア機能は同一Go module内のlibraryとして実装し、CLIはflag/argument解析、型付きrequest変換、表示、prompt、終了code変換に限定する。
- CLIへdomain判断、path決定、TOML/state直接操作、network、展開、link、process、環境生成、security policyを置かない。
- filesystem、link、HTTP、process、archive、clock、lock、event等の外部作用はport経由にし、Application Serviceへ依存注入する。
- package global mutable stateを置かない。request/resultは境界を越えた後にimmutableとして扱う。
- 将来のWails v3 bridgeもCLIと同じlocale-neutralなApplication Service、request/result/error/event契約を使える構造を維持する。

## 6. 設定駆動・データ契約

- tool固有のversion発見、artifact選択、checksum、導入step、公開command、環境、依存、警告、probeをGoコードへhard-codeしない。`docs/06-tool-definition-schema.md`、`docs/12-standard-tools.md`、`docs/15-reference-definition.md`に従いTOMLで表現する。
- 標準定義は開発branchの`/registry/`で管理し、client release archiveへ同梱する。registry専用branch、単体download、単体updateを追加しない。
- 永続TOML/JSON/JSON Lines、receipt、catalog、Plan、helper、上限は`docs/03-storage-and-state.md`と`docs/14-data-contracts.md`に厳密に従う。
- parserは未知key、重複、型違い、上限超過を仕様どおり拒否する。寛容なfallbackや黙示変換を追加しない。
- tool versionは完全指定を基本とし、仕様で認めた`--latest`以外の部分版、range、wildcardを追加しない。

## 7. Platform・安全性

- Windows標準ユーザー、Linux非rootで完結させる。自動昇格、UAC要求、`sudo`、HKLM変更、system環境変数変更、system package自動導入を実装しない。
- Windowsの通常切替はdirectory junction、Linuxはrelative symlinkを使う。開発tool本体を切替のためにcopyしない。
- shimはGo製native resolverとし、外部`symexe.exe`へ依存しない。仕様で許可されたWindowsの小型fallback shimだけをcopy対象にできる。
- path containment、archive traversal、symlink/reparse point、case collision、archive bomb、command injection、credential漏えいをfail closedで扱う。
- 外部programはPlanで名称、完全版、取得元、digest、license、実行理由、argv要約、書込み先を表示し、検証前に実行しない。
- release clientの検証は公式GitHub repository identity、canonical `checksums.txt`、archive SHA-256、archive構造、binary version、registry tree hashに従う。Ed25519、client release用秘密鍵・公開鍵、署名manifestを再導入しない。
- 上流tool artifactのPGP/Minisign等はtool definitionが指定する別契約であり、client release検証と混同しない。

## 8. Go実装・コメント

- minimum toolchain、CGO、build metadata、version、再現可能buildは`docs/13-quality-and-release.md`に従う。
- production pathでpanicを通常のerror処理として使わない。
- 各packageに責務と依存範囲を説明するpackage documentation commentを置く。
- export宣言にはGo conventionに従うdocumentation commentを書く。
- domain invariant、security検査、transaction、rollback、並行制御、platform固有処理、非自明なalgorithmには、処理内容だけでなく理由を説明するコメントを書く。
- 自明な逐語コメント、コメントアウトした旧コード、追跡先のない`TODO`/`FIXME`を残さない。
- 仕様で設定駆動とされたtool固有動作をコメントだけで補完しない。

## 9. テストと検証

変更前に該当する受入条件を特定し、変更と同じ作業でテストを追加・更新する。

最低限:

- format、unit test、該当package test
- strict schema/parserのpositive/negative test
- fake clock/HTTP/process/filesystemによるdeterministic test
- failure injection、rollback、cancel、再開、並行実行
- Windows一般ユーザーのintegration/E2EをLinuxより先に実施
- security境界に対するnegative test
- CLIと内部APIのcontract一致
- 標準toolはregistry TOMLのcontract testで検証し、tool固有Go分岐を追加しない

network、特定OS、architecture、外部toolが必要で実行できないテストは、未実施理由と再現commandを報告して進捗を完了にしない。既存の失敗を隠すためにテストを削除、skip、弱体化しない。

## 10. 変更管理

- 作業開始前に既存差分を確認し、利用者の変更を保持する。依頼外のformatや大規模整理を混ぜない。
- 破壊的なGit操作、無関係ファイルの削除、公開済みartifactの上書きを行わない。
- observable behavior、CLI、schema、state、registry、security、release工程を変更する場合は、該当仕様、fixture、テスト、`docs/16-implementation-progress.md`を同じ変更で更新する。
- repository rootの`README.md`と`USER_GUIDE.md`は`docs/17-repository-documentation.md`に従って作成し、未実装機能を実装済みとして記載しない。
- secret、token、credential、個人home path、内部限定URLをsource、fixture、log、証跡へ保存しない。

## 11. 作業報告

最終報告は日本語で、次を簡潔に含める。

- 完了したタスクIDと成果
- 変更した主要ファイル
- 実行した検証commandと結果
- 未実施検証、既知の制約、blocker
- `docs/16-implementation-progress.md`に記録した次のタスクID

推測で完了を宣言せず、実際に確認できた事実と未確認事項を分ける。
