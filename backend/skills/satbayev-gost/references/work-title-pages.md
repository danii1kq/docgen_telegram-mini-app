# Working title pages (forms Ф КазНИТУ 706-XX)

These are the title pages of **current assignments** (СРС, СРСП, реферат, курсовая,
лабораторная, практическая, эссе, ИДЗ …) with the assessment-criteria table and the
signature block. They differ from the **official** diploma/master title pages
(`title-pages.md`, `make_official_title_page.py`). Current default year: **2026**
(«Алматы 2026 г» / «Almaty 2026»).

> **How generation works.** `scripts/make_title_page.py` does **not** redraw the page — it
> clones the matching page from the official `assets/titulki-template.docx` (the university
> «ТИТУЛКИ» file) and substitutes the text only. Page size, margins (1.5 / 1.5 / 1.5 / 2.5 cm
> in the form), indents, alignments, fonts, spacing, the logo and the criteria table stay
> **1:1**. Documented deviations: the ministry line is **centred** (the raw file left the RU
> «Реферат» page left-aligned), the artefact «Кафедра¬» is removed, and the typos
> «Ministy»→«Ministry», «Abstact»→«Abstract», Latin «A»→«А» in «ЛАБОРАТОРНАЯ» are corrected
> (the same fixes live in the shipped asset and are applied on the fly to a replaced one).

## Type keys (`--type`)

| Key | Русский | Қазақша | English |
|-----|---------|---------|---------|
| `referat` | РЕФЕРАТ | РЕФЕРАТ | REPORT |
| `srs` | СРС | СӨЖ | STUDENT'S INDEPENDENT WORK |
| `srsp` | СРСП | СОӨЖ | SIS |
| `tsis` | — | — | TSIS (СРС+СРСП) |
| `srm` | СРМ | МӨОЖ | — |
| `srmp` | СРМП | — | — |
| `esse` | ЭССЕ | ЭССЕ | — |
| `kursovaya` | КУРСОВАЯ РАБОТА | КУРСТЫҚ ЖҰМЫС | — |
| `laboratornaya` | ЛАБОРАТОРНАЯ РАБОТА | ЛАБОРАТОРИЯЛЫҚ ЖҰМЫС | LABORATORY WORK |
| `zertkhanalyk` | — | ЗЕРТХАНАЛЫҚ ЖҰМЫС | — |
| `prakticheskaya` | ПРАКТИЧЕСКАЯ РАБОТА | ПРАКТИКАЛЫҚ ЖҰМЫС | — |
| `tazhiribelik` | — | ТӘЖІРБИЕЛІК ЖҰМЫС | — |
| `semestrovaya` | СЕМЕСТРОВАЯ РАБОТА | СЕМЕСТРЛІК ЖҰМЫС | — |
| `seminar` | Семинар | — | — |
| `doklad` | ДОКЛАД | БАЯНДАМА | — |
| `bayandama` | — | БАЯНДАМА (alias of `doklad`) | — |
| `idz` | ИДЗ | — | — |
| `samostoyatelnaya` | Самостоятельная работа | — | — |
| `otchet` | Отчет | Есеп беру | — |
| `kommercheskiy` | Коммерческий проект | — | — |
| `rubezhny` | Рубежный контроль | — | — |
| `rgr` | РГР | — | — |
| `zhut` | — | ЖҰТ | — |

Other accepted aliases: `sors`/`sozh`→`srs`, `report`→`referat` (en), `siw`→`srs` (en),
`sis`→`srsp` (en), `srs_srsp`→`tsis` (en), `kursovoy`→`kursovaya`. `--list` prints every
supported type/language pair. If no ready form exists for the requested type, clone the
closest page and override the title with `--title "КОНТРОЛЬНАЯ РАБОТА"`.

## Page structure

1. Header: ministry → university → institute → chair. The two header lines are
   exactly «МИНИСТЕРСТВО ОБРАЗОВАНИЯ И НАУКИ РЕСПУБЛИКИ КАЗАХСТАН» and the **short**
   university line: «УНИВЕРСИТЕТ САТПАЕВ» (ru) / «СӘТБАЕВ УНИВЕРСИТЕТІ» (kk) /
   «SATBAYEV UNIVERSITY» (en). Never print the long «Некоммерческое акционерное
   общество "Казахский национальный исследовательский технический университет
   имени К.И. Сатпаева"» wording on a working title page.
2. **Satbayev logo**, centred (inside the template).
3. **Work type** — large, centred («РЕФЕРАТ», «КУРСОВАЯ РАБОТА» …).
4. **Theme** with a place to fill.
5. **Assessment-criteria table** (per language).
6. **Signature block** (see below).
7. **City and year**: «Алматы 2026 г» / «Алматы 2026 ж» / «Almaty 2026».
8. Optional **form code line** at the bottom (e.g. «Ф КазНИТУ 706-02. СРС»), kept from the
   template 1:1; `--form-code "…"` overrides it, `--no-form-code` removes it. Never invent one.

## Signature block (rebuilt by the script)

The template right-aligns the lines and pads them with spaces, so a shorter value breaks the
columns and can leave a bare-underline line. The script rewrites the block for every
language as clean lines on the right side, left-aligned (fixed 8.5 cm indent), and drops
unused underline lines:

| Language | Lines printed |
|----------|---------------|
| ru | `Преподаватель` · `Студент` (or `Магистрант`) · **`Шифр ОП`** |
| kz | `Оқытушы:` · `Студент:` (or `Магистрант:`) · **`Мамандық шифрі:`** |
| en | `Written by:` · `Group:` · `Checked by` |

«Специальность» + «Группа» are merged into **Шифр ОП** (pass the code with `--specialty`);
the KZ form merges «Мамандығы» + «Тобы» into «Мамандық шифрі». `--group` is used by the EN
form only. The KZ «ЗЕРТХАНАЛЫҚ ЖҰМЫС» page uses its own block
(`Білім алушының аты-жөні / Мамандық шифрі / Оқытушының аты-жөні`) — the value goes on the
underline line below the label; if a field does not exist on the form (e.g. group there),
it is simply not printed.

## Assessment-criteria tables (from the template)

**RU (7 criteria):** Не выполнено 0% · Выполнено 0-50% · Самостоятельная систематизация
материала 0-10% · Выполнение требуемого объема и в указанный срок 0-5% · Использование
дополнительной научной литературы 0-5% · Уникальность выполненного задания 0-10% ·
Защита работы 0-20% · **Итого: 0-100%**.

**KZ (7 criteria):** Орындалған жоқ 0% · Орындалды 0-50% · Материялдық өзіндік жүйелендіру
0-10% · Талап етілген көлемде және көрсетілген мерзімде орындау 0-5% · Қосымша ғылыми
әдебиеттерді пайдалану 0-5% · Орындаған тапсырманың ерекшелігі 0-10% · СӨЖ-ді қорғау 0-20%
· **Қорытынды: 0-100%**.

**EN (5 criteria):** Abstract 30% · Introduction 20% · The main body 20% · Conclusion 20% ·
Literature 10%.

## Form codes seen in the file

`Ф КазНИТУ 706-02. СРС`, `Ф КазНИТУ 706-04. Семестровая работа`,
`Ф КазНИТУ 706-05. Лабораторная работа`, `Ф КазНИТУ 706-06. Курсовая работа`,
`Ф КазҰТЗУ 706-04. Зертханалық жұмыс`, `Ф КазҰТЗУ 706-06. СӨЖ`,
`ҚазҰТЗУ 706-04 Ү. Семестрлік жұмыс`, `ҚазНИТУ 706-04 Ү. Семестровая работа`.

## Choosing the form

1. Infer the **work type** from the prompt («сделай титулку для лабораторной» →
   `laboratornaya`); the **language** defaults to ru, switch for kz/en prompts.
2. Fill the user's data. **Omitted fields keep the template's underscore blanks** — never
   invent values (institute, chair, names, group, form code).
3. Year — **2026** («Алматы 2026 г» / «Алматы 2026 ж» / «Almaty 2026»).
4. Run `scripts/make_title_page.py` (examples in `SKILL.md`), then validate the result once
   with `python scripts/validate_docx.py titul.docx` (it finds the sibling `docx` skill's
   validator itself).
