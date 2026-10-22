---
name: satbayev-gost
description: >-
  Format, generate, and audit the text and graphic material of student works
  (учебные работы) of Satbayev University (НАО «Казахский национальный
  исследовательский технический университет имени К.И. Сатпаева», КазНИТУ) per the
  organization standard СТ КазНИТУ 09-2023 «Работы учебные…» (ред. №3). Also builds
  university title pages (титулки) for current assignments — СРС, СРСП, СРМ, СӨЖ, реферат,
  эссе, курсовая, лабораторная, практическая, семестровая, доклад, ИДЗ, РГР (forms
  Ф КазНИТУ 706-XX) in ru, kz, en. Use whenever the user mentions Satbayev, КазНИТУ,
  СТ КазНИТУ 09-2023, титулку/титульный лист, оформление учебных работ, дипломной
  работы/проекта, курсовой, магистерской диссертации, отчёта, реферата, аннотации,
  задания, содержания, списка литературы по ГОСТ 7.1-2003, ГОСТ 2.105, ГОСТ 8.417,
  полей/шрифта Times New Roman 14, нумерации страниц, таблиц, рисунков, формул, сносок,
  примечаний или приложений — even for a lone formatting check, title page, or bibliography.
---

# Satbayev GOST — student works per СТ КазНИТУ 09-2023

Encodes **СТ КазНИТУ 09-2023** (3rd ed., 20.04.2023): generate student works, audit an
existing `.docx` against the standard (report violations with clause numbers), answer
formatting questions. **Documents are written in Russian or Kazakh** (match the user's
language); these instructions are in English. Work from this file; open a file from
`references/` only when the task needs it — never read them all.

## Golden rules (details: `references/text-formatting.md`)

- A4 210×297 mm, single-sided, Word; margins top **20**, bottom **25**, left **30** mm,
  right **10** mm (**15 mm** for a master's thesis) (п. 5.1.3).
- Times New Roman; body **14 pt**, title page/annotation **12 pt**, table header may be
  12 pt; line spacing **single** ("через один"); uniform first-line indent ≈1.25 cm
  (five characters) (п. 5.1.4).
- Page numbers: continuous Arabic, **centred at the bottom, no dot**; the first printed
  number is on the page with «ВВЕДЕНИЕ» (the title page counts but shows no number) (п. 5.1.8).
- Structural headings — АННОТАЦИЯ, СОДЕРЖАНИЕ, ВВЕДЕНИЕ, ЗАКЛЮЧЕНИЕ/ВЫВОДЫ, СПИСОК
  ИСПОЛЬЗОВАННОЙ ЛИТЕРАТУРЫ, ПРИЛОЖЕНИЕ — are UPPERCASE, centred, never numbered (п. 4.4–4.12).
- Every section starts on a new page; no dot after section/subsection numbers (п. 5.2.2).
- Volume: diploma ≥ 30 pages, master's ≥ 60 (title pages and appendices excluded) (п. 4.1.2).

## Workflow

1. **Identify the work type and its elements** — Table 1 in `references/document-structure.md`.
   Never guess between diploma / course / master's / report: ask.
2. **Collect data** (theme, institute, chair, program cipher, names, year). Missing values
   keep the standard's underscore blanks — never invent them.
3. **Build** per the rules below. Title pages always come from a bundled script:
   - current assignments (СРС, реферат, лабораторная, курсовая, …) → `scripts/make_title_page.py`
     (1:1 clone of the matching page of the official «ТИТУЛКИ» file);
   - official diploma / master's / course title page → `scripts/make_official_title_page.py`.
4. **Check once** (read the output; do not write your own verifiers, do not search the disk):
   - `python scripts/validate_docx.py out.docx` — runs the sibling `docx` skill's XSD
     validator (the script finds it itself; falls back to a ZIP/XML sanity check);
   - for full works also `python scripts/check_docx.py out.docx [--type master]` — standard
     audit. **Skip `check_docx.py` for working title pages** (Ф КазНИТУ 706-XX): their
     geometry is the form's own, so its margin/size FAILs are expected noise.

## Cost discipline (keeps runs fast and cheap)

- Never open, copy, patch or diff `assets/titulki-template.docx` — the script clones the
  right page for you.
- Don't render PDFs/images to verify, don't convert files, don't write preview scripts
  (no LibreOffice in this environment). `scripts/validate_docx.py` plus
  `pandoc out.docx -t plain` (text order) are enough.
- Never search the filesystem for tools — both checkers live in `scripts/` next to this file.
- Read at most the one `references/` file the task needs; don't restate the rules in your
  answer — deliver the file and a short report. Fix/extend a document only when asked.

## Applying the rules to DOCX

```python
from docx import Document
from docx.shared import Mm, Pt, Cm
from docx.enum.text import WD_LINE_SPACING
from docx.oxml.ns import qn

doc = Document()
s = doc.sections[0]
s.page_width, s.page_height = Mm(210), Mm(297)
s.top_margin, s.bottom_margin = Mm(20), Mm(25)
s.left_margin, s.right_margin = Mm(30), Mm(10)      # Mm(15) for a master's thesis

st = doc.styles['Normal']
st.font.name = 'Times New Roman'
st.font.size = Pt(14)                                # titles/annotation — Pt(12)
st.element.rPr.rFonts.set(qn('w:eastAsia'), 'Times New Roman')
st.paragraph_format.line_spacing_rule = WD_LINE_SPACING.SINGLE
st.paragraph_format.space_before = st.paragraph_format.space_after = Pt(0)
st.paragraph_format.first_line_indent = Cm(1.25)     # ≈ five characters
```

- «Через один интервал» = **single**, not 1.5. Page numbers live in the footer, centred,
  and start showing from the «ВВЕДЕНИЕ» page (different-first-page footer or new section).
- Headings: own style, no automatic numbering, first letter capitalised, not bold/underlined
  unless the department demands it; structural headings are separate centred UPPERCASE paragraphs.

## DOCX toolkit — use the sibling `docx` skill (`../docx/`)

Its rules apply here: validate every produced file, and edit an existing `.docx` only via
unzip → edit `word/document.xml` → zip (never reformat the XML). Quick commands
(`DOCX=../docx` = the sibling skill, e.g. `~/.agents/skills/docx`):

| Need | Command |
|------|---------|
| Validate the produced `.docx` | `python scripts/validate_docx.py out.docx` (wrapper: finds the docx skill itself) |
| Auto-repair common issues | add `--auto-repair` |
| Coalesce fragmented runs before find/replace | `python $DOCX/scripts/merge_runs.py out.docx -o merged.docx` |
| Visual check (only where LibreOffice exists) | `python $DOCX/scripts/office/soffice.py --headless --convert-to pdf out.docx` |

**Always run `validate.py` on a generated or edited `.docx`** and fix what it reports.
For redlining/comments see the docx skill itself — those are rare here.

## Title pages (титулки)

Two kinds — never mix them up:

- **Working title page** (forms Ф КазНИТУ 706-XX) for current assignments — clone it with
  `scripts/make_title_page.py` (see `references/work-title-pages.md`). The clone keeps the
  form's own geometry (margins 1.5/1.5/1.5/2.5 cm), logo, criteria table and form-code line.
- **Official title page** for diploma / diploma project / master's / course —
  `scripts/make_official_title_page.py` (see `references/title-pages.md`), A4, margins
  20/25/30/10 mm (15 mm right for a master's thesis), 12 pt, «Алматы <год>» without «год».

```bash
# current assignment (RU): РЕФЕРАТ page of the ТИТУЛКИ file
python scripts/make_title_page.py --type referat --lang ru \
  --theme "Контроль качества на производстве" \
  --institute "Институт автоматики и информационных технологий" \
  --chair "Кафедра компьютерных наук" --student "Иванов И.И." \
  --teacher "Петров П.П." --specialty "6B06101" -o titul.docx

# Kazakh СӨЖ / English report / official diploma page
python scripts/make_title_page.py --type sors --lang kz --theme "Сандық құрылғылар" -o t.docx
python scripts/make_title_page.py --type referat --lang en --theme "Quality control" -o t.docx
python scripts/make_official_title_page.py --type diplom_rabota --theme "…" \
  --institute "…" --chair "…" --program "6B06101 «Информационные системы»" \
  --student "Иванов И.И." --supervisor "к.т.н., доцент Петров П.П." --year 2025 -o titul.docx
```

- **Signature block** (all languages): rebuilt as clean lines on the right side
  (8.5 cm indent, left-aligned), unused underline lines dropped. RU: `Преподаватель /
  Студент (Магистрант) / Шифр ОП`; KZ: `Оқытушы: / Студент: / Мамандық шифрі:`;
  EN keeps `Written by: / Group: / Checked by`. The specialty code goes to «Шифр ОП»
  (`--specialty`); `--group` is only used by the EN form. The KZ «ЗЕРТХАНАЛЫҚ ЖҰМЫС»
  page uses its own labels (`Білім алушының аты-жөні / Мамандық шифрі / Оқытушының аты-жөні`).
- **Template defects** are normalised automatically (and fixed in the shipped asset):
  the RU «Реферат» ministry line is **centred** (the raw file left it left-aligned),
  the artefact «Кафедра¬» is removed, «Ministy»→«Ministry», «Abstact»→«Abstract» and the
  Latin «A» in «ЛАБОРАТОРНАЯ» are corrected. Omitted fields keep the form's underscores.

## Quick answers (most frequent questions)

- **Таблица** — caption **above** with the paragraph indent, «Таблица N − Название» (no dot
  after the number); table from the left edge, page-wide, separated from text by one blank
  line; no «Номер по порядку» column (п. 5.5.2, 5.5.7).
- **Рисунок** — caption **below**, centred: `Рисунок N − Название`; explanatory data between
  the figure and the caption (п. 5.6.3).
- **Формула** — number right in parentheses at the formula's level; symbol list under the
  formula starts with «где» (no colon) from the paragraph indent; at least one blank line
  above and below (п. 5.4).
- **Приложение** — own numbering with the appendix letter: `Таблица А.1`, `Рисунок Б.2`,
  formula `(В.1)` (п. 4.12.2, 5.4.1).
- **References in text** — the source number in square brackets: `[1]` (п. 5.7).

## Reference files

| File | Read it for |
|------|-------------|
| `references/document-structure.md` | work composition, Table 1, annotation/abstract, contents, introduction, conclusion, glossary, volume (section 4) |
| `references/text-formatting.md` | margins, fonts, spacing, indents, page numbers, headings, lists (5.1–5.3) |
| `references/tables-figures-formulas.md` | tables, figures, formulas, references, notes, footnotes (5.4–5.9) |
| `references/citations-references.md` | citations and the bibliography per ГОСТ 7.1-2003 (4.11, 5.7, Приложение П) |
| `references/title-pages.md` | the official title page, cover and assignment samples (4.2, 4.3) |
| `references/work-title-pages.md` | working title pages (Ф КазНИТУ 706-XX), type keys, criteria tables, RU/KZ/EN |
| `references/graphic-material.md` | drawing formats, frames, title block, scales, posters (section 6) |
| `assets/checklist.md` | final manual check of a finished work |
| `scripts/check_docx.py` | automated `.docx` audit (`--kind body|title`, `--type master`) |
| `scripts/make_title_page.py` | working title page 1:1 from `assets/titulki-template.docx` (`--list` for types) |
| `scripts/make_official_title_page.py` | official diploma/master/course title page |

## Provenance

Source: «СТ КазНИТУ 09-2023. Работы учебные…», ред. №3 от 20.04.2023, утв. приказом
№173-П. Clause numbers in the reference files match this edition; reconcile against the
official text if the university issues a newer one.
