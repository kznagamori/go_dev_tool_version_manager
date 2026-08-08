#!/usr/bin/env python3
"""Markdown文書の構造検査（docs/11-quality-and-ci.md §5 `lint` jobの文書link/example検査）。

検査するのは次の4件で、いずれも仕様書間の相互参照と表が壊れていないことを保証する。

1. 相対linkのfile実在。仕様は16文書が相互linkする構成のため、file renameで参照が
   切れるとどの文書が正かを追えなくなる。
2. anchorの実在。章番号で参照し合うため、見出し変更でanchorだけが古くなる事故を防ぐ。
3. code fenceの対応。fenceが片方だけ残るとそれ以降の全体がcode blockとして描画され、
   仕様本文が読めなくなる。
4. tableの列数一致。GFMはinline code内の`|`もcell区切りとして解釈するため、
   `\\|`へescapeしないとcellが割れる。過去に停止・再開記録で実際に発生した。

markdown linkの検出前にinline code spanを除去する。CalVer grammarのような正規表現
（`(0[1-9]|1[0-2])`）が`[..](..)`形にmatchし、link誤検出になるためである。
"""

from __future__ import annotations

import re
import sys
import unicodedata
from pathlib import Path

# 検査対象。repository rootからの相対path。
TARGET_GLOBS = ("docs/**/*.md", "*.md")

CODE_SPAN = re.compile(r"`[^`]*`")
FENCE = re.compile(r"^\s*(```|~~~)")
LINK = re.compile(r"\[[^\]]*\]\(([^)\s]+)\)")
TABLE_CELL_SEP = re.compile(r"(?<!\\)\|")


def strip_code_spans(text: str) -> str:
    """inline code spanを同じ長さの空白へ置換する。

    除去ではなく同幅置換にするのは、後段でcolumn位置を報告に使えるようにするため。
    """
    return CODE_SPAN.sub(lambda m: " " * len(m.group(0)), text)


def heading_anchors(lines: list[str]) -> set[str]:
    """GitHubのheading anchor生成規則に合わせてanchor集合を作る。

    小文字化し、英数字・hyphen・underscore・空白以外を落とし、空白をhyphenへ変換する。
    同名見出しには`-1`, `-2`の連番が付く。日本語見出しはそのまま残る。
    """
    anchors: set[str] = set()
    seen: dict[str, int] = {}
    in_fence = False
    for line in lines:
        if FENCE.match(line):
            in_fence = not in_fence
            continue
        if in_fence or not line.startswith("#"):
            continue
        text = line.lstrip("#").strip()
        text = CODE_SPAN.sub(lambda m: m.group(0).strip("`"), text)
        slug = unicodedata.normalize("NFC", text).lower()
        slug = re.sub(r"[^\w\s-]", "", slug, flags=re.UNICODE)
        slug = re.sub(r"\s+", "-", slug.strip())
        count = seen.get(slug, 0)
        seen[slug] = count + 1
        anchors.add(slug if count == 0 else f"{slug}-{count}")
    return anchors


def check_file(path: Path, root: Path, anchors_cache: dict[Path, set[str]]) -> list[str]:
    errors: list[str] = []
    raw = path.read_text(encoding="utf-8")
    lines = raw.splitlines()

    # 3. code fenceの対応
    fence_count = sum(1 for line in lines if FENCE.match(line))
    if fence_count % 2 != 0:
        errors.append(f"{path}: code fence marker数が奇数 ({fence_count})。閉じていないfenceがある")

    in_fence = False
    header_cols: int | None = None
    for number, line in enumerate(lines, start=1):
        if FENCE.match(line):
            in_fence = not in_fence
            header_cols = None
            continue
        if in_fence:
            continue

        # 4. tableの列数一致
        if line.lstrip().startswith("|"):
            cols = len(TABLE_CELL_SEP.split(line.strip().strip("|")))
            if header_cols is None:
                header_cols = cols
            elif cols != header_cols:
                errors.append(
                    f"{path}:{number}: table列数が{cols}でheaderの{header_cols}と不一致。"
                    "inline code内の`|`は`\\|`へescapeする"
                )
        else:
            header_cols = None

        # 1, 2. linkとanchor
        for match in LINK.finditer(strip_code_spans(line)):
            target = match.group(1)
            if target.startswith(("http://", "https://", "mailto:")):
                continue
            file_part, _, anchor = target.partition("#")
            if file_part:
                resolved = (path.parent / file_part).resolve()
                if not resolved.exists():
                    errors.append(f"{path}:{number}: link先が存在しない -> {target}")
                    continue
            else:
                resolved = path.resolve()
            if anchor:
                if resolved not in anchors_cache:
                    anchors_cache[resolved] = heading_anchors(
                        resolved.read_text(encoding="utf-8").splitlines()
                    )
                if anchor.lower() not in anchors_cache[resolved]:
                    errors.append(f"{path}:{number}: anchorが存在しない -> {target}")
    return errors


def main() -> int:
    root = Path(__file__).resolve().parents[2]
    paths: list[Path] = []
    for pattern in TARGET_GLOBS:
        paths.extend(sorted(root.glob(pattern)))

    anchors_cache: dict[Path, set[str]] = {}
    errors: list[str] = []
    for path in paths:
        errors.extend(check_file(path.relative_to(root), root, anchors_cache))

    print(f"検査file数: {len(paths)}")
    if errors:
        for error in errors:
            print(f"ERROR {error}")
        print(f"文書検査に失敗した: {len(errors)}件")
        return 1
    print("文書検査に成功した: link/anchor/fence/table列数")
    return 0


if __name__ == "__main__":
    sys.exit(main())
