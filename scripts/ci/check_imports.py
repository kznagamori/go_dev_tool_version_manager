#!/usr/bin/env python3
"""依存方向の静的検査（docs/13-progress.md P1-01、`policy` job）。

docs/02-architecture.md §1 の依存方向を機械的に固定する。

    CLI adapter → Application Service → Domain / 抽象ポート ← Infrastructure adapter

§1・§2・§16から一意に決まる不変条件を常に検査し、それ以外のpackage間importは
下のALLOWED表へ明示的に登録したものだけを通す（fail closed）。

18の論理領域すべての依存関係を先に決め切ることは仕様から一意にできないため、
表は空から始める。importを増やすtaskが、その時点の仕様根拠とともに表へ追記する。
「後で決める」を黙って通さないためであり、表に無いimportは失敗する。

使い方:
    check_imports.py [--root <dir>]
"""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path

MODULE = "github.com/kznagamori/go_dev_tool_version_manager"

# package（module rootからの相対dir）ごとに許可するinternal import先。
#
# 値は許可するpackage dirの集合である。ここに無いinternal importは失敗する。
# 空集合は「現時点でinternal importを持たない」を意味する。
ALLOWED: dict[str, set[str]] = {
    "cmd/gdtvm": set(),
    # 02-architecture.md §4の`NewServices(build BuildInfo, ports Ports)`が
    # port.Portsを受け取るため、appはportをimportする（P1-03）。
    # fakeは`_test.go`からの注入だけに使う。production pathからのfake importは
    # 11-quality-and-ci.md §7.1に従いcheck_policy.pyが別途禁止する。
    # 11-quality-and-ci.md §7.2の書込み範囲記録wrapperと02-architecture.md §8手順5の
    # Execute時不変条件を、§2が同領域へ割り当てた「transaction境界」として実装する。
    # 封じ込め判定はrole付きpathとcanonical containmentを使うためdomainと
    # internal/securityを、記録するURLのmaskも同packageのmask規則を使う。
    # 規則を複製すると、変えたときに片方だけが古いままになる（P5-04）。
    "internal/app": {
        "internal/domain",
        "internal/domain/port",
        "internal/domain/port/fake",
        "internal/security",
    },
    # 02-architecture.md §2「配布元照会、版正規化、channel/lifecycle判定」。
    # 06-tool-definition.md §6.1・§6.3のchannel導出とlifecycle優先順位が、
    # domainのVersion/Channel/Lifecycle/Scalarと、definitionが読んだ
    # `lifecycle_overrides`・`lifecycle_map`を入力に取る（P3-02）。
    # 06-tool-definition.md §6.1の文書取得がHTTPClient portを使う（P3-03）。
    # §6.4の`W_LIFECYCLE_OVERRIDE_UNUSED`が04-storage-and-data.md §16.2の
    # ResultWarningを作るためinternal/progressを使う（P3-03の2本目）。
    # §15のcatalog JSONはinternal/storeがcodecを持つ型であり、catalog組立ては
    # その型をそのまま作る（P3-03の3本目）。storeはcatalogをimportしない。
    # fakeは`_test.go`からの注入だけに使う。fake upstream応答でversion sourceの
    # 取得と評価を決定的にtestするためで、production pathからのfake importは
    # 11-quality-and-ci.md §7.1に従いcheck_policy.pyが別途禁止する（P3-03）。
    # 04-storage-and-data.md §15のcatalog cache pathを§2が同領域へ割り当てた
    # 「カタログcache」の責務として組み立てるため、§6のlogical rootからのpath
    # 組立て（security.Join）を使う。path検査規則を複製すると、規則を変えたときに
    # 片方だけが古いままになる（P6-01）。
    "internal/catalog": {
        "internal/domain",
        "internal/domain/port",
        "internal/domain/port/fake",
        "internal/definition",
        "internal/progress",
        "internal/security",
        "internal/store",
    },
    # 04-storage-and-data.md §1のroot決定がmode/platform/path roleのdomain値と、
    # OS user lookupの結果（port.UserIdentity）を使うため（P2-01）。
    "internal/config": {"internal/domain", "internal/domain/port"},
    # 06-tool-definition.md §3のidentifier grammarがWindows予約device名を拒否し、
    # §4・§5がversion scheme/platform tuple/message ID/tool IDのdomain値を扱う。
    # 予約名listをinternal/securityから複製せず、02-architecture.md §2が同package
    # へ割り当てたpath検査を使う（P3-01）。
    "internal/definition": {"internal/domain", "internal/security"},
    "internal/doctor": set(),
    "internal/domain": set(),
    # LoggerのLogRecordとRandomのID byte長がdomain値を使うため
    # （02-architecture.md §4.1・§15、04-storage-and-data.md §7・§18、P1-04）。
    "internal/domain/port": {"internal/domain"},
    # fakeは対象interfaceを実装するため、portとそのsignatureが使うdomainをimportできる。
    "internal/domain/port/fake": {"internal/domain/port", "internal/domain"},
    # 02-architecture.md §2「ダウンロード、検証、安全展開、probe、receipt、
    # transaction」。downloadがHTTPClient/FileSystem portとdomainのdigest/path/
    # error値を扱い、05-configuration.md §3.5のstream処理へP5-02(1/2)の
    # StreamHasherと10-security.md §9.2のURL maskをinternal/securityから使う。
    # §10のprogress通知にinternal/progressのPhase/Unitを使う（P5-02）。
    # fakeは`_test.go`からの注入だけに使う。partial破棄とoffline判定を
    # 決定的にtestするためで、production pathからのfake importは
    # 11-quality-and-ci.md §7.1に従いcheck_policy.pyが別途禁止する。
    # 08-install-runtime.md §6の安全展開が、06-tool-definition.md §7.1の
    # archive形式（`zip|tar.gz`）と`strip_components`を入力に取る。形式enumを
    # installへ複製すると、値を変えたときに片方だけが古いままになる（P5-03）。
    "internal/install": {
        "internal/definition",
        "internal/domain",
        "internal/domain/port",
        "internal/domain/port/fake",
        "internal/progress",
        "internal/security",
    },
    "internal/message": set(),
    # 02-architecture.md §1がInfrastructure adapterへHTTPを含め、§4.1の
    # HTTPClientが「GET、HEAD、redirect、proxy、TLS、response limit」を担う。
    # 10-security.md §9.2のURL maskとheader判定をinternal/securityから使う。
    # mask規則を複製すると、規則を変えたときに片方だけが古いままになる（P5-01）。
    # fakeは`_test.go`からの注入だけに使う。04-storage-and-data.md §21のretry
    # backoff（1/2/4秒）をfake Clockで決定的にtestするためで、production pathからの
    # fake importは11-quality-and-ci.md §7.1に従いcheck_policy.pyが別途禁止する。
    "internal/platform": {
        "internal/domain/port",
        "internal/domain/port/fake",
        "internal/security",
    },
    # 02-architecture.md §2「型付き進捗、warning、cancel境界」。Progressと
    # ResultWarningがmessage ID、scalar、ID、tool/versionのdomain値を持つ（P1-04）。
    "internal/progress": {"internal/domain"},
    # 07-registry-and-tools.md §3のregistry manifestがtool ID、client version、
    # message ID、path roleのdomain値を扱う（P4-01）。
    # 04-storage-and-data.md §20のmessage catalogが「template内の秘密値展開」を
    # 拒否するため、10-security.md §9.2のmask対象名の判定をinternal/securityから
    # 使う。同じpatternをregistry側へ複製すると、mask規則を変えたときに片方だけ
    # 古いままになる（P4-02）。
    # 07-registry-and-tools.md §5のsource validationが、§7〜§10の表との一致を
    # 検査するためdefinitionをparseする。§13-11の「registry全体のID/alias/command
    # 衝突と07-registry-and-tools.md contract」はdefinition単体では判定できず、
    # registry全体を見る側の責務である（P4-02）。
    "internal/registry": {
        "internal/domain",
        "internal/security",
        "internal/definition",
    },
    "internal/runtime": set(),
    # 10-security.md §9.2のmaskをscalar parameterへ適用するため（P1-04）。
    "internal/security": {"internal/domain"},
    "internal/selection": set(),
    "internal/shell": set(),
    "internal/shim": set(),
    # 04-storage-and-data.md §7〜§18のcodecがdomainのID/digest/enum/path role/
    # scalarを扱い、§18のstructured logがport.LogRecordを直接serializeする（P2-04）。
    # 04-storage-and-data.md §4のatomic writeが公開fileのdigest照合へ内部SHA-256を
    # 使い、§12のlog rotationがlogical rootからのpath組み立てを使う。
    # 02-architecture.md §2が「内部SHA-256、path検査」をinternal/securityの責務とする（P2-05）。
    # fakeは`_test.go`からの注入だけに使う。§4のfailure injectionとrotationの
    # 並行testがfake FileSystemとInjectorを要する。production pathからのfake import
    # は11-quality-and-ci.md §7.1に従いcheck_policy.pyが別途禁止する（P2-05）。
    "internal/store": {
        "internal/domain",
        "internal/domain/port",
        "internal/domain/port/fake",
        "internal/security",
    },
}

# 表に関係なく常に成立させる不変条件。(判定関数, 説明) の組。
#
# 表はtaskごとに増えるが、これらは§1・§2が直接定めるため緩められない。
INVARIANTS: list[tuple[str, str]] = [
    (
        "domain-leaf",
        "internal/domain配下はinternal/domain配下しかimportできない"
        "（§1: 抽象ポートはcore側が所有し、DomainはCLI・具体的OS API・具体的HTTP clientを参照しない）",
    ),
    (
        "no-cmd-import",
        "どのpackageもcmd配下をimportできない（§1: CLI adapterが最外層）",
    ),
    (
        "app-not-platform",
        "internal/appはinternal/platformをimportできない"
        "（§1: Application Serviceは具体的OS APIを参照しない）",
    ),
]

IMPORT_BLOCK = re.compile(r"^import\s*\(\s*$")
IMPORT_LINE = re.compile(r'"([^"]+)"')
SINGLE_IMPORT = re.compile(r'^import\s+(?:[\w.]+\s+)?"([^"]+)"')


def force_utf8_stdio() -> None:
    """stdout/stderrをUTF-8へ固定する。

    Windowsのconsole既定encodingでは日本語のdiagnosticがUnicodeEncodeErrorになる。
    CI matrixは両OSで同じscriptを実行するため（docs/11-quality-and-ci.md §5.2）、
    platform差はruntime側で吸収する。
    """
    for stream in (sys.stdout, sys.stderr):
        reconfigure = getattr(stream, "reconfigure", None)
        if reconfigure is not None:
            reconfigure(encoding="utf-8", errors="backslashreplace")


def go_files(root: Path) -> list[Path]:
    """testdataを除く全Go fileを返す。

    `_test.go`も対象にする。testからのimportも依存方向を壊しうるためである。
    fake portのimportだけはtestに許すが、それはcheck_policy.pyが別途扱う。
    """
    files: list[Path] = []
    for path in sorted(root.rglob("*.go")):
        parts = path.relative_to(root).parts
        if "testdata" in parts:
            continue
        files.append(path)
    return files


def internal_imports(path: Path) -> list[tuple[int, str]]:
    """fileが持つ自module内importを (行番号, package dir) で返す。"""
    found: list[tuple[int, str]] = []
    in_block = False
    for number, line in enumerate(path.read_text(encoding="utf-8").splitlines(), start=1):
        stripped = line.strip()
        if in_block:
            if stripped == ")":
                in_block = False
                continue
            match = IMPORT_LINE.search(stripped)
            if match:
                found.append((number, match.group(1)))
            continue
        if IMPORT_BLOCK.match(stripped):
            in_block = True
            continue
        match = SINGLE_IMPORT.match(stripped)
        if match:
            found.append((number, match.group(1)))

    prefix = MODULE + "/"
    return [(n, imp[len(prefix) :]) for n, imp in found if imp.startswith(prefix)]


def check_invariants(pkg: str, target: str) -> str | None:
    if pkg.startswith("internal/domain") and not target.startswith("internal/domain"):
        return INVARIANTS[0][1]
    if target.startswith("cmd/"):
        return INVARIANTS[1][1]
    if pkg == "internal/app" and target.startswith("internal/platform"):
        return INVARIANTS[2][1]
    return None


def main() -> int:
    force_utf8_stdio()
    parser = argparse.ArgumentParser()
    parser.add_argument("--root", default=str(Path(__file__).resolve().parents[2]))
    args = parser.parse_args()
    root = Path(args.root).resolve()

    files = go_files(root)
    if not files:
        print("Go sourceがまだ無いため依存方向検査の対象は0件")
        return 0

    errors: list[str] = []
    packages: set[str] = set()
    edges = 0

    for path in files:
        rel = path.relative_to(root)
        pkg = rel.parent.as_posix()
        packages.add(pkg)
        if pkg not in ALLOWED:
            errors.append(
                f"{rel}: package `{pkg}` がcheck_imports.pyのALLOWED表に無い。"
                "package追加時は許可するinternal importを表へ明示する"
            )
            continue
        for number, target in internal_imports(path):
            edges += 1
            reason = check_invariants(pkg, target)
            if reason is not None:
                errors.append(f"{rel}:{number}: `{pkg}` → `{target}` は依存方向違反。{reason}")
                continue
            if target not in ALLOWED[pkg]:
                errors.append(
                    f"{rel}:{number}: `{pkg}` → `{target}` はALLOWED表に無い。"
                    "仕様根拠とともに表へ追記する"
                )

    print(f"検査したpackage数: {len(packages)} / internal import数: {edges}")
    if errors:
        for error in errors:
            print(f"ERROR {error}")
        print(f"依存方向検査に失敗した: {len(errors)}件")
        return 1
    print("依存方向検査に成功した: 不変条件3件とALLOWED表")
    return 0


if __name__ == "__main__":
    sys.exit(main())
