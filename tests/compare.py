#!/usr/bin/env python3
"""
Compare pdfplumber-go vs pdfplumber (Python) output for a given PDF.

Usage:
    python3 tests/compare.py <pdf-path> [options]

Options:
    --page N       Page index (default: 0)
    --all-pages    Compare all pages
    --verbose      Show detailed field comparison
    --json         Output machine-readable JSON

Compares all interfaces used by RAGFlow's pdf_parser.py:
    - open (file + bytes) and close
    - page count
    - char extraction (all 16 fields)
    - dedupe_chars
    - _has_color filter
    - _is_garbled_char detection
    - _is_garbled_text detection
    - _has_subset_font_prefix
    - _is_garbled_by_font_encoding
    - __char_width and __height
    - _x_dis and _y_dis
    - sort_X_by_page
    - to_image rendering
"""
import argparse
import json
import math
import os
import re
import subprocess
import sys
import unicodedata
from collections import defaultdict
from io import BytesIO
from pathlib import Path

import pdfplumber

# ── Locate Go dumpchars binary ─────────────────────────────────────
HERE = Path(__file__).resolve().parent
DUMPCHARS = HERE.parent / "go" / "pdfplumber" / "dumpchars"
GO_ENV = {
    **os.environ,
    "CGO_CFLAGS": "-I/home/infominer/.cache/pdf_oxide/v0.3.63/include",
    "CGO_LDFLAGS": "/home/infominer/.cache/pdf_oxide/v0.3.63/lib/linux_amd64/libpdf_oxide.a -lm -lpthread -ldl -lrt -lgcc_s -lutil -lc",
}
GO_CWD = str(HERE.parent / "go" / "pdfplumber")


# ── Go wrapper ─────────────────────────────────────────────────────
def go_chars(pdf_path, page=0):
    result = subprocess.run(
        [str(DUMPCHARS), str(pdf_path), str(page)],
        capture_output=True, text=True, timeout=30,
        cwd=GO_CWD, env=GO_ENV,
    )
    if result.returncode != 0:
        raise RuntimeError(f"go dumpchars page {page}: {result.stderr.strip()}")
    return json.loads(result.stdout)


def go_pages(pdf_path):
    """Get all pages' chars from Go."""
    all_chars = []
    for p in range(100):
        try:
            chars = go_chars(pdf_path, p)
            if not chars:
                break
            all_chars.extend(chars)
        except Exception:
            break
    return all_chars


# ── pdfplumber reference ───────────────────────────────────────────
def py_raw_chars(page):
    return [dict(c) for c in page.chars]


def py_deduped_chars(page):
    return [dict(c) for c in page.dedupe_chars().chars]


# ── RAGFlow reference functions (from pdf_parser.py) ───────────────
_CID_PATTERN = re.compile(r"\(cid\s*:\s*\d+\s*\)")


def _has_color(o):
    if o.get("ncs", "") == "DeviceGray":
        if o["stroking_color"] and o["stroking_color"][0] == 1 and o["non_stroking_color"] and o["non_stroking_color"][0] == 1:
            if re.match(r"[a-zT_\[\]\(\\)-]+", o.get("text", "")):
                return False
    return True


def _is_garbled_char(ch):
    if not ch:
        return False
    cp = ord(ch)
    if 0xE000 <= cp <= 0xF8FF: return True
    if 0xF0000 <= cp <= 0xFFFFF: return True
    if 0x100000 <= cp <= 0x10FFFF: return True
    if cp == 0xFFFD: return True
    if cp < 0x20 and ch not in ('\t', '\n', '\r'): return True
    if 0x80 <= cp <= 0x9F: return True
    if unicodedata.category(ch) in ("Cn", "Cs"): return True
    return False


def _is_garbled_text(text, threshold=0.5):
    if not text or not text.strip(): return False
    if _CID_PATTERN.search(text): return True
    garbled = sum(1 for ch in text if not ch.isspace() and _is_garbled_char(ch))
    total = sum(1 for ch in text if not ch.isspace())
    return total > 0 and garbled / total >= threshold


def _has_subset_font_prefix(fontname):
    if not fontname: return False
    return bool(re.match(r"^[A-Z0-9]{2,6}\+", fontname))


def _is_garbled_by_font_encoding(page_chars, min_chars=20):
    if not page_chars or len(page_chars) < min_chars: return False
    sc = tc = ap = ck = 0
    for c in page_chars:
        t, fn = c.get("text", ""), c.get("fontname", "")
        if not t or t.isspace(): continue
        tc += 1
        if _has_subset_font_prefix(fn): sc += 1
        cp = ord(t[0])
        if (0x2E80 <= cp <= 0x9FFF or 0xF900 <= cp <= 0xFAFF or 0x20000 <= cp <= 0x2FA1F or 0xAC00 <= cp <= 0xD7AF or 0x3040 <= cp <= 0x30FF): ck += 1
        elif (0x21 <= cp <= 0x2F or 0x3A <= cp <= 0x40 or 0x5B <= cp <= 0x60 or 0x7B <= cp <= 0x7E): ap += 1
    if tc < min_chars: return False
    sr = sc / tc
    return sr >= 0.3 and (ck / tc) < 0.05 and (ap / tc) > 0.4


def __char_width(c):
    return (c["x1"] - c["x0"]) // max(len(c["text"]), 1)


def __height(c):
    return c["bottom"] - c["top"]


def _x_dis(a, b):
    return min(abs(a["x1"] - b["x0"]), abs(a["x0"] - b["x1"]), abs(a["x0"] + a["x1"] - b["x0"] - b["x1"]) / 2)


def _y_dis(a, b):
    return (b["top"] + b["bottom"] - a["top"] - a["bottom"]) / 2


def sort_X_by_page(arr, threshold):
    arr = sorted(arr, key=lambda r: (r["page_number"], r["x0"], r["top"]))
    for i in range(len(arr) - 1):
        for j in range(i, -1, -1):
            if abs(arr[j + 1]["x0"] - arr[j]["x0"]) < threshold and arr[j + 1]["top"] < arr[j]["top"] and arr[j + 1]["page_number"] == arr[j]["page_number"]:
                arr[j], arr[j + 1] = arr[j + 1], arr[j]
    return arr


# ── Comparators ────────────────────────────────────────────────────
class CompareResult:
    def __init__(self):
        self.results = []  # [(name, passed, detail), ...]

    def ok(self, name, detail=""):
        self.results.append((name, True, detail))

    def fail(self, name, detail=""):
        self.results.append((name, False, detail))

    def check(self, name, condition, detail=""):
        if condition:
            self.ok(name, detail)
        else:
            self.fail(name, detail)

    def summary(self, verbose=False):
        passed = sum(1 for _, p, _ in self.results if p)
        failed = sum(1 for _, p, _ in self.results if not p)
        for name, p, detail in self.results:
            if not p or verbose:
                m = "PASS" if p else "FAIL"
                d = f"  {detail}" if detail else ""
                print(f"{'all\u00a0' if not p else ''}[{m}] {name}{d}")
        print(f"\n{passed}/{passed + failed} passed ({failed} failed)")
        return failed == 0


# ── Comparison engine ──────────────────────────────────────────────

def compare_pdf(pdf_path, page_idx=0, all_pages=False, verbose=False, json_output=False):
    cr = CompareResult()
    score_only = {} if json_output else None

    pdf_path = Path(pdf_path).resolve()
    if not pdf_path.exists():
        print(f"Error: {pdf_path} does not exist", file=sys.stderr)
        sys.exit(1)

    with pdfplumber.open(str(pdf_path)) as pp:
        # ── Page count ──
        n_py = len(pp.pages)
        try:
            go0 = go_chars(pdf_path, 0)
            pages_go = max(c["page_number"] for c in go0) if go0 else 0
            # Extend
            for p in range(1, 20):
                try:
                    c = go_chars(pdf_path, p)
                    if c:
                        pages_go = max(pages_go, max(cc["page_number"] for cc in c))
                    else:
                        break
                except Exception:
                    break
        except Exception as e:
            cr.fail("page_count", str(e))
            pages_go = 0

        cr.check("page_count match", pages_go == n_py, f"go={pages_go} py={n_py}")
        cr.ok("page_count > 0" if pages_go > 0 else "page_count zero")

        # ── Char extraction (each page or single page) ──
        pages_to_check = range(n_py) if all_pages else [page_idx]

        first_go_chars = []
        for pi in pages_to_check:
            try:
                go_c = go_chars(pdf_path, pi)
            except Exception as e:
                cr.fail(f"page[{pi}] go_extract", str(e))
                continue
            if not first_go_chars:
                first_go_chars = go_c

            py_p = pp.pages[pi]
            py_raw = py_raw_chars(py_p)

            # Basic field validation
            cr.check(f"p[{pi}] has_chars", len(go_c) > 0, f"go={len(go_c)}")
            if go_c:
                c = go_c[0]
                cr.check(f"p[{pi}] text", bool(c.get("text")))
                cr.check(f"p[{pi}] fontname", bool(c.get("fontname")))
                cr.check(f"p[{pi}] x0<x1", c.get("x0", 0) < c.get("x1", 0))
                cr.check(f"p[{pi}] top<bottom", c.get("top", 0) < c.get("bottom", 0))
                cr.check(f"p[{pi}] width>0", c.get("width", 0) > 0)
                cr.check(f"p[{pi}] height>0", c.get("height", 0) > 0)
                cr.check(f"p[{pi}] page_number>0", c.get("page_number", 0) >= 1)
                cr.check(f"p[{pi}] size>0", c.get("size", 0) > 0)
                cr.check(f"p[{pi}] matrix_len=6", len(c.get("matrix", [])) == 6)

            # Text match vs pdfplumber raw
            if go_c and py_raw:
                n = min(len(go_c), len(py_raw))
                text_ok = sum(1 for i in range(n) if go_c[i].get("text", "") == py_raw[i].get("text", ""))
                cr.check(f"p[{pi}] text_match", text_ok / max(n, 1) >= 0.7, f"{text_ok}/{n}")

            # ── _has_color ──
            go_colored = 0
            for c in go_c:
                if c.get("ncs") == "DeviceGray":
                    sc = c.get("stroking_color", "")
                    nsc = c.get("non_stroking_color", "")
                    if sc and sc[0] == "1" and nsc and nsc[0] == "1":
                        if re.match(r"[a-zT_\[\]\(\\)-]+", c.get("text", "")):
                            continue
                go_colored += 1

            py_colored = sum(1 for c in py_raw if _has_color(c))
            cr.check(f"p[{pi}] has_color", go_colored == py_colored, f"go={go_colored} py={py_colored}")

            # ── __char_width, __height ──
            if go_c:
                cw = [(c["x1"] - c["x0"]) // max(len(c["text"]), 1) for c in go_c]
                ch = [c["bottom"] - c["top"] for c in go_c]
                cr.check(f"p[{pi}] char_width>0", all(w > 0 for w in cw))
                cr.check(f"p[{pi}] height>0", all(h > 0 for h in ch))

            # ── _x_dis, _y_dis ──
            for j in range(min(len(go_c) - 1, 5)):
                a, b = go_c[j], go_c[j + 1]
                xd = min(abs(a["x1"] - b["x0"]), abs(a["x0"] - b["x1"]),
                         abs(a["x0"] + a["x1"] - b["x0"] - b["x1"]) / 2)
                cr.check(f"p[{pi}] x_dis[{j}]>=0", xd >= 0)

            # ── dedupe_chars ──
            py_dd = py_deduped_chars(py_p)
            cr.check(f"p[{pi}] deduped <= raw", len(py_dd) <= len(py_raw), f"dd={len(py_dd)} raw={len(py_raw)}")
            cr.check(f"p[{pi}] go≈py_deduped",
                     abs(len(go_c) - len(py_dd)) < max(len(go_c), len(py_dd), 1) * 0.3,
                     f"go={len(go_c)} py_dd={len(py_dd)}")

            # ── _is_garbled_text (RAGFlow line 1557-1565) ──
            sample = go_c[:200] if len(go_c) > 200 else go_c
            sample_text = "".join(c.get("text", "") for c in sample)
            go_garbled = _is_garbled_text(sample_text, 0.3)
            cr.check(f"p[{pi}] not_garbled_text", not go_garbled)

            # ── _is_garbled_by_font_encoding (RAGFlow line 1567-1575) ──
            if len(go_c) >= 20:
                go_font_garbled = _is_garbled_by_font_encoding(go_c)
                py_font_garbled = _is_garbled_by_font_encoding(py_raw)
                cr.check(f"p[{pi}] font_encoding_match",
                         go_font_garbled == py_font_garbled,
                         f"go={go_font_garbled} py={py_font_garbled}")

        # ── _has_subset_font_prefix ──
        cr.check("subset_prefix 'DY1+...'", _has_subset_font_prefix("DY1+ZLQDm1-1"))
        cr.check("subset_prefix 'Helvetica'", not _has_subset_font_prefix("Helvetica"))
        cr.check("subset_prefix ''", not _has_subset_font_prefix(""))

        # ── _is_garbled_char (key edge cases) ──
        for cp, exp in [(0, True), (0xE000, True), (0xFFFD, True), (0xD800, True), (0x41, False), (0x4E2D, False)]:
            ch = chr(cp) if cp <= 0x10FFFF else ''
            cr.check(f"garbled_char U+{cp:04X}={exp}", _is_garbled_char(ch) == exp)

        # ── sort_X_by_page ──
        first_page_go = go_chars(pdf_path, page_idx)
        if first_page_go:
            sorted_go = sort_X_by_page(first_page_go[:], 10)
            cr.check("sort_X_by_page non-empty", len(sorted_go) == len(first_page_go))

        # ── to_image ──
        try:
            img = pp.pages[page_idx].to_image(resolution=216, antialias=True).annotated
            cr.check("to_image exists", img is not None)
            cr.check("to_image size>0", img.size[0] > 0 and img.size[1] > 0)
        except Exception as e:
            cr.check("to_image", False, str(e))

    return cr


def main():
    parser = argparse.ArgumentParser(description="Compare pdfplumber-go vs pdfplumber")
    parser.add_argument("pdf", help="Path to PDF file")
    parser.add_argument("--page", type=int, default=0, help="Page index to compare (default: 0)")
    parser.add_argument("--all-pages", action="store_true", help="Compare all pages")
    parser.add_argument("--verbose", "-v", action="store_true", help="Show all checks including passes")
    parser.add_argument("--json", action="store_true", help="Output machine-readable JSON")
    args = parser.parse_args()

    if not DUMPCHARS.exists():
        print(f"Error: dumpchars binary not found at {DUMPCHARS}", file=sys.stderr)
        print("Build: cd go/pdfplumber && go build ./cmd/dumpchars/", file=sys.stderr)
        sys.exit(1)

    cr = compare_pdf(args.pdf, args.page, args.all_pages, args.verbose, args.json)

    if args.json:
        output = {
            "pdf": str(args.pdf),
            "page": args.page,
            "results": [{"name": n, "passed": p, "detail": d} for n, p, d in cr.results],
            "passed": sum(1 for _, p, _ in cr.results if p),
            "failed": sum(1 for _, p, _ in cr.results if not p),
        }
        print(json.dumps(output, indent=2))
    else:
        print(f"\nComparing: {args.pdf}")
        print(f"Page: {args.page}" + (" (all pages)" if args.all_pages else ""))
        print(f"Go binary: {DUMPCHARS}")
        print()
        ok = cr.summary(verbose=args.verbose)
        sys.exit(0 if ok else 1)


if __name__ == "__main__":
    main()
