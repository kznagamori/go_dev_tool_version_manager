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
    "internal/app": {"internal/domain/port", "internal/domain/port/fake"},
    "internal/catalog": set(),
    "internal/config": set(),
    "internal/definition": set(),
    "internal/doctor": set(),
    "internal/domain": set(),
    "internal/domain/port": set(),
    # fakeは対象interfaceを実装するため、portだけをimportできる。
    "internal/domain/port/fake": {"internal/domain/port"},
    "internal/install": set(),
    "internal/message": set(),
    "internal/platform": set(),
    "internal/progress": set(),
    "internal/registry": set(),
    "internal/runtime": set(),
    "internal/security": set(),
    "internal/selection": set(),
    "internal/shell": set(),
    "internal/shim": set(),
    "internal/store": set(),
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
