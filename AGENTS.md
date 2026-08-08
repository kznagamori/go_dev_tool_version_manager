# Codex CLI 作業指示

## 1. 適用範囲

このファイルはrepository全体に適用する。`go_dev_tool_version_manager`（CLI名`gdtvm`）の実装、標準registry、テスト、公開文書、release工程は、必ず`docs/`以下の仕様書に従う。

作業開始時は最初に`docs/README.md`を全文読み、製品優先順位、v0.1のrelease段階、規範領域、固定された製品判断を確認する。その後、対象タスクに関係する番号付き仕様書と`docs/13-progress.md`を読む。要約や本ファイルは仕様書の代替ではない。

## 2. 仕様の優先順位

1. 利用者から与えられた現在の指示
2. `docs/README.md`が定める規範領域と、該当する番号付き仕様書
3. 本ファイル
4. 実装済みコード、テスト、既存の慣例

仕様書間に矛盾がある、またはenum、既定値、上限、失敗時動作、platform差を一意に決められない場合は、実装を開始または継続しない。矛盾箇所と影響範囲を報告し、仕様、schema、テスト、進捗項目を先に同期修正する。Go標準ライブラリ、外部library、OS、既存コードの暗黙既定値で仕様不足を補わない。

## 3. v0.1のscope

初期完成対象は**v0.1**である。これは機能scope・完成段階名であり、client version/tagではない。実versionは`docs/11-quality-and-ci.md`の`YYYY.MM.DD.XX`を使う。CLIは次の9 commandだけ。

```text
setup  available  install  installed  use  current  uninstall  doctor  version
```

`docs/15-deferred.md`に記録した延期機能を、予約key、未使用enum値、コメント、部分実装として先行導入しない。延期機能の実装要求は`docs/15-deferred.md`の該当節と§4のpromptを入口にする。同章の「v0.1での代替」で目的が達成できる場合は、その旨を報告して実装しない。

## 4. 作業開始・停止・再開

実装と評価の単一進捗台帳は`docs/13-progress.md`とする。

作業開始時:

1. 進捗スナップショットと最新の停止・再開記録を読む。
2. 現在のbranch、commit、作業tree、OS、architecture、shell、Go versionを確認する。
3. 「次に開始するタスクID」と依存gateを確認する。
4. 同時に進行中とするタスクは1件だけにし、対象を`[-]`へ変更してスナップショットを更新する。
5. タスクの規範仕様と受入条件を読んでからファイルを変更する。

タスク完了時は、仕様で要求されたテストを実行し、command、結果、report pathまたは判断記録を「証跡」へ記載してから`[x]`にする。テスト未実施、失敗、証跡未記録のタスクを完了扱いにしない。

途中停止時は`docs/13-progress.md`の停止手順に従い、未完了タスクを`[ ]`へ戻し、完了済み部分、残作業、blocker、再開時の最初の具体操作を記録する。`[-]`のまま作業を終了しない。

### 4.1 branch・PR workflow

branch lifecycle、merge方式、protection、CI gate、release freezeの正本は`docs/11-quality-and-ci.md`§5.2～§5.6、§13とする。

- Codexの通常taskは`codex/work`から`codex/feature-<task-id>-<slug>`を作って行う。`<task-id>`は`docs/13-progress.md`のIDを小文字化したexact値、`<slug>`は小文字英数字のkebab-case 1～48文字とする。例は`codex/feature-p6-02-install-plan`。
- 実装・文書・ローカル検証後、featureから`codex/work`へPRする。両OSの`lint`、`unit`、`policy`成功後のsquash mergeとfeature branch削除は指定maintainerが行う。Codex task群の統合後、`codex/work`から`develop/work`へPRし、両OSの全6 job成功後にmerge commitで統合する。
- **Codexはremote branchを削除しない。** 削除はfeature branchを含めすべて指定maintainerのbranch lifecycle作業とし、Codexは削除対象のbranch名と削除してよい根拠（mergeまたはPR close済み、固有commitなし）を報告するまでを担当する。
- `develop/work→main`もmerge commitだけを使う。agent work、develop、mainの間でsquash/rebase mergeを使わない。required approving reviewは0件だが、最新base、必須CI、未解決conversationなし、指定maintainerによるmergeを必須とする。
- `main`、`develop/work`、`claude/work`、`codex/work`へ通常のdirect push、force-push、削除を行わない。同期のrebase、`--force-with-lease`、削除・再作成は、利用者が指定したmaintainerのbranch lifecycle作業だけで行う。
- release PR作成から公開完了まで`codex/work→develop/work`をmergeしない。feature→`codex/work`は継続できる。
- 現在branchがこの規則と異なる場合、作業を開始せず利用者へ報告する。repository再作成時の初期登録だけは`docs/11-quality-and-ci.md`§5.6の一回限りの例外手順に従う。

## 5. 実装順序

- **WindowsとLinuxを同時に開発する。** OSごとの段階gateを設けない。
- platform差が出る箇所（link、PATH integration、user lookup、path規則、signal）は同じタスクの中で両OS分を実装する。
- 合格判定はCI matrix（`ubuntu-latest`＋`windows-latest`）だけで行う。片方のOSだけを回すworkflow分岐を作らない。
- 実機での手動確認はrelease blockerにしない。`docs/11-quality-and-ci.md`§9の利用者確認チェックリストとして扱う。
- 依存未完了のタスクを、実装しやすさだけを理由に先行させない。

## 6. アーキテクチャ原則

- Goコードは`docs/02-architecture.md`の責務・依存方向に従う。
- コア機能は同一Go module内のlibraryとして実装し、CLIはflag/argument解析、型付きrequest変換、表示、prompt、終了code変換に限定する。
- CLIへdomain判断、path決定、TOML/state直接操作、network、展開、link、process、環境生成、security policyを置かない。
- filesystem、link、registry、HTTP、process、archive、clock、lock、progress等の外部作用はport経由にし、Application Serviceへ依存注入する。この構造はCIの書込み範囲検査（`docs/11-quality-and-ci.md`§7.2）の前提でもあるため、portを迂回した直接呼出しを作らない。
- package global mutable stateを置かない。request/resultは境界を越えた後にimmutableとして扱う。

## 7. 設定駆動・データ契約

- tool固有のversion発見、artifact選択、checksum、導入、公開command、環境、警告、probeをGoコードへhard-codeしない。`docs/06-tool-definition.md`と`docs/07-registry-and-tools.md`に従いTOMLで表現する。
- 標準定義は開発branchの`/registry/`で管理し、client release archiveへ同梱する。registry専用branch、単体download、単体updateを追加しない。
- 永続TOML/JSON、receipt、catalog、Plan、上限は`docs/04-storage-and-data.md`に厳密に従う。
- 標準toolはGo、Node.js、Python、.NET SDKだけとする。その他のtool、schema/config/platform追加と不具合調査・修正は`docs/14-maintenance.md`の手順とpromptを使う。
- parserは未知key、重複、型違い、上限超過を仕様どおり拒否する。寛容なfallbackや黙示変換を追加しない。
- tool versionは完全指定を基本とし、仕様で認めた`--latest`以外の部分版、range、wildcardを追加しない。
- 未使用のenum値、kind、fieldを「将来のため」に残さない。使わないものは削除し、`docs/15-deferred.md`へ再導入gateとして記録する。

## 8. Platform・安全性

- Windows標準ユーザー、Linux非rootで完結させる。自動昇格、UAC要求、`sudo`、HKLM変更、system環境変数変更、system package自動導入を実装しない。これらはCIの静的検査（`docs/11-quality-and-ci.md`§7.1）で拒否される。
- Windowsの通常切替はdirectory junction、Linuxはrelative symlinkを使う。開発tool本体を切替のためにcopyしない。
- shimはGo製native resolverとする。仕様で許可されたWindowsの小型fallback resolverだけをclientへ内蔵できる。
- path containment、archive traversal、symlink/reparse point、case collision、archive bomb、command injection、credential漏えいをfail closedで扱う。
- 外部programはPlanで名称、完全版、取得元、digest、license、実行理由、argv要約、書込み先を表示し、検証前に実行しない。
- 標準toolのartifactはupstream checksumが公開されているものだけを採用し、providerが公開したalgorithm（`sha256`/`sha512`）での照合を必須にする。checksum非提供artifactの扱いは`docs/15-deferred.md` D-06のgateを先に通す。
- 公式配布物でもOSI承認OSS licenseでないplatformには`license_notice`を宣言し、Planの重要要約で明示承認を求める。.NET SDKのWindows配布物が該当する。
- 署名検証、artifact lock、専用security audit log、SBOM/provenance/attestationをv0.1へ導入しない。

## 9. Go実装・コメント

- minimum toolchain、CGO、build metadata、versionは`docs/11-quality-and-ci.md`に従う。
- production pathでpanicを通常のerror処理として使わない。
- 各packageに責務と依存範囲を説明するpackage documentation commentを置く。
- export宣言にはGo conventionに従うdocumentation commentを書く。
- domain invariant、security検査、transaction、rollback、並行制御、platform固有処理、非自明なalgorithmには、処理内容だけでなく理由を説明するコメントを書く。
- 自明な逐語コメント、コメントアウトした旧コード、追跡先のない`TODO`/`FIXME`を残さない。
- 仕様で設定駆動とされたtool固有動作をコメントだけで補完しない。

## 10. テストと検証

変更前に該当する受入条件を特定し、変更と同じ作業でテストを追加・更新する。

最低限:

- format、unit test、該当package test
- strict schema/parserのpositive/negative test
- fake clock/HTTP/process/filesystemによるdeterministic test
- failure injection、rollback、cancel、再開、並行実行
- security境界に対するnegative test
- CLIと内部APIのcontract一致
- 標準toolはregistry TOMLのcontract testで検証し、tool固有Go分岐を追加しない
- CI matrixの両OS（`ubuntu-latest`／`windows-latest`）

network、特定OS、architecture、外部toolが必要で実行できないテストは、未実施理由と再現commandを報告して進捗を完了にしない。既存の失敗を隠すためにテストを削除、skip、弱体化しない。

## 11. ドッグフーディング対応

v0.1は利用者が実際に使って問題を見つける前提で作る。不具合対応では次を守る。

- 利用者報告がある場合、`gdtvm doctor --report`の出力を一次資料として扱い、手元で再現する前に環境・導入状態・診断結果と症状の整合を確認する（`docs/14-maintenance.md`§5.2）。
- 症状が既存の診断項目に現れていない場合、修正と同じ変更で診断項目を追加できないか必ず検討する。
- 仕様から期待値を一意に引用できない場合はコードで補わず、選択肢を1件ずつ確認する。

## 12. 変更管理

- 作業開始前に既存差分を確認し、利用者の変更を保持する。依頼外のformatや大規模整理を混ぜない。
- 破壊的なGit操作、無関係ファイルの削除、公開済みartifactの上書きを行わない。
- observable behavior、CLI、schema、state、registry、security、release工程を変更する場合は、該当仕様、fixture、テスト、`docs/13-progress.md`を同じ変更で更新する。
- repository rootの`README.md`と`USER_GUIDE.md`は`docs/12-public-docs.md`に従って作成し、未実装機能を実装済みとして記載しない。存在しないcommandを案内しない。
- 仕様変更が利用者判断を必要とする場合、独断で実装せず、推奨・他の選択肢・それぞれのメリット/デメリットを表で示し、日本語で1問ずつ確認する。
- secret、token、credential、個人home path、内部限定URLをsource、fixture、log、証跡へ保存しない。

## 13. 作業報告

最終報告は日本語で、次を簡潔に含める。

- 完了したタスクIDと成果
- 変更した主要ファイル
- 実行した検証commandと結果
- 未実施検証、既知の制約、blocker
- `docs/13-progress.md`に記録した次のタスクID

推測で完了を宣言せず、実際に確認できた事実と未確認事項を分ける。
