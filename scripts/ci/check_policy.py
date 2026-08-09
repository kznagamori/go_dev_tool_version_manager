#!/usr/bin/env python3
"""禁止APIの静的検査（docs/11-quality-and-ci.md §7.1の`policy` job）。

GitHub Actionsの`windows-latest` runnerはAdministratorsグループに属するため、
「標準userでしか動かない」ことをOS権限で証明できない。代わりにproduction pathへ
昇格・system変更・package manager起動・TLS無効化のsymbolが存在しないことを
source走査で証明する（§7.1）。

command名は文字列literal内でだけ検査する。`adapt`のような識別子が`apt`にmatchして
偽陽性になるのを避けるためで、実際に外部programを起動するにはcommand名が
literalとして現れる必要がある。`InsecureSkipVerify`のようなAPI識別子は
literalに限らずsource全体で検査する。

使い方:
    check_policy.py [--root <dir>]
"""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path

# production pathから除外する。Goのtest fileとtestdataはtest資材である。
EXCLUDED_SUFFIX = "_test.go"
EXCLUDED_DIRS = {"testdata"}

# Goの文字列literal（interpreted、raw）。
STRING_LITERAL = re.compile(r'"(?:[^"\\\n]|\\.)*"' r"|`[^`]*`")

# 文字列literal内で禁止するcommand名。§7.1の昇格・system変更・package manager。
FORBIDDEN_COMMANDS = {
    "sudo": "昇格",
    "gsudo": "昇格",
    "pkexec": "昇格",
    "runas": "昇格（ShellExecuteのrunas verb）",
    "setx": "system環境変数変更",
    "reg.exe": "registry直接操作",
    "winget": "package manager起動",
    "choco": "package manager起動",
    "chocolatey": "package manager起動",
    "apt": "package manager起動",
    "apt-get": "package manager起動",
    "dnf": "package manager起動",
    "yum": "package manager起動",
    "pacman": "package manager起動",
}

# production pathからのimportを禁止するpackage。
#
# fakeはtest資材であり、productionへ混ざると決定的testのための細工が
# 実行時経路へ載る。import自体を静的に拒否する。
FORBIDDEN_IMPORTS = {
    "github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port/fake": (
        "fake portはtest専用（docs/11-quality-and-ci.md §6）"
    ),
}

# FORBIDDEN_IMPORTSの検査から除外するpath接頭辞。
# fake package自身のfileは当然fakeに属するため対象外にする。
IMPORT_CHECK_EXEMPT_PREFIXES = ("internal/domain/port/fake/",)

# source全体で禁止する識別子。
FORBIDDEN_SYMBOLS = {
    "InsecureSkipVerify": "TLS検証の無効化",
    "HKEY_LOCAL_MACHINE": "HKLM変更",
    "HKLM": "HKLM変更",
    "registry.LOCAL_MACHINE": "HKLM変更",
    "x509.NewCertPool": "独自CA bundle読込み",
    "SystemCertPool": "CA bundleの差し替え",
    "RootCAs": "独自CA bundleの適用",
}


def strip_comments(source: str) -> str:
    """Goのcommentを同じ長さの空白へ置換する。行構造は保つ。

    禁止symbolはcodeに存在してはならないものであり、commentでの言及は禁止しない。
    むしろ`CLAUDE.md`§9は「なぜ禁止なのか」をcommentへ書くことを求めるため、
    comment内の言及を検出すると、正しい説明を書くほど検査が落ちることになる。
    string literalとrune literalの中の`//`をcomment開始と誤認しないよう、
    literalの状態も追う。
    """
    out: list[str] = []
    index = 0
    length = len(source)
    while index < length:
        char = source[index]
        pair = source[index : index + 2]

        if pair == "//":
            while index < length and source[index] != "\n":
                out.append(" ")
                index += 1
            continue
        if pair == "/*":
            while index < length and source[index : index + 2] != "*/":
                out.append("\n" if source[index] == "\n" else " ")
                index += 1
            out.append("  ")
            index += 2
            continue
        if char in ('"', "'", "`"):
            quote = char
            out.append(char)
            index += 1
            while index < length:
                current = source[index]
                if quote != "`" and current == "\\":
                    out.append(source[index : index + 2])
                    index += 2
                    continue
                out.append(current)
                index += 1
                if current == quote:
                    break
                if quote != "`" and current == "\n":
                    break
            continue

        out.append(char)
        index += 1
    return "".join(out)


def production_go_files(root: Path) -> list[Path]:
    files: list[Path] = []
    for path in sorted(root.rglob("*.go")):
        if path.name.endswith(EXCLUDED_SUFFIX):
            continue
        if EXCLUDED_DIRS & set(path.relative_to(root).parts):
            continue
        files.append(path)
    return files


def scan(path: Path, rel: Path) -> list[str]:
    findings: list[str] = []
    rel_posix = rel.as_posix()
    import_exempt = rel_posix.startswith(IMPORT_CHECK_EXEMPT_PREFIXES)
    code = strip_comments(path.read_text(encoding="utf-8"))

    for number, line in enumerate(code.splitlines(), start=1):
        for symbol, reason in FORBIDDEN_SYMBOLS.items():
            if symbol in line:
                findings.append(f"{rel}:{number}: 禁止symbol `{symbol}` ({reason})")
        for literal in STRING_LITERAL.findall(line):
            body = literal[1:-1]
            if not import_exempt:
                for module, reason in FORBIDDEN_IMPORTS.items():
                    if body == module:
                        findings.append(f"{rel}:{number}: 禁止import `{module}` ({reason})")
            for command, reason in FORBIDDEN_COMMANDS.items():
                if re.search(rf"(?<![\w.-]){re.escape(command)}(?![\w-])", body):
                    findings.append(f"{rel}:{number}: 禁止command `{command}` ({reason})")
    return findings


def force_utf8_stdio() -> None:
    """stdout/stderrをUTF-8へ固定する。

    Windowsのconsole既定encoding（cp1252等）では日本語のdiagnosticが
    UnicodeEncodeErrorになり、検査そのものではなく出力で落ちる。CI matrixは
    両OSで同じscriptを実行するため（docs/11-quality-and-ci.md §5.2）、
    platform差はruntime側で吸収する。
    """
    for stream in (sys.stdout, sys.stderr):
        reconfigure = getattr(stream, "reconfigure", None)
        if reconfigure is not None:
            reconfigure(encoding="utf-8", errors="backslashreplace")


def main() -> int:
    force_utf8_stdio()
    parser = argparse.ArgumentParser()
    parser.add_argument("--root", default=str(Path(__file__).resolve().parents[2]))
    args = parser.parse_args()
    root = Path(args.root).resolve()

    files = production_go_files(root)
    findings: list[str] = []
    for path in files:
        findings.extend(scan(path, path.relative_to(root)))

    print(f"走査したproduction Go file数: {len(files)}")
    if not files:
        print(
            "Go sourceがまだ無いため検出対象は0件。"
            "P1-01でpackage骨格が入った時点から実質的な検査になる（docs/13-progress.md P1-01）"
        )
    if findings:
        for finding in findings:
            print(f"ERROR {finding}")
        print(f"禁止API検査に失敗した: {len(findings)}件")
        return 1
    print("禁止API検査に成功した: 昇格/system変更/package manager/TLS")
    return 0


if __name__ == "__main__":
    sys.exit(main())
