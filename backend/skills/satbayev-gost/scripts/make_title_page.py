#!/usr/bin/env python3
"""Generate Satbayev University work title pages **1:1** from the official template.

Instead of re-drawing the layout, this clones the exact page from
``assets/titulki-template.docx`` (the university «ТИТУЛКИ» file) and only substitutes
the variable text (theme, student, teacher, specialty, group, institute, chair, year).
Everything else — page size, margins, paragraph indents, alignments, fonts, sizes,
spacing, the logo and the assessment table — is copied verbatim, so the result matches
the original layout exactly.

Pure standard library — writes the OOXML package directly, no third-party packages.

Examples:
    python make_title_page.py --type laboratornaya --theme "Изучение осциллографа" \
        --institute "Институт автоматики и информационных технологий" \
        --chair "Кафедра компьютерных наук" --student "Иванов И.И." \
        --teacher "Петров П.П." --specialty "6B06101" --group "ИС-21" -o titul.docx
    python make_title_page.py --type sors --lang kz --theme "Сандық құрылғылар" -o t.docx
    python make_title_page.py --type referat --lang en --theme "Quality control" -o t.docx
    python make_title_page.py --list
"""
from __future__ import annotations

import argparse
import copy
import os
import re
import sys
import zipfile
import xml.etree.ElementTree as ET

HERE = os.path.dirname(os.path.abspath(__file__))
TEMPLATE = os.path.join(HERE, os.pardir, "assets", "titulki-template.docx")
W = "{http://schemas.openxmlformats.org/wordprocessingml/2006/main}"
XML_SPACE = "{http://www.w3.org/XML/1998/namespace}space"
HEADERS = ("МИНИСТЕРСТВО ОБРАЗОВАНИЯ", "ҚАЗАҚСТАН РЕСПУБЛИКАСЫ", "Ministry", "Ministy")

# Defects of the official «ТИТУЛКИ» file that are normalised while cloning, so title
# pages stay clean even if the template is re-downloaded / replaced by the pristine one.
TEXT_FIXUPS = (
    ("\u00ac", ""),                    # stray NOT SIGN in «Кафедра¬___» (RU лабораторная)
    ("Ministy", "Ministry"),           # typo on the English forms
    ("Abstact", "Abstract"),           # typo in the English criteria table
    ("ЛAБОРАТОРНАЯ", "ЛАБОРАТОРНАЯ"),  # Latin «A» in the RU lab title
)

# The university line under the ministry is short (client's requirement): the RU
# and KZ pages of the template already print it, the English ones still carry the
# long wording, and a replaced template may carry the long RU/KZ lines too.
UNIVERSITY_SHORT = {
    "ru": "УНИВЕРСИТЕТ САТПАЕВ",
    "kz": "СӘТБАЕВ УНИВЕРСИТЕТІ",
    "en": "SATBAYEV UNIVERSITY",
}
UNIVERSITY_LONG = (
    "Некоммерческое акционерное общество",
    "Казахский национальный исследовательский технический университет",
    "Коммерциялық емес акционерлік қоғам",
    "Қ.И. Сәтбаев атындағы Қазақ ұлттық",
    "K.I. Satpaev Kazakh National Research Technical University",
    "Kazakh National Research Technical University",
    "Non-profit joint-stock company",
    "Nonprofit joint-stock company",
)


def short_university(text, lang):
    """The short university line when the paragraph is a long university name.

    Only short paragraphs qualify: the same wording may legitimately appear inside
    a paragraph of the document body (references, questions), which must stay.
    """
    stripped = text.strip()
    if not stripped or len(stripped) > 200:
        return None
    if any(bad in stripped for bad in UNIVERSITY_LONG):
        return UNIVERSITY_SHORT[lang]
    return None

# (lang, type_key) -> page index inside the template (page numbers from the inventory)
PAGE_MAP = {
    ("ru", "referat"): 0, ("kz", "referat"): 1,
    ("ru", "srs"): 2, ("kz", "srs"): 3,
    ("ru", "kursovaya"): 4, ("kz", "kursovaya"): 5,
    ("ru", "srsp"): 6, ("kz", "esse"): 7,
    ("ru", "srm"): 8, ("ru", "esse"): 9, ("kz", "srsp"): 10,
    ("ru", "laboratornaya"): 11, ("kz", "laboratornaya"): 12,
    ("kz", "tazhiribelik"): 13, ("ru", "srmp"): 14, ("kz", "srm"): 15,
    ("ru", "prakticheskaya"): 16, ("kz", "prakticheskaya"): 17,
    ("kz", "zertkhanalyk"): 18, ("ru", "semestrovaya"): 19, ("kz", "semestrovaya"): 20,
    ("ru", "seminar"): 21, ("en", "referat"): 22, ("en", "srs"): 23,
    ("ru", "doklad"): 24, ("kz", "doklad"): 25, ("kz", "zhut"): 26,
    ("ru", "idz"): 27, ("ru", "samostoyatelnaya"): 28,
    ("kz", "otchet"): 29, ("ru", "otchet"): 30,
    ("en", "tsis"): 31, ("en", "srsp"): 32, ("en", "laboratornaya"): 33,
    ("ru", "rgr"): 34, ("ru", "kommercheskiy"): 35, ("ru", "rubezhny"): 36,
}

# friendly aliases so the user can type what they see
ALIASES = {
    ("ru", "sors"): ("ru", "srs"), ("kz", "sors"): ("kz", "srs"),
    ("kz", "sozh"): ("kz", "srs"), ("ru", "sozh"): ("ru", "srs"),
    ("en", "report"): ("en", "referat"), ("en", "siw"): ("en", "srs"),
    ("en", "sis"): ("en", "srsp"), ("ru", "kursovoy"): ("ru", "kursovaya"),
    ("en", "srs_srsp"): ("en", "tsis"), ("kz", "bayandama"): ("kz", "doklad"),
}

PUBLIC_TYPES = sorted({k[1] for k in PAGE_MAP})

# Work-type line of every template page — lets --title replace it (e.g. КОНТРОЛЬНАЯ РАБОТА).
PAGE_TITLES = {
    0: "РЕФЕРАТ", 1: "РЕФЕРАТ", 2: "СРС", 3: "СӨЖ", 4: "КУРСОВАЯ РАБОТА",
    5: "КУРСТЫҚ ЖҰМЫС", 6: "СРСП", 7: "ЭССЕ", 8: "СРМ", 9: "ЭССЕ", 10: "СОӨЖ",
    11: "ЛАБОРАТОРНАЯ РАБОТА", 12: "ЛАБОРАТОРИЯЛЫҚ ЖҰМЫС", 13: "ТӘЖІРБИЕЛІК  ЖҰМЫС",
    14: "СРМП", 15: "МӨОЖ", 16: "ПРАКТИЧЕСКАЯ РАБОТА", 17: "ПРАКТИКАЛЫҚ ЖҰМЫС",
    18: "ЗЕРТХАНАЛЫҚ ЖҰМЫС", 19: "СЕМЕСТРОВАЯ РАБОТА", 20: "СЕМЕСТРЛІК ЖҰМЫС",
    21: "Семинар", 22: "REPORT", 23: "STUDENT’S INDEPENDENT WORK", 24: "ДОКЛАД",
    25: "БАЯНДАМА", 26: "ЖҰТ", 27: "ИДЗ", 28: "Самостоятельная работа", 29: "Есеп беру",
    30: "Отчет", 31: "TSIS", 32: "SIS", 33: "LABORATORY WORK", 34: "РГР",
    35: "Коммерческий проект", 36: "Рубежный контроль",
}


def ptext(p):
    return "".join(t.text or "" for t in p.iter(W + "t"))


def clean_text(text):
    """Normalise the source-template defects listed in TEXT_FIXUPS."""
    for bad, good in TEXT_FIXUPS:
        text = text.replace(bad, good)
    return text


def set_text(p, new_text, anchor_label=None):
    """Write new_text into one run (keeping its formatting) and blank the rest.

    The run whose text contains anchor_label is used as the formatting donor; otherwise
    the run with the largest font size wins. Paragraph properties (jc/ind/spacing) are
    untouched, so the line keeps its exact position.
    """
    runs = [r for r in p.findall(W + "r") if r.find(W + "t") is not None]
    if not runs:
        return
    anchor = None
    if anchor_label:
        for r in runs:
            if anchor_label in (r.find(W + "t").text or ""):
                anchor = r
                break
    if anchor is None:
        def size_of(r):
            sz = r.find(W + "rPr/" + W + "sz")
            v = sz.get(W + "val") if sz is not None else None
            return int(v) if v and v.isdigit() else 0
        anchor = max(runs, key=size_of)
    for r in runs:
        r.find(W + "t").text = ""
    anchor.find(W + "t").text = new_text
    anchor.find(W + "t").set(XML_SPACE, "preserve")


SIG_IND_TW = 4800  # twips (~8.5 cm) — left edge of the right-side signature block

# Per-language signature fields, in assignment order:
# (label found in the template, data key, label printed in the output).
# «Специальность» becomes «Шифр ОП» and the group line is dropped (RU); the KZ form
# merges «Мамандығы» + «Тобы» into «Мамандық шифрі»; EN keeps its own three fields.
SIG_SPEC = {
    "ru": (("Преподаватель", "teacher", "Преподаватель"),
           ("Специальность", "specialty", "Шифр ОП"),
           ("Студент", "student", "Студент"),
           ("Магистрант", "student", "Магистрант")),
    "kz": (("Оқытушы:", "teacher", "Оқытушы:"),
           ("Мамандығы:", "specialty", "Мамандық шифрі:"),
           ("Студент:", "student", "Студент:"),
           ("Магистрант:", "student", "Магистрант:")),
    "en": (("Written by:", "student", "Written by:"),
           ("Witten by:", "student", "Written by:"),
           ("Group:", "group", "Group:"), ("Checked by", "teacher", "Checked by")),
}

# Template lines merged into another field — removed from the rebuilt block.
SIG_DROP = {"ru": ("Группа",), "kz": ("Тобы:",), "en": ()}

# KZ «ЗЕРТХАНАЛЫҚ ЖҰМЫС» uses a label line plus a separate underline line instead of
# «Оқытушы: ___»; the value is written on the underline line.
KZ_SPLIT_LABELS = (("Білім алушының аты-жөні", "student"),
                   ("Мамандық шифрі", "specialty"),
                   ("Оқытушының аты-жөні", "teacher"))


def fill_split_labels(paras, spec, d):
    """Fill «label line + underline line» blocks (label stays, value goes on the line)."""
    for lbl, key in spec:
        val = d.get(key, "")
        if not val:
            continue
        for i, p in enumerate(paras):
            t = ptext(p)
            if lbl not in t or val in t:   # already filled — never duplicate the value
                continue
            if "_" in t:  # label and blank share one line
                set_text(p, re.sub(re.escape(lbl) + r"[\s_]*$", lbl + " " + val, t, count=1))
            elif i + 1 < len(paras) and re.fullmatch(r"\s*_+\s*", ptext(paras[i + 1])):
                set_text(paras[i + 1], val)
            else:
                set_text(p, t.rstrip() + " " + val)
            break


def strip_affixes(value, *words):
    """Drop a leading «Институт» / trailing «институты» so the label is not doubled."""
    v = value.strip()
    for w in words:
        v = re.sub(rf"^\s*{w}[\s:]+", "", v, flags=re.I)
        stripped = re.sub(rf"\s*{w}\s*$", "", v, flags=re.I)
        if stripped.strip():
            v = stripped
    return v.strip()


def fill_infrastructure(text, d):
    """Fill «____ институты / ____ кафедрасы» lines (all KZ pages, RU семестровая)."""
    if d["institute"]:
        inst = strip_affixes(d["institute"], "институт", "институты")
        text = re.sub(r"_+\s*институты", inst + " институты", text)
    if d["chair"]:
        ch = strip_affixes(d["chair"], "кафедра", "кафедрасы")
        text = re.sub(r"_+\s*кафедрасы", ch + " кафедрасы", text)
    return text



def _ensure_ppr(p):
    ppr = p.find(W + "pPr")
    if ppr is None:
        ppr = ET.Element(W + "pPr")
        p.insert(0, ppr)
    return ppr


# CT_PPr is order-sensitive: w:ind must precede w:jc, and both must precede w:rPr/sectPr.
_AFTER_IND = frozenset((
    "contextualSpacing", "mirrorIndents", "suppressOverlap", "jc", "textDirection",
    "textAlignment", "textboxTightWrap", "outlineLvl", "divId", "cnfStyle",
    "rPr", "sectPr", "pPrChange"))
_AFTER_JC = frozenset((
    "textDirection", "textAlignment", "textboxTightWrap", "outlineLvl", "divId",
    "cnfStyle", "rPr", "sectPr", "pPrChange"))


def _insert_ordered(ppr, elem, after_set):
    """Insert elem into pPr just before the first child that must follow it."""
    pos = len(ppr)
    for k, child in enumerate(ppr):
        if child.tag.split("}")[-1] in after_set:
            pos = k
            break
    ppr.insert(pos, elem)


def set_right_block(p, text, indent_tw=SIG_IND_TW):
    """Left-align paragraph text but shift it right by a fixed indent."""
    ppr = _ensure_ppr(p)
    jc = ppr.find(W + "jc")
    ind = ppr.find(W + "ind")
    if ind is None:
        ind = ET.Element(W + "ind")
        _insert_ordered(ppr, ind, _AFTER_IND)
    ind.set(W + "left", str(indent_tw))
    for k in ("firstLine", "hanging", "firstLineChars", "leftChars", "right"):
        ind.attrib.pop(W + k, None)
    if jc is None:
        jc = ET.Element(W + "jc")
        _insert_ordered(ppr, jc, _AFTER_JC)
    jc.set(W + "val", "left")
    set_text(p, text)


def restyle_signatures(paras, lang, d):
    """Rebuild the signature block as clean right-positioned, left-aligned lines.

    The template spaces its signature lines and right-aligns them, so a shorter value
    knocks the columns out of place (and can leave a bare underline line). Here every
    field becomes its own line: left-aligned, shifted to the right side of the page.
    Works for ru / kz / en. Returns the paragraphs that are no longer needed, so the
    caller can drop them instead of leaving blank gaps.
    """
    spec = SIG_SPEC.get(lang)
    if not spec:
        return []

    found = {}
    find_labels = [s[0] for s in spec] + list(SIG_DROP.get(lang, ()))
    for i, p in enumerate(paras):
        t = ptext(p)
        for lbl in find_labels:
            if lbl in t and lbl not in found:
                found[lbl] = i
    if not found:
        return []

    # bare-underline lines that can absorb an extra field (e.g. RU shares one line)
    found_idx = set(found.values())
    spare_pool = [i for i, p in enumerate(paras)
                  if i not in found_idx and re.fullmatch(r"\s*_+\s*", ptext(p))]

    used = set()
    groups = {}  # paragraph index -> list of field texts
    for lbl, key, out in spec:
        if lbl not in found:
            continue
        i = found[lbl]
        if i in used:
            # this template line already holds another field: move to a spare line if
            # one is free, otherwise keep both fields on the same line
            sp = next((j for j in spare_pool if j not in used), None)
            if sp is not None:
                i = sp
        used.add(i)
        val = d.get(key, "")
        text = f"{out} {val}" if val else out + " " + "_" * 20
        groups.setdefault(i, []).append(text)

    for i, texts in groups.items():
        set_right_block(paras[i], "   ".join(texts))

    # Unused bare-underline lines would leave an ugly gap — drop them, and drop the
    # template lines merged into another field («Группа» / «Тобы:»). The caller removes
    # the returned paragraphs from the document.
    drop = [paras[j] for j in spare_pool if j not in used]
    for lbl in SIG_DROP.get(lang, ()):
        if lbl in found and found[lbl] not in used:
            drop.append(paras[found[lbl]])
    return drop


def page_ranges(blocks):
    """Return a list of (start, end) block ranges, one per title page."""
    starts = []
    for i, b in enumerate(blocks):
        if b.tag == W + "p":
            t = ptext(b).strip()
            if any(t.startswith(h) for h in HEADERS):
                starts.append(i)
    starts.append(len(blocks))
    return [(starts[k], starts[k + 1]) for k in range(len(starts) - 1)]


def fill_label(text, label, value, sep=" "):
    """Replace 'Label____' with 'Label<sep>value' (no-op when value is empty).

    If the value already begins with the label word (e.g. the user typed
    'Институт автоматики…'), the label is not repeated.
    """
    if not value:
        return text
    base = label.rstrip(": ").strip()
    if base and value.strip().lower().startswith(base.lower()):
        return re.sub(re.escape(label) + r"\s*_+", value, text, count=1)
    return re.sub(re.escape(label) + r"\s*_+", label + sep + value, text, count=1)


def substitute(text, lang, d):
    """Return the paragraph text with placeholders filled, or None if unchanged."""
    new = clean_text(text)
    if lang == "ru":
        if d["theme"]:
            new = re.sub(r"(Тема:?\s*)_+", lambda m: "Тема: " + d["theme"], new)
        for lbl, key in (("Институт", "institute"), ("Кафедра", "chair"),
                         ("Преподаватель", "teacher"), ("Студент", "student"),
                         ("Магистрант", "student")):
            new = fill_label(new, lbl, d[key])
        # On the RU «Семестровая работа» page the institute/chair lines carry the KZ
        # tails «институты / кафедрасы» — replace the whole blank with the RU wording.
        if d["institute"]:
            new = re.sub(r"_+\s*институты", d["institute"].strip(), new)
        if d["chair"]:
            new = re.sub(r"_+\s*кафедрасы", d["chair"].strip(), new)
    elif lang == "kz":
        if d["theme"]:
            new = re.sub(r"(Тақырыбы:?\s*)_+", lambda m: "Тақырыбы: " + d["theme"], new)
        new = fill_infrastructure(new, d)
        for lbl, key, sep in (("Оқытушы:", "teacher", " "), ("Студент:", "student", " "),
                              ("Магистрант:", "student", " ")):
            new = fill_label(new, lbl, d[key], sep=sep)
    else:  # en
        if d["theme"]:
            new = re.sub(r"(Subject matter:?\s*)_+",
                         lambda m: "Subject matter: " + d["theme"], new)
        if d["institute"]:
            new = re.sub(r"The institute of .*", "The institute of " + d["institute"], new)
        if d["chair"]:
            new = re.sub(r"Chair of .*", "Chair of " + d["chair"], new)
        for lbl, key, sep in (("Witten by:", "student", " "), ("Written by:", "student", " "),
                              ("Group:", "group", ""), ("Checked by", "teacher", " ")):
            new = fill_label(new, lbl, d[key], sep=sep)
    if "Алматы" in new or "Almaty" in new:
        new = re.sub(r"\b20\d\d\b", str(d["year"]), new)
    return new if new != text else None


def build(template_path, page_index, lang, d, out_path, form_code=None,
          drop_form_code=False, title=""):
    with zipfile.ZipFile(template_path) as zin:
        names = zin.namelist()
        raw = zin.read("word/document.xml").decode("utf-8")
        others = {n: zin.read(n) for n in names if n != "word/document.xml"}

    for prefix, uri in re.findall(r'xmlns:([\w.\-]+)="([^"]+)"', raw):
        try:
            ET.register_namespace(prefix, uri)
        except ValueError:
            pass

    # Keep the original root open tag verbatim: ElementTree drops namespace declarations
    # that are unused, but mc:Ignorable still references them, which makes Word/OOXML
    # validators report "Namespace X in Ignorable but not declared".
    _i = raw.index("<w:document")
    raw_root_open = raw[_i:raw.index(">", _i) + 1]

    root = ET.fromstring(raw)
    body = root.find(W + "body")
    blocks = list(body)
    ranges = page_ranges(blocks)
    if not (0 <= page_index < len(ranges)):
        raise ValueError(f"page index {page_index} out of range")
    start, end = ranges[page_index]

    # the single body-level sectPr (page setup) is reused verbatim
    sect = body.find(W + "sectPr")

    new_body = []
    page_paras = []
    for blk in blocks[start:end]:
        if blk.tag == W + "sectPr":
            continue
        blk = copy.deepcopy(blk)
        for t in blk.iter(W + "t"):          # normalise template typos/artefacts,
            if t.text:                       # including text inside the criteria table
                fixed = clean_text(t.text)
                if fixed != t.text:
                    t.text = fixed
        if blk.tag == W + "p":
            t = ptext(blk)
            if form_code is not None and "706-" in t:
                set_text(blk, form_code)
            elif drop_form_code and "706-" in t:
                set_text(blk, "")
            elif short_university(t, lang) is not None:
                set_text(blk, short_university(t, lang))
            else:
                new = substitute(t, lang, d)
                if new is not None:
                    set_text(blk, new)
            page_paras.append(blk)
        new_body.append(blk)

    if lang == "kz":
        fill_split_labels(page_paras, KZ_SPLIT_LABELS, d)
    if lang in SIG_SPEC:
        for p in restyle_signatures(page_paras, lang, d):
            new_body.remove(p)

    # --title replaces the built-in work-type line (e.g. «КОНТРОЛЬНАЯ РАБОТА»)
    if title:
        want = PAGE_TITLES.get(page_index, "")
        for p in page_paras:
            if ptext(p).strip() == want:
                set_text(p, title, anchor_label=want)
                break

    # The ministry line must be centred: the source template carries no <w:jc> on the
    # «Реферат» (RU) page, which renders it left-aligned. Force it for every page.
    if page_paras and ptext(page_paras[0]).strip().startswith(HEADERS):
        ppr = page_paras[0].find(W + "pPr")
        if ppr is None or ppr.find(W + "jc") is None:
            ppr = _ensure_ppr(page_paras[0])
            jc = ET.Element(W + "jc")
            _insert_ordered(ppr, jc, _AFTER_JC)
            jc.set(W + "val", "center")

    for child in list(body):
        body.remove(child)
    for blk in new_body:
        body.append(blk)
    if sect is not None:
        body.append(copy.deepcopy(sect))

    doc_xml = ET.tostring(root, encoding="unicode")
    root_end = doc_xml.index(">") + 1
    et_open, rest = doc_xml[:root_end], doc_xml[root_end:]
    declared = set(re.findall(r'xmlns:([\w.\-]+)=', et_open))
    extra = "".join(f' xmlns:{p}="{u}"'
                    for p, u in re.findall(r'xmlns:([\w.\-]+)="([^"]+)"', raw_root_open)
                    if p not in declared)
    if extra:
        et_open = et_open.replace("<w:document", "<w:document" + extra, 1)
    xml_out = ('<?xml version="1.0" encoding="UTF-8" standalone="yes"?>\r\n'
               + et_open + rest).encode("utf-8")

    with zipfile.ZipFile(out_path, "w", zipfile.ZIP_DEFLATED) as zout:
        for n in names:
            zout.writestr(n, xml_out if n == "word/document.xml" else others[n])


def main():
    ap = argparse.ArgumentParser(
        description="Generate a Satbayev title page 1:1 from the official ТИТУЛКИ template")
    ap.add_argument("--type", default="referat", help="work type key (see --list)")
    ap.add_argument("--lang", default="ru", choices=["ru", "kz", "en"])
    ap.add_argument("--theme", default="", help="тема / тақырыбы / subject matter")
    ap.add_argument("--institute", default="")
    ap.add_argument("--chair", default="")
    ap.add_argument("--student", default="", help="Ф.И.О. обучающегося / студента / магистранта")
    ap.add_argument("--teacher", default="", help="Ф.И.О. преподавателя / оқытушы / checked by")
    ap.add_argument("--specialty", default="",
                    help="шифр ОП / specialty code (printed as «Шифр ОП» on RU, "
                         "«Мамандық шифрі» on KZ)")
    ap.add_argument("--group", default="", help="group — EN form only")
    ap.add_argument("--title", default="",
                    help="replace the work-type line, e.g. 'КОНТРОЛЬНАЯ РАБОТА'")
    ap.add_argument("--form-code", default=None,
                    help="override the form code line, e.g. 'Ф КазНИТУ 706-05. Лабораторная работа'")
    ap.add_argument("--no-form-code", action="store_true", help="remove the form code line")
    ap.add_argument("--year", default="2026")
    ap.add_argument("--template", default=TEMPLATE)
    ap.add_argument("-o", "--out", default="title_page.docx")
    ap.add_argument("--list", action="store_true", help="list supported type/lang combinations")
    args = ap.parse_args()

    if args.list:
        for (lg, tp), pg in sorted(PAGE_MAP.items()):
            print(f"{lg}  {tp:18s} -> page {pg:02d}")
        return 0

    key = (args.lang, args.type)
    key = ALIASES.get(key, key)
    if key not in PAGE_MAP:
        print(f"No title page for type '{args.type}' in '{args.lang}'. "
              f"Use --list to see supported combinations.", file=sys.stderr)
        return 2

    d = dict(theme=args.theme, institute=args.institute, chair=args.chair,
             student=args.student, teacher=args.teacher, specialty=args.specialty,
             group=args.group, year=args.year)

    build(args.template, PAGE_MAP[key], args.lang, d, args.out,
          form_code=args.form_code, drop_form_code=args.no_form_code, title=args.title)
    print(f"Wrote {args.out}  (lang={args.lang}, type={args.type}, "
          f"page={PAGE_MAP[key]:02d}, year={args.year})")
    return 0


if __name__ == "__main__":
    sys.exit(main())


