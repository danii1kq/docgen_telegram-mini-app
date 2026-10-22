#!/usr/bin/env python3
"""Check a .docx against key СТ КазНИТУ 09-2023 formatting requirements.

Pure standard library (no third-party packages). It inspects the OOXML inside the
.docx archive directly.

Usage:
    python check_docx.py work.docx --type diploma
    python check_docx.py work.docx --type master
    python check_docx.py titul.docx --kind title        # title page: expects 12pt, skips headings

Profiles:
    diploma (default) : top 20, bottom 25, left 30, right 10 mm
    master            : top 20, bottom 25, left 30, right 15 mm

Kinds:
    body  (default)   : main text — expects 14 pt, checks first-line indent and structural headings
    title             : title page — expects 12 pt, skips indent/heading checks

Exit code: 0 if no FAIL, 1 if any FAIL, 2 on error.
"""
from __future__ import annotations

import argparse
import sys
import zipfile
import xml.etree.ElementTree as ET
from collections import Counter
from statistics import median

W = "{http://schemas.openxmlformats.org/wordprocessingml/2006/main}"
TWIPS_PER_MM = 1440 / 25.4          # 1 mm = 56.6929 twips
A4_W_TW, A4_H_TW = 11906, 16838     # 210 x 297 mm
TOL_TW = 25                         # ~0.44 mm tolerance

PROFILES = {
    "diploma": {"top": 20, "bottom": 25, "left": 30, "right": 10},
    "master":  {"top": 20, "bottom": 25, "left": 30, "right": 15},
}
BODY_FONT_PT = 14
# «пять знаков» при TNR 14 pt ≈ ширина 5 знаков ≈ 5 * 0.5em = 5 * 7pt = 35pt = 700 twips
DEFAULT_INDENT_TW = round(5 * 0.5 * BODY_FONT_PT * 20)


def mm2tw(mm: float) -> int:
    return round(mm * TWIPS_PER_MM)


def tw2mm(tw: float) -> float:
    return round(tw / TWIPS_PER_MM, 1)


def a(el, name):
    return el.get(W + name) if el is not None else None


def find(el, *tags):
    cur = el
    for t in tags:
        if cur is None:
            return None
        cur = cur.find(W + t)
    return cur


def read_part(zf: zipfile.ZipFile, name: str):
    try:
        with zf.open(name) as fh:
            return ET.fromstring(fh.read())
    except KeyError:
        return None
    except ET.ParseError:
        return None


def build_styles(styles_root):
    """Return (default_font, default_sz_pt, style_sz, style_font, style_based, default_style_id)."""
    default_font = default_sz = None
    default_style_id = None
    style_sz, style_font, style_based = {}, {}, {}
    if styles_root is None:
        return default_font, default_sz, style_sz, style_font, style_based, default_style_id

    dd = find(styles_root, "docDefaults", "rPrDefault", "rPr")
    if dd is not None:
        rf = find(dd, "rFonts")
        default_font = a(rf, "ascii") or a(rf, "hAnsi") or a(rf, "cs")
        sz = find(dd, "sz")
        if sz is not None and a(sz, "val"):
            default_sz = int(a(sz, "val")) / 2  # half-points -> pt

    for st in styles_root.findall(W + "style"):
        sid = a(st, "styleId")
        if not sid:
            continue
        if a(st, "default") == "1" and a(st, "type") == "paragraph":
            default_style_id = sid      # the style paragraphs inherit when pStyle is absent
        style_based[sid] = a(find(st, "basedOn"), "val")
        sz = find(st, "rPr", "sz")
        if sz is not None and a(sz, "val"):
            style_sz[sid] = int(a(sz, "val")) / 2
        rf = find(st, "rPr", "rFonts")
        if rf is not None:
            f = a(rf, "ascii") or a(rf, "hAnsi") or a(rf, "cs")
            if f:
                style_font[sid] = f
    return default_font, default_sz, style_sz, style_font, style_based, default_style_id


def _resolve(sid, table, based, fallback):
    seen = set()
    while sid and sid not in seen:
        seen.add(sid)
        if sid in table:
            return table[sid]
        sid = based.get(sid)
    return fallback


def analyze(doc_root, style_sz, style_font, style_based, default_sz, default_font,
            default_style_id=None):
    sizes, fonts, lines, indents = Counter(), Counter(), Counter(), []
    body_with_indent = 0
    for p in doc_root.iter(W + "p"):
        text = "".join(t.text or "" for t in p.iter(W + "t"))
        if len(text.strip()) < 40:
            continue  # headings, captions, short/empty lines are not body text
        body_with_indent += 1
        pstyle = a(find(p, "pPr", "pStyle"), "val") or default_style_id
        para_sz = a(find(p, "pPr", "rPr", "sz"), "val")
        para_font_el = find(p, "pPr", "rPr", "rFonts")

        for r in p.iter(W + "r"):
            if not "".join(t.text or "" for t in r.iter(W + "t")).strip():
                continue
            rsz = a(find(r, "rPr", "sz"), "val")
            if rsz:
                sz = int(rsz) / 2
            elif para_sz:
                sz = int(para_sz) / 2
            else:
                sz = _resolve(pstyle, style_sz, style_based, default_sz)
            if sz:
                sizes[sz] += 1

            rf = find(r, "rPr", "rFonts")
            font = (a(rf, "ascii") or a(rf, "hAnsi") or a(rf, "cs")) if rf is not None else None
            if font is None and para_font_el is not None:
                font = a(para_font_el, "ascii") or a(para_font_el, "hAnsi")
            if font is None:
                font = _resolve(pstyle, style_font, style_based, default_font)
            if font:
                fonts[font] += 1

        spacing = find(p, "pPr", "spacing")
        line = a(spacing, "line")
        rule = a(spacing, "lineRule")
        if line and (rule in (None, "auto")):
            lines[int(line)] += 1

        ind = find(p, "pPr", "ind")
        fl = a(ind, "firstLine") if ind is not None else None
        if fl:
            indents.append(int(fl))

    return sizes, fonts, lines, indents, body_with_indent


def check(path: str, profile: str, kind: str = "body"):
    prof = PROFILES[profile]
    is_title = kind == "title"
    findings = []  # (status, text)

    def add(status, text):
        findings.append((status, text))

    try:
        zf = zipfile.ZipFile(path)
    except (FileNotFoundError, zipfile.BadZipFile) as exc:
        print(f"Cannot open '{path}': {exc}")
        return 2

    with zf:
        doc = read_part(zf, "word/document.xml")
        styles = read_part(zf, "word/styles.xml")
    if doc is None:
        print("word/document.xml not found — not a valid .docx?")
        return 2

    # --- page geometry ---
    sect = None
    for s in doc.iter(W + "sectPr"):
        sect = s  # use the last section
    pgsz = find(sect, "pgSz") if sect is not None else None
    pgmar = find(sect, "pgMar") if sect is not None else None
    w_tw = int(a(pgsz, "w")) if pgsz is not None and a(pgsz, "w") else None
    h_tw = int(a(pgsz, "h")) if pgsz is not None and a(pgsz, "h") else None

    if w_tw and h_tw and abs(w_tw - A4_W_TW) <= TOL_TW and abs(h_tw - A4_H_TW) <= TOL_TW:
        add("OK", f"Формат страницы: A4 ({tw2mm(w_tw)}×{tw2mm(h_tw)} мм)")
    elif w_tw and h_tw:
        add("FAIL", f"Формат страницы: {tw2mm(w_tw)}×{tw2mm(h_tw)} мм (нужен A4 210×297)")
    else:
        add("WARN", "Не удалось определить формат страницы (pgSz)")

    for key, ru in (("top", "верхнее"), ("bottom", "нижнее"),
                    ("left", "левое"), ("right", "правое")):
        raw = a(pgmar, key) if pgmar is not None else None
        need = prof[key]
        if raw is None:
            add("WARN", f"Поле {ru}: не задано (норма {need} мм)")
            continue
        got = tw2mm(int(raw))
        if abs(got - need) <= 0.5:
            add("OK", f"Поле {ru}: {got} мм (норма {need})")
        else:
            add("FAIL", f"Поле {ru}: {got} мм (норма {need})")

    # --- styles ---
    def_font, def_sz, style_sz, style_font, style_based, def_style_id = build_styles(styles)
    sizes, fonts, lines, indents, body = analyze(
        doc, style_sz, style_font, style_based, def_sz, def_font, def_style_id)

    if def_font:
        if "times new roman" in def_font.lower():
            add("OK", f"Шрифт по умолчанию: {def_font}")
        else:
            add("FAIL", f"Шрифт по умолчанию: {def_font} (нужен Times New Roman)")
    else:
        add("WARN", "Шрифт по умолчанию не задан в styles.xml")

    if def_sz is not None:
        expected_sizes = {11, 12, 13} if is_title else {13, 14}
        if def_sz in expected_sizes:
            add("OK", f"Кегль по умолчанию: {def_sz:g} pt")
        else:
            add("WARN", f"Кегль по умолчанию: {def_sz:g} pt "
                        f"(для титульного листа — 12, для основного текста — 14)")

    if sizes:
        top_sz, top_n = sizes.most_common(1)[0]
        share = top_n / sum(sizes.values()) * 100
        ok_sizes = {11, 12} if is_title else {BODY_FONT_PT}
        if top_sz in ok_sizes:
            add("OK", f"Основной кегль текста: {top_sz:g} pt ({share:.0f}% знаков)")
        else:
            extra = " (в таблице допускается меньший)" if is_title else ""
            add("FAIL", f"Основной кегль текста: {top_sz:g} pt ({share:.0f}%), "
                        f"нужен {'11–12' if is_title else '14'} pt{extra}")
    else:
        add("WARN", "Не найдено длинных абзацев для оценки кегля")

    if fonts:
        fam, fam_n = fonts.most_common(1)[0]
        if "times new roman" in fam.lower():
            add("OK", f"Основной шрифт текста: {fam}")
        else:
            add("WARN", f"Основной шрифт текста: {fam} (нужен Times New Roman)")

    if lines:
        common_line, _ = lines.most_common(1)[0]
        if abs(common_line - 240) <= 12:
            add("OK", f"Междустрочный интервал: одинарный (line={common_line})")
        elif abs(common_line - 360) <= 12:
            add("FAIL", f"Междустрочный интервал: полуторный (line={common_line}); нужен одинарный")
        else:
            add("WARN", f"Междустрочный интервал: line={common_line} (норма — одинарный, 240)")
    else:
        add("WARN", "Явный межстрочный интервал не найден (наследуется — проверьте вручную)")

    if is_title:
        add("OK", "Титульный лист: абзацный отступ не требуется (проверка пропущена)")
    elif indents:
        med = median(indents)
        if abs(med - DEFAULT_INDENT_TW) <= 60:
            add("OK", f"Абзацный отступ: {med:g} twips (~{tw2mm(med)} мм ≈ 5 знаков)")
        else:
            add("WARN", f"Абзацный отступ: {med:g} twips (~{tw2mm(med)} мм), норма ≈ {DEFAULT_INDENT_TW} twips")
    else:
        add("WARN", "Абзацный отступ (firstLine) не задан явно — проверьте вручную")

    # --- structural heading presence (informational) ---
    if is_title:
        add("OK", "Титульный лист: проверка структурных заголовков пропущена")
    else:
        full_text = "".join(t.text or "" for t in doc.iter(W + "t")).upper()
        for word in ("СОДЕРЖАНИЕ", "ВВЕДЕНИЕ", "ЗАКЛЮЧЕНИЕ", "СПИСОК ИСПОЛЬЗОВАННОЙ ЛИТЕРАТУРЫ"):
            if word in full_text:
                add("OK", f"Найден структурный заголовок «{word}»")
            else:
                add("WARN", f"Не найден заголовок «{word}» (проверьте состав работы, таблица 1)")

    # --- output ---
    print(f"=== Проверка СТ КазНИТУ 09-2023: {path} (профиль: {profile}) ===\n")
    for status, txt in findings:
        mark = {"OK": "[ OK ]", "WARN": "[WARN]", "FAIL": "[FAIL]"}[status]
        print(f"{mark} {txt}")
    n_ok = sum(1 for s, _ in findings if s == "OK")
    n_warn = sum(1 for s, _ in findings if s == "WARN")
    n_fail = sum(1 for s, _ in findings if s == "FAIL")
    print(f"\nИтог: {n_ok} OK, {n_warn} WARN, {n_fail} FAIL")
    print("Примечание: скрипт проверяет геометрию и шрифты. Нумерацию страниц, "
          "таблицы, формулы, сноски и приложения проверьте по assets/checklist.md.")
    return 1 if n_fail else 0


def main():
    ap = argparse.ArgumentParser(description="Check .docx vs СТ КазНИТУ 09-2023")
    ap.add_argument("path", help="path to the .docx file")
    ap.add_argument("--type", choices=sorted(PROFILES), default="diploma",
                    help="diploma (right margin 10mm) or master (right margin 15mm)")
    ap.add_argument("--kind", choices=["body", "title"], default="body",
                    help="body (main text, 14pt) or title (title page, 12pt)")
    args = ap.parse_args()
    sys.exit(check(args.path, args.type, args.kind))


if __name__ == "__main__":
    main()


