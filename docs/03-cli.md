# CLI仕様

## 1. 基本構文

```text
gdtvm [global-options] <command> [command-options] [arguments]
```

command、option、正規tool IDはASCII lowercaseかつcase-sensitive。aliasは入力時だけ正規化し、結果には正規IDを返す。versionとpathを勝手にcase変換・Unicode正規化しない。

v0.1のcommandは次の9件だけとする。延期commandは[15-deferred.md](15-deferred.md)を正とする。

| command | 種別 |
|---|---|
| `setup` | 状態変更 |
| `available` | 読取り |
| `install` | 状態変更 |
| `installed` | 読取り |
| `use` | 状態変更 |
| `current` | 読取り |
| `uninstall` | 状態変更 |
| `doctor` | 読取り |
| `version` | 読取り |

## 2. global option

| option | 意味 |
|---|---|
| `-h, --help` | config/stateを読まずhelp表示 |
| `--version` | client完全版だけを1行表示 |
| `--json` | 読取り専用commandの完了時に単一JSONをstdoutへ出す |
| `--quiet` | success/progressを抑制。warning/errorは残す |
| `-v, --verbose` | Plan、HTTP、path解決の診断を増やす |
| `--no-color` | ANSIを使わない |
| `-y, --yes` | 通常確認へyes。third-party/EOL警告は表示する |
| `--non-interactive` | 入力待ち禁止 |
| `--offline` | network禁止 |
| `--home <path>` | user modeのdata rootをこの実行だけ上書き |
| `--mode portable\|user` | modeをこの実行だけ上書き |
| `--project-file <path>` | project探索をせず指定fileを使う |
| `--no-project` | project selectionを無視する |
| `--project-search-beyond-vcs-root` | Git境界を越えてproject fileを探索する |

global optionの正規位置はcommandより前。`--help`だけはcommand直後も許す。効果のないoptionを黙って無視せず`E_USAGE`とする。

`--json`は`available|installed|current|doctor|version`だけに許す。状態変更commandへ付けた場合は`E_USAGE`とし、機械判定には終了codeを使う。progress/logはstderr、最後のJSONだけstdout。

global `--version`は単独でだけ使い、command/他optionと併用せずclient完全版＋LFを返す。machine build情報は`gdtvm version --json`を使う。global helpは`gdtvm --help`または`gdtvm <command> --help`で、後者はそのcommandの他argument/optionと併用しない。help/versionはconfig/state/registry/networkを読まない。

`--home`とportable、`--project-file`と`--no-project`は排他。`--offline`はnetworkを使い得る`available|install|use|doctor`だけ。project系optionは`install --use|installed|use|current|uninstall|doctor`だけに許す。`--quiet`と`--json`は併用できる（JSONはstdout、progressはstderrで分離済み）。

## 3. command

### 3.1 `setup`

```text
gdtvm setup [--mode portable|user]
            [--path-integration user-path|shell-profile|none]
            [--shell bash|zsh|fish]
gdtvm setup --remove
```

data root、state、registry、shim、PATH/shell integrationを冪等に初期化する。

- Windowsは`user-path|none`だけを表示し、`user-path`を推奨既定とする。
- Linuxは`shell-profile|none`だけを表示し、`shell-profile`を推奨既定とする。`--shell`省略時は現在shellだけを対象にし、判定不能なら1件選ばせる。検出した全shellを一括変更しない。
- `--shell`はLinuxの`shell-profile`のときだけ許す。Windowsで指定した場合と、`none`と併用した場合は`E_USAGE`。
- [04-storage-and-data.md](04-storage-and-data.md)§16の`SetupPlan`でmode、旧/新root、filesystem能力とlink方式、shim、integration対象、backup、再起動要否をまとめて表示し、1回確認する。
- `--non-interactive`では`--path-integration`を必須とし、自動fallbackしない。
- `user-path`が安全検査に失敗した場合、変更せず`none`での再実行例を出す。
- mode変更は`SetupPlan.previous_*_root`と新root、および「dataを移動しない」ことをPlanへ示す。
- setup済みrootへの再実行は差分だけを適用する。portable rootのdirectoryを移動した場合は、`setup`を再実行してlink/shim/integrationを作り直す。
- `--remove`はPATH/shell integrationだけを除去し、tool、state、cacheを残す。

### 3.2 `available`

```text
gdtvm available <tool> [--channel stable|prerelease]
                       [--lifecycle supported|eol|unknown] [--refresh]
```

完全versionをversion降順で表示する。filter省略時は全channel/lifecycleを表示する。cacheなしでonlineならrefresh、offlineなら`E_CATALOG_MISSING`。近似版候補は生成しない。

`--refresh`はcatalog cacheをatomic置換する運用data更新であり、Plan/確認を要求しない。tool payload、selection、storage、config、setup stateを変更しない。

### 3.3 `install`

```text
gdtvm install <tool>@<version> [--use] [--project]
gdtvm install <tool> --latest [--use] [--project]
```

- `@version`は最後の`@`で分割し、空、部分版、range、wildcardを拒否する。
- `--latest`と`@version`は排他。stableかつlifecycleがEOLでない完全versionへ解決する。
- `--use`なしではselectionを変更しない。`--project`は`--use`必須。
- project fileがなければ作成pathを確認する。
- 正常な同一receiptがあればdownloadせずsuccess。
- Plan要約は完全version、platform、provider、checksum algorithm、license notice、channel、lifecycle、warning数を冒頭と確認直前に繰り返す。本文の詳細は省略しない。

### 3.4 `installed`

```text
gdtvm installed [tool]
```

完全version、platform、install時刻、health、receipt、provider、selection mark、disk sizeを表示する。破損/orphanを隠さない。

### 3.5 `use`

```text
gdtvm use <tool>@<version> [--project]
```

導入済みでhealthyなreceiptの完全versionだけを選ぶ。未導入で`runtime.auto_install_on_use=true`なら同じ完全versionのinstall Planを提示できる。別versionへ変えない。非対話で承認がなければ`E_VERSION_NOT_INSTALLED`。

### 3.6 `current`

```text
gdtvm current [tool] [--explain]
```

effective selection、`source=project|user|none`（[04-storage-and-data.md](04-storage-and-data.md)§17.1）、完全version、選択receiptのpayload path、healthを表示する。toolが複数commandを持つため単一command pathをprimaryとして選ばない。`--explain`は探索project、候補、PATH resolution、inactive理由を追加する。

### 3.7 `uninstall`

```text
gdtvm uninstall <tool>@<version> [--force] [--purge-shared]
```

現在platformの一致receiptだけを対象にする。project/user選択が参照していれば拒否する。`--force`は列挙済みselectionだけを解除でき、未知projectの参照は回避できない。`--purge-shared`は最後のversionかつ参照なしの場合だけ、同じuninstall Planへtool-scope storageとdestructive warningを追加し、1回の確認後に削除する。既定は共有領域を保持する。

### 3.8 `doctor`

```text
gdtvm doctor [--tool <tool>] [--report <path>]
```

read-only。config/state/registry、root、lock、receipt、payload、link、shim、PATH順位、storage、disk、catalogを項目別に診断する。

`--report <path>`はMarkdown 1 fileを出力する。内容と除去規則は[10-security.md](10-security.md)§9を正とする。出力先fileが既に存在する場合は上書き前に確認する。`--report`と`--json`は併用でき、JSONには生成したreport pathを含める。

全体statusは`healthy|degraded|unhealthy`。[04-storage-and-data.md](04-storage-and-data.md)§17.1の10診断項目を集約し、errorがあればunhealthy、errorなしでwarnがあればdegraded、それ以外をhealthyとする。診断処理自体が完了した場合、unhealthyでもJSONは成功envelopeと全診断結果を返す。

### 3.9 `version`

```text
gdtvm version [--short]
```

client version/build identity、OS/arch、state/definition/registry schemaを返す。config/state/networkを読まない。`--short`はversionだけ。

## 4. version入力

完全versionはcatalog正規文字列との完全一致。前後空白、Unicode類似記号、leading `v`の暗黙除去、zero padding変更、case変換をしない。invalid/not-found時は入力値、理由、`gdtvm available <tool>`だけを案内し、近似候補や自動修正を行わない。

## 5. 対話

- 全変更operationはPlan後にdefault Noで1回確認する。`PlanWarningCode`は[04-storage-and-data.md](04-storage-and-data.md)§16.1のexactly 8件（third-party、制限的license、prerelease、EOL、destructive、shell/PATH変更、mode変更、要再起動）とする。
- `--yes`は明示承認が必要な7件すべてを承認できる。警告表示は消さない。
- checksum mismatch、archive安全違反、root逸脱は`--yes|--force`で回避不可。
- Plan作成後、承認直前とlock取得後に入力のrevision/digestを再検査し、変化時は`E_PLAN_STALE`。
- password/tokenをpromptしない。

## 6. stdout/stderrとprogress

- stdout: success text、一覧、単一JSON。
- stderr: progress、prompt、warning、error、diagnostic log。
- TTYでは1行progress barまたは複数段階表示を更新し、非TTYでは開始・25%単位・完了・段階変更だけを行単位で出す。
- downloadはbytes/total、percent、速度、残り時間（計算可能時）、artifact名を表示する。extract/verify/probe/commitは現在段階と対象を表示する。
- `--quiet`でもwarning/errorは残す。`--json`のstdoutへANSI/人向けprogressを混ぜない。

## 7. 終了code

| code | 意味 | 含むerror code |
|---:|---|---|
| 0 | success（変更不要を含む） | — |
| 1 | internal/unknown | `E_INTERNAL` |
| 2 | 使い方 | `E_USAGE` |
| 3 | 設定 | `E_CONFIG_INVALID`, `E_PROJECT_CONFIG_INVALID` |
| 4 | 対象不明・不正 | `E_TOOL_UNKNOWN`, `E_VERSION_INVALID`, `E_VERSION_NOT_FOUND`, `E_VERSION_NOT_INSTALLED`, `E_PLATFORM_UNSUPPORTED`, `E_UNSUPPORTED_SHELL` |
| 5 | network | `E_NETWORK`, `E_OFFLINE`, `E_CATALOG_MISSING` |
| 6 | 完全性 | `E_CHECKSUM_MISMATCH`, `E_REGISTRY_INVALID`, `E_DEFINITION_INVALID`, `E_ARCHIVE_UNSAFE`, `E_PROBE_FAILED` |
| 7 | filesystem/権限/path | `E_PERMISSION`, `E_FILESYSTEM`, `E_PATH_CONFLICT`, `E_PATH_UNSAFE`, `E_LINK_FAILED`, `E_SHELL_PROFILE_CONFLICT`, `E_PATH_INTEGRATION_FAILED` |
| 8 | 競合 | `E_LOCK_TIMEOUT`, `E_PLAN_STALE`, `E_STATE_CONFLICT`, `E_CONFLICT` |
| 9 | 状態破損 | `E_STATE_CORRUPT`, `E_RECEIPT_INVALID` |
| 10 | 中断 | `E_CANCELLED` |
| 11 | 承認 | `E_APPROVAL_REQUIRED`, `E_APPROVAL_DENIED` |
| 12 | doctorがunhealthy | gdtvm errorなし。成功した診断結果の`status=unhealthy` |

exit 0はdoctorの`healthy|degraded`を含む。exit 12は診断operationの失敗ではないためerror objectを作らず、human/JSONへ全診断結果を返す。その他の行はgdtvm自身のerrorに適用し、全34 error codeはexit 1～11のexactly 1件へmapする。

shimでchildを正常起動できた後のnonzero exitはgdtvm errorへ変換せず、child exit statusをplatformで表現可能な範囲でそのまま返す。signalも[09-platform.md](09-platform.md)どおり透過する。起動前失敗は上表のstable errorを使う。

JSON errorは[04-storage-and-data.md](04-storage-and-data.md)どおりstable `code`, `message_id`, scalar `parameters`, `retryable`だけを持つ。表示済みmessage、remediation、Go causeを機械JSONへ追加せず、human表示はmessage catalogから安全な対処を生成する。
