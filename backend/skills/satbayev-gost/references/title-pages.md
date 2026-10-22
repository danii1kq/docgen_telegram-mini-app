# Official title page, cover and assignment (п. 4.2, 4.3; Приложения А, Б, Г, Д, Ж, Л, М, Н)

> This is the **official** title page (diploma work/project, master's dissertation, course,
> practice report, scientific article). Generate it with
> `scripts/make_official_title_page.py` — it reproduces Приложение Б/Ж/М (A4, margins
> 20/25/30/10 mm — 15 mm right for a master's, Times New Roman 12 pt, single spacing).
> For **current assignments** (СРС, СРСП, реферат, эссе, лабораторная …) use
> `work-title-pages.md` and `scripts/make_title_page.py`.

## Field map

Fields 1–4 are **centred**. Fields 5–6 sit at the **right edge, left-aligned block**.
Fields 7–9 are centred. Field 10 — «Выполнил» left, name right. Fields 11–12 — reviewer and
supervisor. Field 13 — city and year **without** «год»/«г.».

| № | Field |
|---|-------|
| 1 | МИНИСТЕРСТВО НАУКИ И ВЫСШЕГО ОБРАЗОВАНИЯ РЕСПУБЛИКИ КАЗАХСТАН |
| 2 | Некоммерческое акционерное общество «Казахский национальный исследовательский технический университет имени К.И. Сатпаева» |
| 3 | Институт (наименование) |
| 4 | Кафедра (наименование) |
| 5 | Restrictive stamp (if any) |
| 6 | ДОПУЩЕН К ЗАЩИТЕ — заведующий кафедрой, signature, name, date |
| 7 | Work type (UPPERCASE): ДИПЛОМНАЯ РАБОТА / ДИПЛОМНЫЙ ПРОЕКТ / МАГИСТЕРСКАЯ ДИССЕРТАЦИЯ |
| 8 | Тема: «…» |
| 9 | Шифр и наименование образовательной программы |
| 10 | Выполнил: Ф.И.О. обучающегося |
| 11 | Рецензент: учёная степень, звание, подпись, Ф.И.О., дата |
| 12 | Научный руководитель: учёная степень, звание, подпись, Ф.И.О., дата |
| 13 | Алматы 2026 |

## Cover sample (Приложение А/Г)

```
             МИНИСТЕРСТВО НАУКИ И ВЫСШЕГО ОБРАЗОВАНИЯ
                    РЕСПУБЛИКИ КАЗАХСТАН

    Некоммерческое акционерное общество «Казахский национальный
      исследовательский технический университет имени К.И. Сатпаева»

                              Институт ______________
                              (наименование института)

                              Кафедра ______________
                              (наименование кафедры)

                          <Ф.И.О. обучающегося>

              <тема дипломной работы / магистерской диссертации>

                     ДИПЛОМНАЯ РАБОТА
                  (МАГИСТЕРСКАЯ ДИССЕРТАЦИЯ)

              <шифр и наименование образовательной программы>

                              Алматы 2026
```

## Title page sample (Приложение Б/Д/Ж/М) — reproduced by the script

```
             МИНИСТЕРСТВО НАУКИ И ВЫСШЕГО ОБРАЗОВАНИЯ
                    РЕСПУБЛИКИ КАЗАХСТАН

    Некоммерческое акционерное общество «Казахский национальный
      исследовательский технический университет имени К.И. Сатпаева»

                              Институт ______________
                              (наименование института)

                              Кафедра ______________
                              (наименование кафедры)

                                                ДОПУЩЕН К ЗАЩИТЕ
                                                Заведующий кафедрой ______
                                                (наименование кафедры)
                                                ______________
                                                (ученая степень, звание)
                                                подпись   Ф.И.О.
                                                «___» ________ 20__ г.


                            ДИПЛОМНАЯ РАБОТА

                     На тему: «_______________________________»
                              (тема дипломной работы)

              <шифр и наименование образовательной программы>

 Выполнил                                            Рецензент
 ______________                                      ______________
 (Ф.И.О. обучающегося)                               (ученая степень, звание)
                                                     подпись   Ф.И.О.
                                                     «___» ______ 20__ г.

                                                     Научный руководитель
                                                     ______________
                                                     (ученая степень, звание)
                                                     подпись   Ф.И.О.
                                                     «___» ______ 20__ г.


                              Алматы 2026
```

> ДИПЛОМНЫЙ ПРОЕКТ — Приложения Г, Д; МАГИСТЕРСКАЯ ДИССЕРТАЦИЯ — Ж, М; КУРСОВАЯ РАБОТА —
> Приложение К; ОТЧЁТ О ПРАКТИКЕ — Н/Л. Override the type line freely with `--work-type`.
> `--type magisterskaya` switches the right margin to 15 mm and the theme caption to
> «(тема магистерской диссертации)».

## Assignment (задание)

Per Приложения В/Е/И: header (fields 1–4, theme, student, supervisor), the task wording, the
list of required graphic/text material, a calendar plan and signatures. «\_» marks a blank.

## Formatting notes

- Title page and annotation — **12 pt** (body text — 14 pt).
- Margins (п. 5.1.3): top 20, bottom 25, left 30, right 10 mm (15 mm for a master's).
- Page numbering is continuous **from the title page**, but **no number is printed on it**;
  the first visible number is on the «ВВЕДЕНИЕ» page (п. 5.1.8).
- Institute/chair names — lowercase except the first capital; work type and
  «ДОПУЩЕН К ЗАЩИТЕ» — UPPERCASE.
- Year — «**Алматы 2026**», without «год»/«г.».
