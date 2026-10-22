#!/usr/bin/env python3
"""Official Satbayev University title page (Приложение Б, В, Ж, М СТ КазНИТУ 09-2023).

Built to match the standard's samples: ministry → university → institute → chair,
«ДОПУЩЕН К ЗАЩИТЕ» block, work type in caps, «На тему: «…»», program cipher,
«Выполнил» / «Рецензент» / «Научный руководитель» blocks, «Алматы <год>».

Usage:
    python make_official_title_page.py --type diplom_rabota \
        --theme "Разработка системы контроля качества" \
        --institute "Институт автоматики и информационных технологий" \
        --chair "Кафедра компьютерных наук" --program "6B06101 «Информационные системы»" \
        --student "Иванов И.И." --supervisor "к.т.н., доцент Петров П.П." \
        --reviewer "к.т.н. Сидоров С.С." --year 2025 -o titul.docx

Omitted values keep the standard's blanks; nothing is invented. «Алматы 2025» —
без слова «год»/«г.» (титул идёт со сквозной нумерацией, но номер не печатают).
"""
from __future__ import annotations

import argparse
import re
import sys
import zipfile

try:
    from docx import Document
    from docx.enum.section import WD_SECTION
    from docx.enum.table import WD_TABLE_ALIGNMENT
    from docx.enum.text import WD_ALIGN_PARAGRAPH, WD_LINE_SPACING
    from docx.oxml.ns import qn
    from docx.shared import Cm, Mm, Pt
except ImportError:  # pragma: no cover
    sys.stderr.write("python-docx is required: pip install python-docx\n")
    raise SystemExit(2)

WORK_TYPES = {
    "diplom_rabota": "ДИПЛОМНАЯ РАБОТА",
    "diplom_proekt": "ДИПЛОМНЫЙ ПРОЕКТ",
    "magisterskaya": "МАГИСТЕРСКАЯ ДИССЕРТАЦИЯ",
    "kursovaya": "КУРСОВАЯ РАБОТА",
    "otchet_praktiki": "ОТЧЕТ О ПРАКТИКЕ",
    "nauchnaya_statya": "НАУЧНАЯ СТАТЬЯ",
}
MINISTRY = ("МИНИСТЕРСТВО НАУКИ И ВЫСШЕГО ОБРАЗОВАНИЯ",
            "РЕСПУБЛИКИ КАЗАХСТАН")
UNIVERSITY = ("Некоммерческое акционерное общество «Казахский национальный "
              "исследовательский технический университет имени К.И. Сатпаева»")
CAPTION_PT = 10


def setup(doc, master):
    sec = doc.sections[0]
    sec.page_width, sec.page_height = Mm(210), Mm(297)
    sec.top_margin, sec.bottom_margin = Mm(20), Mm(25)
    sec.left_margin = Mm(30)
    sec.right_margin = Mm(15) if master else Mm(10)
    st = doc.styles["Normal"]
    st.font.name = "Times New Roman"
    st.font.size = Pt(12)
    st.element.rPr.rFonts.set(qn("w:eastAsia"), "Times New Roman")
    st.element.rPr.rFonts.set(qn("w:cs"), "Times New Roman")
    pf = st.paragraph_format
    pf.line_spacing_rule = WD_LINE_SPACING.SINGLE
    pf.space_before = pf.space_after = Pt(0)


def para(doc, text="", *, bold=False, italic=False, size=None, align=None,
         indent_cm=None, underline=False, first_line_cm=None):
    p = doc.add_paragraph()
    if align is not None:
        p.alignment = align
    pf = p.paragraph_format
    if indent_cm is not None:
        pf.left_indent = Cm(indent_cm)
    if first_line_cm is not None:
        pf.first_line_indent = Cm(first_line_cm)
    if text:
        run = p.add_run(text)
        run.bold = bold
        run.italic = italic
        run.underline = underline
        if size:
            run.font.size = Pt(size)
    return p


def blank(doc, n=1, size=12):
    for _ in range(n):
        p = doc.add_paragraph()
        p.add_run("").font.size = Pt(size)


def _strip_label(value, *labels):
    """Drop a leading «Институт»/«Кафедра» so the form's label is not repeated."""
    v = value.strip()
    for lab in labels:
        v2 = re.sub(rf"^\s*{lab}\s+", "", v, flags=re.I)
        if v2.strip():
            v = v2
    return v


def fill_line(doc, label, value, caption, *, indent_cm=1.6, width_cm=15.0,
              center_caption=True):
    """«Label ______ value ______» with the form's italic caption under the blank."""
    p = para(doc, align=WD_ALIGN_PARAGRAPH.LEFT)
    pf = p.paragraph_format
    pf.left_indent = Cm(indent_cm)
    r = p.add_run(label)
    r.font.size = Pt(12)
    fill = p.add_run("\u00a0" + (value if value else "\u00a0" * int(width_cm * 2)))
    fill.underline = True
    fill.font.size = Pt(12)
    cap = para(doc, caption, italic=True, size=CAPTION_PT,
               align=WD_ALIGN_PARAGRAPH.CENTER if center_caption
               else WD_ALIGN_PARAGRAPH.LEFT)
    return p, cap


def right_block(doc, lines, indent_cm=9.0):
    """Right-side left-aligned block («ДОПУЩЕН К ЗАЩИТЕ», signature columns)."""
    out = []
    for kind, text in lines:
        if kind == "cap":
            p = para(doc, text, italic=True, size=CAPTION_PT,
                     align=WD_ALIGN_PARAGRAPH.CENTER, indent_cm=indent_cm)
        elif kind == "blank":
            p = para(doc, "", indent_cm=indent_cm)
        elif kind == "under":
            p = para(doc, align=WD_ALIGN_PARAGRAPH.LEFT, indent_cm=indent_cm)
            r = p.add_run(text if text else "\u00a0" * 26)
            r.underline = True
        else:
            p = para(doc, text, bold=(kind == "bold"),
                     align=WD_ALIGN_PARAGRAPH.LEFT, indent_cm=indent_cm)
        out.append(p)
    return out


def build(type_key, work_type, d, out_path, master=False):
    doc = Document()
    setup(doc, master)

    # 1–2. Министерство и вуз
    para(doc, MINISTRY[0], bold=True, align=WD_ALIGN_PARAGRAPH.CENTER)
    para(doc, MINISTRY[1], bold=True, align=WD_ALIGN_PARAGRAPH.CENTER)
    blank(doc)
    para(doc, UNIVERSITY, align=WD_ALIGN_PARAGRAPH.CENTER)
    blank(doc)

    # 3–4. Институт и кафедра
    fill_line(doc, "Институт", _strip_label(d["institute"], "институт"),
              "(наименование института)", indent_cm=1.6)
    fill_line(doc, "Кафедра", _strip_label(d["chair"], "кафедра"),
              "(наименование кафедры)", indent_cm=1.6)
    blank(doc)

    # 5. ДОПУЩЕН К ЗАЩИТЕ
    right_block(doc, [
        ("bold", "ДОПУЩЕН К ЗАЩИТЕ"),
        ("", "Заведующий кафедрой"),
        ("under", d["zavkafedra"]),
        ("cap", "(наименование кафедры)"),
        ("under", ""),
        ("cap", "(ученая степень, звание)"),
        ("", "подпись                      Ф.И.О."),
        ("", "«___» ________________ 20___ г."),
    ])
    blank(doc)

    # 6–8. Вид работы, тема, шифр ОП
    para(doc, work_type, bold=True, align=WD_ALIGN_PARAGRAPH.CENTER)
    blank(doc)
    theme = d["theme"].strip()
    if theme and not theme.startswith("«"):
        theme = "«%s»" % theme.rstrip("»")
    fill_line(doc, "На тему:", theme, "(тема %s)" % (
        "магистерской диссертации" if type_key == "magisterskaya" else "дипломной работы"),
        indent_cm=0.0)
    blank(doc)
    fill_line(doc, "", d["program"],
              "(шифр и наименование образовательной программы)", indent_cm=0.0)
    blank(doc)

    # 9–10. Выполнил / Рецензент / Научный руководитель
    tbl = doc.add_table(rows=2, cols=2)
    tbl.alignment = WD_TABLE_ALIGNMENT.CENTER
    tbl.autofit = False
    left, right = tbl.cell(0, 0), tbl.cell(0, 1)
    left.paragraphs[0].add_run("Выполнил")
    rp = right.paragraphs[0]
    rp.add_run("\u00a0" + (d["student"] if d["student"] else "\u00a0" * 30)).underline = True
    rp.add_run("\n(Ф.И.О. обучающегося)").italic = True
    rp.runs[-1].font.size = Pt(CAPTION_PT)
    for cell, title, who in ((tbl.cell(1, 0), "Рецензент", d["reviewer"]),
                             (tbl.cell(1, 1), "Научный руководитель", d["supervisor"])):
        cell.paragraphs[0].add_run(title)
        q = cell.add_paragraph()
        r = q.add_run(who if who else "\u00a0" * 24)
        r.underline = True
        q2 = cell.add_paragraph()
        c = q2.add_run("(ученая степень, звание)")
        c.italic = True
        c.font.size = Pt(CAPTION_PT)
        cell.add_paragraph("подпись        Ф.И.О.")
        cell.add_paragraph("«___» ____________ 20___ г.")

    blank(doc, 2)

    # 11. Город и год
    para(doc, "Алматы %s" % d["year"], align=WD_ALIGN_PARAGRAPH.CENTER)

    doc.save(out_path)


def _fix_settings(out_path):
    """python-docx ships ``<w:zoom w:val="bestFit"/>`` without the required ``w:percent``;
    patch it after saving so XSD validation (the docx skill's validate.py) passes."""
    with zipfile.ZipFile(out_path) as zin:
        infos = zin.infolist()
        parts = {i.filename: zin.read(i.filename) for i in infos}
    name = "word/settings.xml"
    if name in parts:
        xml = parts[name].decode("utf-8")
        if "<w:zoom" in xml and "w:percent" not in xml:
            xml = re.sub(r"<w:zoom([^>]*?)/>", r'<w:zoom\1 w:percent="100"/>', xml, count=1)
            parts[name] = xml.encode("utf-8")
            with zipfile.ZipFile(out_path, "w", zipfile.ZIP_DEFLATED) as zout:
                for info in infos:
                    zout.writestr(info, parts[info.filename])


def main():
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--type", default="diplom_rabota", choices=sorted(WORK_TYPES))
    ap.add_argument("--work-type", default="", help="override the work-type line")
    ap.add_argument("--theme", default="")
    ap.add_argument("--institute", default="")
    ap.add_argument("--chair", default="")
    ap.add_argument("--program", default="", help="шифр и наименование ОП")
    ap.add_argument("--student", default="")
    ap.add_argument("--supervisor", default="", help="научный руководитель (степень, звание, Ф.И.О.)")
    ap.add_argument("--reviewer", default="", help="рецензент (степень, звание, Ф.И.О.)")
    ap.add_argument("--zavkafedra", default="")
    ap.add_argument("--year", default="2026")
    ap.add_argument("-o", "--out", default="official_title_page.docx")
    args = ap.parse_args()

    d = dict(theme=args.theme, institute=args.institute, chair=args.chair,
             program=args.program, student=args.student, supervisor=args.supervisor,
             reviewer=args.reviewer, zavkafedra=args.zavkafedra, year=args.year)
    build(args.type, args.work_type or WORK_TYPES[args.type], d, args.out,
          master=args.type == "magisterskaya")
    _fix_settings(args.out)
    print(f"Wrote {args.out}  (type={args.type}, year={args.year})")


if __name__ == "__main__":
    main()
