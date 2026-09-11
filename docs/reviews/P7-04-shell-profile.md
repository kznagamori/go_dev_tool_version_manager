# P7-04 決定記録（1/2）: shell profile統合

対象タスク: `docs/13-progress.md` P7-04の1本目。規範仕様は[09-platform.md](../09-platform.md)§2.2・§5.2・§7・§9、[04-storage-and-data.md](../04-storage-and-data.md)§9・§16.1・§17.1。

## 1. 着手時の確認事項

2件とも**仕様から一意に決まった**ため、利用者判断を求めていない。

### 1.1 `Registry` portのproduction実装は2本目で作る

§4.1は`Registry` portを「Windows HKCU valueのraw/type読書き、再読、通知」と定めるが、production実装がまだ無い。

**仕様はproduction adapterをtaskへ割り当てていない。** ただしP5-01（HTTP client）、P5-04（ProcessRunner）、P7-01（LinkManager）はいずれも**それを必要とするtaskが`internal/platform`へ実装する**形で進んでおり、§1が同packageをInfrastructure adapterと定めることとも一致する。Registryを要するのは§4.2のuser PATH統合だけなので、**2本目で作る**。

testは実の`HKCU\Environment\Path`へ触れない。CI runnerの環境を書き換えるためで、scratch keyで行う。

### 1.2 24,576の単位はUTF-16 code unit

§4.2「追加後のPATHは**32,767 UTF-16 code unit未満**かつ[04-storage-and-data.md](../04-storage-and-data.md)§21のより小さい安全上限内でなければ変更しない」、§21「Windows user PATH | **24,576 UTF-16 code unit**」。

byte数でも文字数でもない。**非BMP文字は2 code unitと数える。** 超過時のcodeは§9の`E_PATH_INTEGRATION_FAILED`である（同codeの条件が「registry PATHの型、**長さ**、競合、再読不一致」と長さを明示している）。

この判断は2本目で使うが、確認事項として先に固定した。

## 2. 分割

P7-04はOSごとに独立している。Linuxは既存portだけで完結し、WindowsはRegistry portのproduction実装を要する。片方ずつ完結するため**2 PRへ分割**する。

| 本 | 内容 |
|---|---|
| 1（本PR） | §5.2のshell profile統合（bash/zsh/fish） |
| 2 | §4.2のuser PATH統合7手順と`Registry` portのproduction実装 |

## 3. 判断

### 3.1 marker文字列と本文を仕様の字句どおりに固定する

§5.2はblockの中身を字句で示している。marker文字列や本文が変わると、**既存profileのblockを自分のものと認識できなくなる**。設定で変えられる余地を作らず、定数とtestで固定した。

### 3.2 single quoteで囲み、quote自体を各shellの規則でescapeする

§5.2「literalを各shell規則でescapeし、**command substitutionを生成しない**」。

POSIX shell（bash/zsh）のsingle quote内は**いかなるescapeも効かない**ため、single quoteそのものは「quoteを閉じる→backslashでescapeしたquote→quoteを開き直す」の4文字へ置き換える。fishのsingle quote内はbackslashとsingle quoteだけがescape可能であり、その2文字をbackslashで前置する。

**どちらの規則も、囲みの外へ出る文字を作らない。** 出せると、そこから先がcommandとして解釈される。

改行とNULは埋め込まず`ErrUnsafeLiteral`で拒否する。改行はblockの行構造を壊してmarkerの対応が取れなくなり、NULはshell fileへ書けない。

### 3.3 利用者fileをsource/evaluateしない

§5.2の明文である。marker行の一致だけで判定する。shellに読ませて判断すると、**profileに書かれた任意のcommandをgdtvmが実行する**ことになる。

### 3.4 markerが不完全・複数なら自動修正しない

§5.2「marker 0件なら末尾へ追加、完全一致1件ならno-op、**不完全/複数なら自動修正せず診断する**」。状態を4値（`absent`／`matches`／`differs`／`conflict`）に分け、`conflict`は理由付きで止める。

`--remove`も同じ判定を使う。**所有を証明できない状態では消さない** —— 利用者が手で書いたblockを巻き込む。

### 3.5 一致していれば書き直さない

書き直すとmtimeだけが変わり、利用者の差分に意味のない変更が出る。§7の冪等性とも一致する。

### 3.6 markerの外は1行も触らない

追加時は末尾へ足し、直前が空行でなければ1行空ける（既存の最終行へ連結すると、その行の意味が変わる）。差替え・除去ではmarker行の間だけを入れ替える。

### 3.7 内容の変換だけを行い、書き込まない

§5.2は「owner、permission、symlink、before digestを検査し、backup→temp→flush→atomic replaceを行う」と定める。**この順序はP7-03(2/2)の`RunTransaction`が持つ段階**であり、本PRは各段階へ渡す内容を作る純粋な変換に徹する。

こうするとmarker判定とescapeを実fileなしで網羅でき、書込み順の検査とescapeの検査が互いに干渉しない。

### 3.8 errorへ安全な代替を含める

§5.2「`E_UNSUPPORTED_SHELL`とし、**`none`を案内する**」、§9「errorには…利用者が選べる安全な代替を含める」。対象shell名と`none`の両方をerrorへ載せる。

## 4. 検査が固定したこと

`internal/shell`で34 caseを追加した。

| 検査 | 対象 |
|---|---|
| `TestProfilePathMatchesSpec` | §5.2の3 file、末尾separator、未対応shell、home未設定 |
| `TestRenderBlockMatchesSpecLiterally` | **§5.2の字句そのもの**（POSIX／fish） |
| `TestRenderBlockNeverEscapesTheQuoting` | **11種の敵対的literal × 3 shell**。literalが元へ戻ること、**囲みの外へ何も漏れないこと** |
| `TestRenderBlockRejectsUnsafeLiteral` | 改行・復帰・NUL・空・未対応shell |
| `TestInspectProfileClassifiesMarkerState` | 8 case（absent／matches／differs／開始だけ／終了だけ／2組／順序逆） |
| `TestApplyBlockFollowsMarkerRules` | 0件→追加、改行なしfile、完全一致→no-op、差替えでmarker外を保つ、不完全→拒否 |
| `TestApplyBlockRoundTripsWithRemove` | 4 caseの往復で**利用者の行が失われないこと** |
| `TestRemoveBlockLeavesForeignContentAlone` | markerなし→no-op、不完全→消さない |
| `TestErrorsCarryTheSafeAlternative` | errorが対象と`none`を名指しすること |

### 4.1 変異test

7件入れ、**当初2件が生き残った**。testを直したうえで、いずれも落ちるようにした。

| 変異 | 結果 |
|---|---|
| POSIXのquote escapeを省く | **当初は生き残り** → 落ちた |
| fishのbackslash escapeを省く | **当初は生き残り** → 落ちた |
| fishのquote escapeを省く | 落ちた（追加） |
| 改行を含むliteralを通す | 落ちた |
| marker複数でも自動修正する | 落ちた |
| 一致していても書き直す | 落ちた |
| 不完全なmarkerでも除去する | 落ちた |
| errorへ安全な代替を含めない | 落ちた |

### 4.2 生き残った2件が示したtestの欠陥

**escapeを省く変異が最初は捕まらなかった。** 原因はtestの作りである。

当初のtestはescapeを解く関数を自分で書き、「埋めた値が往復で戻るか」を見ていた。ところが**escapeを一切しない実装でも、解く側が何も戻さないため往復が成立する**。`strings.ReplaceAll(inner, "'\\''", "'")`は、そのpatternが無ければ入力をそのまま返す。つまりこのtestは、production側とtest側が同じ誤りをしていても通る。

**往復では確かめられない性質だった。** 直したtestはshell自身のlexerを模して**描画結果を走査**し、literalがどこで終わるかを見る。escapeが効いていなければ、literalの外に利用者の文字列の続きが現れる。`/root/'; rm -rf ~; echo '`を埋めた場合、囲みの外へ`; rm -rf ~; echo ''`が漏れることを直接検出する。

scannerはproduction側の実装を参照しない。参照すると、実装の誤りをそのまま正解として扱うことになる。

## 5. 検証

Linux containerで実行した（Go 1.26.6）。両OSの判定はCI matrixで行う。

| command | 結果 |
|---|---|
| `gofmt -l .` / `go build ./...` / `GOOS=windows go build ./...` | 出力なし・成功 |
| `go vet ./...` / `GOOS=windows go vet ./...` | 成功 |
| `GOOS=windows go test -c` | 成功 |
| `go test ./... -race -shuffle=on -covermode=atomic` | 全package成功。`internal/shell` 92.7% |
| `check_policy.py` / `check_imports.py` / `check_docs.py` / `check_licenses.py` / `check_messages.py` | すべて成功 |
| `git diff --check` | 出力なし |

`allowedGlobals`へ4件を登録した。`internal/shell`のimport表はP7-03から変えていない。

**gofmtのdoc comment整形が、コメント中の`'\''`とbackquoteを組版用のquoteへ変換した。** 規則の説明が誤りになるため、該当箇所を字句を直接書かない表現へ改めた。

## 6. 未実施・制約

- **profileへ実際に書き込んでいない。** §5.2の「owner、permission、symlink、before digestを検査し、backup→temp→flush→atomic replace」は、P7-03(2/2)の`RunTransaction`の段階として呼出し側が組み立てる。本PRはその段階へ渡す**内容**を作るところまでである。実fileへの書込みと検査は`port.FileSystem`のproduction実装（P8-01）と合成後のE2E（[11-quality-and-ci.md](../11-quality-and-ci.md)§8）で確かめる。
- **現在shellの判定を実装していない。** §5.2「現在shellだけを対象とし、判定不能ならbash/zsh/fishの1件を選ばせる」の判定は`port.Environment`（`SHELL`等）と対話に依存する。前者のproduction実装はP8-01、対話はP8-04（CLI adapter）の範囲である。本PRは判定済みのshellを受け取る形にしてある。
- **Windows側（§4.2）は2本目。** user PATHの7手順、`Registry` portのproduction実装、24,576 UTF-16 code unitの上限、`WM_SETTINGCHANGE`通知と`W_ENVIRONMENT_NOTIFICATION_FAILED`は本PRに含まない。
- **`E_SHELL_PROFILE_CONFLICT`と`E_UNSUPPORTED_SHELL`にmessage IDを付けていない。** `internal/catalog`の`E_PLATFORM_UNSUPPORTED`と同じく、表示文はP8-05のpresentation層が生成する。
- **P7-03から継続**: 6段階の中身は`Action`として呼出し側が渡す。capability probeを実filesystemで走らせていない。
- **P7-02から継続**: §10手順2〜6と`cmd/gdtvm`の分岐はP8-03／P8-04の範囲。
- **P7-01から継続**: junction経路のsyscallは`windows-latest`だけが動かす（PR #145で確認済み）。
- **P6-03から継続する未実装**: 合成側の`InstallEngine` adapter、`app.Guard`を噛ませた経路のE2E照合（§7.2）、receipt indexの再構築、`port.FileSystem`／`port.Environment`のproduction実装（いずれもP8-01）。
- **P6-02から継続する食い違いが1件**: `internal/store`のtemplate grammarが`internal/definition`と一致しない。fail closedは保たれ、正当なdefinitionからは生じない値である。§2の責務表を要する判断であり未着手。
- **P5-03から継続する未決が1件**（`./`始まりのtar entryを[10-security.md](../10-security.md)§5に従って拒否している）。**P6-01で埋めた仕様の空白が1件継続**（exact指定で`installable=false`のときの`E_PLATFORM_UNSUPPORTED`）。**仕様側の未決が2件継続**（[07-registry-and-tools.md](../07-registry-and-tools.md)§5第2項のlicense file size上限が§21の表に無い、§2の「license file名はASCII kebab grammar」が§2自身の例と食い違う）。`python.toml`の`lifecycle = "unknown"`はP3-04から継続。source error専用のerror code、version数値要素の64 bit上限、`logs/` file名規約と§11「専用lock」の解釈は未決である。
