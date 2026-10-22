# Work composition (section 4 of СТ КазНИТУ 09-2023)

## 4.1 Elements and volume

In order (п. 4.1.1): обложка; титульный лист; задание; аннотация (статья — реферат);
содержание; введение; основная часть; заключение; перечень принятых терминов, перечень
сокращений; список использованной литературы; приложения; спецификации. The set depends on
the work type (Table 1) and may be extended by the assignment.

### Table 1 — elements per work type

Legend: **+** mandatory, **−** absent, **\*** optional. Types: **Дп/Др** diploma work/project
· **Кп/Кр** course work/project · **Мд** master's dissertation · **Практика** Ппр/Упр ·
**НИРО** research work of the student · **Статья** scientific article · **РГР** ·
**К/р** контрольная · **Л/р** лабораторная.

| Element | Дп/Др | Кп/Кр | Мд | Практика | НИРО | Статья | РГР | К/р | Л/р |
|---------|:---:|:---:|:---:|:---:|:---:|:---:|:---:|:---:|:---:|
| Обложка | + | − | − | − | − | − | − | − | − |
| Титульный лист | + | + | + | + | + | − | + | + | + |
| Задание | + | + | + | + | + | − | + | + | − |
| Аннотация (статья — реферат) | + | − | + | − | + | + | − | − | − |
| Содержание | + | + | + | + | + | − | − | − | − |
| Введение | + | + | + | + | + | − | − | − | − |
| Основная часть | + | + | + | + | + | + | + | + | + |
| Заключение | + | + | + | + | + | − | + | − | + |
| Перечень сокращений, терминов | + | + | + | + | + | − | − | − | − |
| Список использованной литературы | + | + | + | + | + | + | + | + | − |
| Приложения | + | + | + | − | + | − | + | − | − |
| Спецификации | + | + | − | + | − | − | − | − | − |

> In the standard's table «Обложка» and «Титульный лист» share one label cell but their
> values differ — they are split here so the two are not mixed. «Практика» in the standard
> is «Отчет о Ппр, Упр»; «\*» marks optional elements in the source table.

**Volume (п. 4.1.2):** diploma work (project) — **≥ 30 pages**; master's dissertation —
**≥ 60 pages**. Title pages and appendices are not counted.

## 4.2 Title page

A4 sheets; fields 1–13 with the alignments in `title-pages.md` (п. 4.2.1). Institute/chair
names in lowercase except the first letter; work type and «ДОПУЩЕН К ЗАЩИТЕ» in UPPERCASE;
city and year without «год»/«г» (field 13). Samples — Приложения А, Б, Г, Д, Ж, Л, М, Н;
generate with `scripts/make_official_title_page.py`.

## 4.3 Задание

Issued by the supervisor; samples — Приложения В, Е, И.

## 4.4 Аннотация

- Brief characterisation of the work and its purpose.
- On **Kazakh, Russian and a foreign language**, **on one page**.
- Contains: work type, subject/object of the presentation and its main characteristics (goal,
  topic and aspect), distinctive features (problem statement, solution, recommendations).
- Must **not** retell the work.
- Heading **«АННОТАЦИЯ»** — UPPERCASE, centred, not numbered. Size — **12**.

## 4.5 Реферат (для статей)

Per ГОСТ 7.9-95: data on volume, illustrations, tables, sources; **5–15 keywords**; text per
plan: abstract volume (**≤ 500 characters**), subject, nature and goal of the work.

## 4.6 Содержание

- Section/subsection numbers and names plus structural elements, with page numbers.
- Counts toward the total page count.
- Heading **«СОДЕРЖАНИЕ»** — UPPERCASE, centred, not numbered.
- Names — lowercase except the first capital.
- **Do not fill the space up to the page number with dots** (no dot leaders).

## 4.7 Введение

- Heading **«ВВЕДЕНИЕ»** — UPPERCASE, centred, not numbered.
- Briefly characterises the state of the problem; states the **goal**, **tasks**, novelty,
  relevance, theoretical and methodological basis, practical base and why the work was done.

## 4.8 Основная часть

- Sections per the requirements of the graduating department.
- Data showing the essence, content, methods and main results of the work.

## 4.9 Заключение. Выводы

- **Заключение** evaluates the results against the assignment, gives the technical/economic
  assessment and the value of the work; states what the work ended with.
- Heading **«ЗАКЛЮЧЕНИЕ»** — UPPERCASE, centred, not numbered.
- Lab reports use **«ВЫВОДЫ»** instead (centred, uppercase, not numbered) with experiment
  analysis and comparison against theory.

## 4.10 Перечень терминов. Перечень сокращений

- With specific terminology, a **list of accepted terms** with explanations goes before the
  bibliography and is included in the contents.
- Abbreviations per **ГОСТ 2.316** and **ГОСТ 7.12**; the **list of abbreviations** comes
  before the list of terms.

## 4.11 Список использованной литературы

- Sources **in order of appearance** in the text.
- In-text citations — the number in square brackets: `[1]`.
- Heading **«СПИСОК ИСПОЛЬЗОВАННОЙ ЛИТЕРАТУРЫ»** — UPPERCASE, centred, not numbered.
- Sources numbered with Arabic digits, **with the paragraph indent, no dot**; bibliographic
  description per **ГОСТ 7.1-2003** (samples — Приложение П, `citations-references.md`).

## 4.12 Приложения

- Each appendix starts on a **new page**.
- If there are several, they are lettered with Russian capitals from **А**, skipping
  **Ё, З, Й, О, Ч, Ы, Ъ, Ь**.
- The word **«Приложение»** — centred, lowercase except the first letter, plus the letter
  («Приложение А»).
- May contain graphic material, large tables, calculations, descriptions.
- Appendix content may be divided into sections; formulas, tables and figures are numbered
  **within the appendix** with its letter: `Таблица А.1`, `Рисунок А.3`, формула `(В.1)`.
- On following sheets write **«Продолжение А»**.
- Appendices share the **continuous page numbering** of the work.
