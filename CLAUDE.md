# Claude Code 作業指示

## 1. 適用範囲

このファイルはrepository全体に適用する。`go_dev_tool_version_manager`（CLI名`gdtvm`）の実装、標準registry、テスト、公開文書、release工程は、必ず`docs/`以下の仕様書に従う。

セッション開始時は、最初に`docs/README.md`を全文読み、規範領域、矛盾時の扱い、固定された製品判断を確認する。次に`docs/16-implementation-progress.md`のスナップショット、最新停止記録、次タスクを読み、対象タスクに関係する番号付き仕様書を読む。会話要約、過去のmemory、このファイルだけで実装判断を完結させない。

## 2. 仕様の優先順位

1. 利用者から与えられた現在の指示
2. `docs/README.md`が定める規範領域と、該当する番号付き仕様書
3. 本ファイル
4. 実装済みコード、テスト、既存の慣例

仕様書間に矛盾がある、またはenum、既定値、上限、失敗時動作、platform差を一意に決められない場合は、実装を開始または継続しない。矛盾箇所と影響範囲を報告し、仕様、schema、テスト、進捗項目を先に同期修正する。libraryやOSの既定動作、既存実装、一般的慣例で仕様不足を補わない。

`anyvm_win/`は移植元の保管物であり、Go実装の規範入力ではない。仕様書が明示的に要求する比較・監査を除き、`anyvm_win`のsource、binary、生成script、version cacheから未記載挙動を推測または移植しない。不足が見つかった場合は仕様を先に改訂する。

## 3. タスク管理

Claude Code内のTodo機能を使う場合も、永続する正本は`docs/16-implementation-progress.md`とする。Todo表示だけで進捗台帳を代替しない。

作業開始時:

1. branch、commit、作業tree、OS、architecture、shell、Go versionを確認する。
2. 進捗台帳の「次に開始するタスクID」と依存gateを確認する。
3. 同時に進行中とするタスクは1件だけにし、対象を`[-]`へ変更してスナップショットを更新する。
4. タスクの規範仕様、入力、完了条件、必要な証跡を確認する。
5. 調査、実装、検証、文書同期を小さなTodoへ分解してから変更する。

完了条件をすべて満たし、test command、結果、report pathまたは判断記録を証跡へ記載した場合だけ`[x]`にする。途中停止時は未完了タスクを`[ ]`へ戻し、停止記録と次の具体操作を残す。`[-]`のままセッションを終了しない。

## 4. 実装順序

- Windowsを先行する。W00～W12と`G-WIN-E2E`を完了する前に、Linux固有コード、Linux固有registry recipe、Linux E2Eへ進まない。
- Windows段階でもplatform-neutralなinterface、fake、fixture、testは実装してよい。
- Linux作業は`G-LINUX-START`合格後にL01から順に進める。
- cross-platform評価とrelease評価はX01、R01の順序とgateに従う。
- 依存未完了タスクを、実装しやすさだけを理由に先行させない。

## 5. アーキテクチャ原則

- `docs/02-architecture.md`と`docs/10-internal-api.md`の責務・依存方向を守る。
- コア機能は同一Go module内のlibraryとし、CLIはflag/argument解析、型付きrequest変換、表示、prompt、終了code変換だけを担当する。
- CLIへdomain判断、path決定、TOML/state直接操作、network、展開、link、process、環境生成、security policyを置かない。
- filesystem、link、HTTP、process、archive、clock、lock、event等の外部作用はport経由にし、Application Serviceへ依存注入する。
- package global mutable stateを置かず、request/resultは境界通過後にimmutableとして扱う。
- 将来のWails v3 bridgeもCLIと同じlocale-neutralなApplication Service、request/result/error/event契約を使用できる構造にする。

## 6. 設定駆動・データ契約

- tool固有のversion発見、artifact、checksum、導入step、公開command、環境、依存、警告、probeをGoへhard-codeしない。`docs/06-tool-definition-schema.md`、`docs/12-standard-tools.md`、`docs/15-reference-definition.md`に従いTOMLで表現する。
- 標準定義は開発branchの`/registry/`で管理し、client release archiveへ同梱する。registry専用branch、registry単体download、registry単体updateを追加しない。
- 永続TOML/JSON/JSON Lines、receipt、catalog、Plan、helper、上限は`docs/03-storage-and-state.md`と`docs/14-data-contracts.md`へ厳密に合わせる。
- parserは未知key、重複、型違い、上限超過を仕様どおり拒否し、黙示変換や寛容fallbackを追加しない。
- tool versionは完全指定を基本とし、仕様で認めた`--latest`以外の部分版、range、wildcardを追加しない。

## 7. Platform・安全性

- Windows標準ユーザー、Linux非rootで完結させる。自動昇格、UAC要求、`sudo`、HKLM変更、system環境変数変更、system package自動導入を実装しない。
- Windowsの通常切替はdirectory junction、Linuxはrelative symlinkを使い、開発tool本体を切替のためにcopyしない。
- shimはGo製native resolverとし、外部`symexe.exe`へ依存しない。仕様で許可されたWindowsの小型fallback shimだけをcopy対象にできる。
- path containment、archive traversal、symlink/reparse point、case collision、archive bomb、command injection、credential漏えいをfail closedで扱う。
- 外部programはPlanで名称、完全版、取得元、digest、license、実行理由、argv要約、書込み先を表示し、検証前に起動しない。
- release clientは公式GitHub repository identity、canonical `checksums.txt`、archive SHA-256、archive構造、binary version、registry tree hashで検証する。Ed25519、client release用秘密鍵・公開鍵、署名manifestを再導入しない。
- 上流tool artifactのPGP/Minisign等はtool definitionが指定する別契約であり、client release検証と混同しない。

## 8. Go実装・コメント

- minimum toolchain、CGO、build metadata、version、再現可能buildは`docs/13-quality-and-release.md`に従う。
- production pathでpanicを通常のerror処理として使わない。
- 各packageに責務と依存範囲を説明するpackage documentation commentを置く。
- export宣言にはGo conventionに従うdocumentation commentを書く。
- domain invariant、security検査、transaction、rollback、並行制御、platform固有処理、非自明なalgorithmには、何をするかに加えて理由を書く。
- 自明な逐語コメント、コメントアウトした旧コード、追跡先のない`TODO`/`FIXME`を残さない。
- tool固有動作をコメントだけで補完せず、TOML定義と仕様へ記載する。

## 9. 調査・編集方針

- ファイル探索は対象を絞り、同じ大規模ファイルを繰り返し全文読込みしない。最初に見出し・参照を確認し、必要な節を読む。
- 変更前に既存差分を確認し、利用者の編集を保持する。依頼外のformat、大規模rename、整理を混ぜない。
- 一括置換は対象と件数を事前確認し、変更後に意図しない一致がないか再検索する。
- 生成途中file、debug出力、temporary scriptをrepositoryへ残さない。
- destructive Git操作、無関係ファイルの削除、公開済みartifactの上書きを行わない。
- secret、token、credential、個人home path、内部限定URLをsource、fixture、log、証跡へ保存しない。

## 10. テストと検証

変更前に該当する受入条件を特定し、変更と同じ作業でtestを追加・更新する。

最低限:

- format、unit test、該当package test
- strict schema/parserのpositive/negative test
- fake clock/HTTP/process/filesystemによるdeterministic test
- failure injection、rollback、cancel、再開、並行実行
- Windows一般ユーザーのintegration/E2EをLinuxより先に実施
- security境界に対するnegative test
- CLIと内部APIのcontract一致
- 標準toolはregistry TOMLのcontract testで検証し、tool固有Go分岐を追加しない

長時間testは、目的、対象task、期待結果を明確にしてから実行する。network、特定OS、architecture、外部toolが必要で実行できない場合は、未実施理由と再現commandを報告し、進捗を完了にしない。既存の失敗を隠すためにtestを削除、skip、弱体化しない。

## 11. 仕様・文書同期

- observable behavior、CLI、schema、state、registry、security、release工程を変更する場合は、該当仕様、fixture、test、`docs/16-implementation-progress.md`を同じ変更で更新する。
- schema変更では互換性、migration、unknown key、fixture、negative testを同時に扱う。
- repository rootの`README.md`と`USER_GUIDE.md`は`docs/17-repository-documentation.md`に従って作成し、未実装機能を実装済みとして記載しない。
- 仕様変更が利用者判断を必要とする場合、独断で実装せず、選択肢、影響、推奨案を日本語で1問ずつ確認する。

## 12. 作業報告

最終報告は日本語で、次を簡潔に含める。

- 完了したタスクIDと成果
- 変更した主要ファイル
- 実行した検証commandと結果
- 未実施検証、既知の制約、blocker
- `docs/16-implementation-progress.md`に記録した次のタスクID

実際に確認できた事実と推測を区別し、テストしていない内容を「動作確認済み」と表現しない。
