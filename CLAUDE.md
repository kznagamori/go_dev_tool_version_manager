# Claude Code 作業指示

## 1. 適用範囲

このファイルはrepository全体に適用する。`go_dev_tool_version_manager`（CLI名`gdtvm`）の実装、標準registry、テスト、公開文書、release工程は、必ず`docs/`以下の仕様書に従う。

セッション開始時は、最初に`docs/README.md`を全文読み、製品優先順位、v0.1のrelease段階、規範領域、固定された製品判断を確認する。次に`docs/13-progress.md`のスナップショット、最新停止記録、次タスクを読み、対象タスクに関係する番号付き仕様書を読む。会話要約、過去のmemory、このファイルだけで実装判断を完結させない。

## 2. 仕様の優先順位

1. 利用者から与えられた現在の指示
2. `docs/README.md`が定める規範領域と、該当する番号付き仕様書
3. 本ファイル
4. 実装済みコード、テスト、既存の慣例

仕様書間に矛盾がある、またはenum、既定値、上限、失敗時動作、platform差を一意に決められない場合は、実装を開始または継続しない。矛盾箇所と影響範囲を報告し、仕様、schema、テスト、進捗項目を先に同期修正する。libraryやOSの既定動作、既存実装、一般的慣例で仕様不足を補わない。

## 3. v0.1のscope

初期完成対象は**v0.1**である。これは機能scope・完成段階名であり、client version/tagではない。実versionは`docs/11-quality-and-ci.md`の`YYYY.MM.DD.XX`を使う。CLIは次の9 commandだけ。

```text
setup  available  install  installed  use  current  uninstall  doctor  version
```

`docs/15-deferred.md`に記録した延期機能（`self-update`、`use --system`、`exec`、`refresh`、`tools`、`disable`、`repair`、`completion`、再現可能build、operation journal、schema migration、Plan fingerprint、多言語ほか）を、予約key、未使用enum値、コメント、部分実装として先行導入しない。

延期機能を実装する要求が来たら、実装から始めず`docs/15-deferred.md`の該当節と§4のpromptを入口にする。同章の「v0.1での代替」で目的が達成できる場合は、その旨を報告して実装しない。

## 4. タスク管理

Claude Code内のTodo機能を使う場合も、永続する正本は`docs/13-progress.md`とする。Todo表示だけで進捗台帳を代替しない。

作業開始時:

1. branch、commit、作業tree、OS、architecture、shell、Go versionを確認する。
2. 進捗台帳の「次に開始するタスクID」と依存gateを確認する。
3. 同時に進行中とするタスクは1件だけにし、対象を`[-]`へ変更してスナップショットを更新する。
4. タスクの規範仕様、入力、完了条件、必要な証跡を確認する。
5. 調査、実装、検証、文書同期を小さなTodoへ分解してから変更する。

完了条件をすべて満たし、test command、結果、report pathまたは判断記録を証跡へ記載した場合だけ`[x]`にする。途中停止時は未完了タスクを`[ ]`へ戻し、停止記録と次の具体操作を残す。`[-]`のままセッションを終了しない。

### 4.1 branch・PR workflow

branch lifecycle、merge方式、protection、CI gate、release freezeの正本は`docs/11-quality-and-ci.md`§5.2～§5.6、§13とする。

- Claude Codeの通常taskは`claude/work`から`claude/feature-<task-id>-<slug>`を作って行う。`<task-id>`は`docs/13-progress.md`のIDを小文字化したexact値、`<slug>`は小文字英数字のkebab-case 1～48文字とする。例は`claude/feature-p6-02-install-plan`。
- 実装・文書・ローカル検証後、featureから`claude/work`へPRし、両OSの`lint`、`unit`、`policy`成功後にsquash mergeしてfeatureを削除する。Claude Code task群の統合後、`claude/work`から`develop/work`へPRし、両OSの全6 job成功後にmerge commitで統合する。
- `develop/work→main`もmerge commitだけを使う。agent work、develop、mainの間でsquash/rebase mergeを使わない。required approving reviewは0件だが、最新base、必須CI、未解決conversationなし、指定maintainerによるmergeを必須とする。
- `main`、`develop/work`、`claude/work`、`codex/work`へ通常のdirect push、force-push、削除を行わない。同期のrebase、`--force-with-lease`、削除・再作成は、利用者が指定したmaintainerのbranch lifecycle作業だけで行う。
- release PR作成から公開完了まで`claude/work→develop/work`をmergeしない。feature→`claude/work`は継続できる。
- 現在branchがこの規則と異なる場合、作業を開始せず利用者へ報告する。repository再作成時の初期登録だけは`docs/11-quality-and-ci.md`§5.6の一回限りの例外手順に従う。

## 5. 実装順序

- **WindowsとLinuxを同時に開発する。** OSごとの段階gateを設けない。
- platform差が出る箇所（link、PATH integration、user lookup、path規則、signal）は同じタスクの中で両OS分を実装する。
- 合格判定はCI matrix（`ubuntu-latest`＋`windows-latest`）だけで行う。片方のOSだけを回すworkflow分岐を作らない。
- 実機での手動確認はrelease blockerにしない。`docs/11-quality-and-ci.md`§9の利用者確認チェックリストとして扱い、実施状況と未確認項目を記録する。
- 依存未完了タスクを、実装しやすさだけを理由に先行させない。

## 6. アーキテクチャ原則

- `docs/02-architecture.md`の責務・依存方向を守る。
- コア機能は同一Go module内のlibraryとし、CLIはflag/argument解析、型付きrequest変換、表示、prompt、終了code変換だけを担当する。
- CLIへdomain判断、path決定、TOML/state直接操作、network、展開、link、process、環境生成、security policyを置かない。
- filesystem、link、registry、HTTP、process、archive、clock、lock、progress等の外部作用はport経由にし、Application Serviceへ依存注入する。この構造はCIの書込み範囲検査（`docs/11-quality-and-ci.md`§7.2）の前提でもあるため、portを迂回した直接呼出しを作らない。
- package global mutable stateを置かず、request/resultは境界通過後にimmutableとして扱う。

## 7. 設定駆動・データ契約

- tool固有のversion発見、artifact選択、checksum、展開parameter、公開command、環境、storage、警告、probeをGoへhard-codeしない。`docs/06-tool-definition.md`と`docs/07-registry-and-tools.md`に従いTOMLで表現する。
- 標準定義は開発branchの`/registry/`で管理し、client release archiveへ同梱する。registry専用branch、registry単体download、registry単体updateを追加しない。
- 永続TOML/JSON、receipt、catalog、Plan、上限は`docs/04-storage-and-data.md`へ厳密に合わせる。
- 標準toolはGo、Node.js、Python、.NET SDKだけとする。その他のtool、schema/config/platform追加と不具合調査・修正は`docs/14-maintenance.md`の手順とpromptを使う。
- parserは未知key、重複、型違い、上限超過を仕様どおり拒否し、黙示変換や寛容fallbackを追加しない。
- tool versionは完全指定を基本とし、仕様で認めた`--latest`以外の部分版、range、wildcardを追加しない。
- 未使用のenum値、kind、fieldを「将来のため」に残さない。使わないものは削除し、`docs/15-deferred.md`へ再導入gateとして記録する。

## 8. Platform・安全性

- Windows標準ユーザー、Linux非rootで完結させる。自動昇格、UAC要求、`sudo`、HKLM変更、system環境変数変更、system package自動導入を実装しない。これらはCIの静的検査（`docs/11-quality-and-ci.md`§7.1）で拒否される。
- Windowsの通常切替はdirectory junction、Linuxはrelative symlinkを使い、開発tool本体を切替のためにcopyしない。
- shimはGo製native resolverとする。仕様で許可されたWindowsの小型fallback resolverだけをclientへ内蔵できる。
- path containment、archive traversal、symlink/reparse point、case collision、archive bomb、command injection、credential漏えいをfail closedで扱う。
- 外部programはPlanで名称、完全版、取得元、digest、license、実行理由、argv要約、書込み先を表示し、検証前に起動しない。
- 標準toolのartifactはupstream checksumが公開されているものだけを採用し、providerが公開したalgorithm（`sha256`/`sha512`）での照合を必須にする。checksum非提供artifactの扱いは`docs/15-deferred.md` D-06のgateを先に通す。
- 公式配布物でもOSI承認OSS licenseでないplatformには`license_notice`を宣言し、Planの重要要約で明示承認を求める。.NET SDKのWindows配布物が該当する。
- 署名検証、artifact lock、専用security audit log、SBOM/provenance/attestationをv0.1へ導入しない。

## 9. Go実装・コメント

- minimum toolchain、CGO、build metadata、versionは`docs/11-quality-and-ci.md`に従う。
- production pathでpanicを通常のerror処理として使わない。
- 各packageに責務と依存範囲を説明するpackage documentation commentを置く。
- export宣言にはGo conventionに従うdocumentation commentを書く。
- domain invariant、security検査、transaction、rollback、並行制御、platform固有処理、非自明なalgorithmには、何をするかに加えて理由を書く。
- 自明な逐語コメント、コメントアウトした旧コード、追跡先のない`TODO`/`FIXME`を残さない。
- tool固有動作をコメントだけで補完せず、TOML定義と仕様へ記載する。

## 10. 調査・編集方針

- ファイル探索は対象を絞り、同じ大規模ファイルを繰り返し全文読込みしない。最初に見出し・参照を確認し、必要な節を読む。
- 変更前に既存差分を確認し、利用者の編集を保持する。依頼外のformat、大規模rename、整理を混ぜない。
- 一括置換は対象と件数を事前確認し、変更後に意図しない一致がないか再検索する。
- 生成途中file、debug出力、temporary scriptをrepositoryへ残さない。
- destructive Git操作、無関係ファイルの削除、公開済みartifactの上書きを行わない。
- secret、token、credential、個人home path、内部限定URLをsource、fixture、log、証跡へ保存しない。

## 11. テストと検証

変更前に該当する受入条件を特定し、変更と同じ作業でtestを追加・更新する。

最低限:

- format、unit test、該当package test
- strict schema/parserのpositive/negative test
- fake clock/HTTP/process/filesystemによるdeterministic test
- failure injection、rollback、cancel、再開、並行実行
- security境界に対するnegative test
- CLIと内部APIのcontract一致
- 標準toolはregistry TOMLのcontract testで検証し、tool固有Go分岐を追加しない
- CI matrixの両OS（`ubuntu-latest`／`windows-latest`）

network、特定OS、architecture、外部toolが必要で実行できない場合は、未実施理由と再現commandを報告し、進捗を完了にしない。既存の失敗を隠すためにtestを削除、skip、弱体化しない。

## 12. ドッグフーディング対応

v0.1は利用者が実際に使って問題を見つける前提で作る。不具合対応では次を守る。

- 利用者報告がある場合、`gdtvm doctor --report`の出力を一次資料として扱い、手元で再現する前に環境・導入状態・診断結果と症状の整合を確認する（`docs/14-maintenance.md`§5.2）。
- **症状が既存の診断項目に現れていない場合、修正と同じ変更で診断項目を追加できないか必ず検討する。** 次に同じ問題が起きたとき`doctor`だけで分かる状態へ近づけることが目的である。
- 仕様から期待値を一意に引用できない場合はコードで補わず、選択肢を1件ずつ確認する。

## 13. 仕様・文書同期

- observable behavior、CLI、schema、state、registry、security、release工程を変更する場合は、該当仕様、fixture、test、`docs/13-progress.md`を同じ変更で更新する。
- schema変更では互換性、unknown key、fixture、negative testを同時に扱う。
- repository rootの`README.md`と`USER_GUIDE.md`は`docs/12-public-docs.md`に従って作成し、未実装機能を実装済みとして記載しない。存在しないcommandを案内しない。
- 仕様変更が利用者判断を必要とする場合、独断で実装せず、推奨・他の選択肢・それぞれのメリット/デメリットを表で示し、日本語で1問ずつ確認する。

## 14. 作業報告

最終報告は日本語で、次を簡潔に含める。

- 完了したタスクIDと成果
- 変更した主要ファイル
- 実行した検証commandと結果
- 未実施検証、既知の制約、blocker
- `docs/13-progress.md`に記録した次のタスクID

実際に確認できた事実と推測を区別し、テストしていない内容を「動作確認済み」と表現しない。
