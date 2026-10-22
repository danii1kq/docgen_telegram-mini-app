/** Wizard option lists (mirrors the backend catalogue). */

export const WORK_KINDS = ['СРС', 'СРСП', 'СӨЖ', 'РГР', 'ИДЗ', 'Эссе', 'Другое'];

export const LANGUAGES = [
  { value: 'ru', label: 'Русский' },
  { value: 'kk', label: 'Қазақша' },
  { value: 'en', label: 'English' },
] as const;

/**
 * Institutes of Satbayev University (satbayev.university → Контакты / Институты).
 * `value` is the full official name and goes to the title page; `short` is for
 * compact UI (chips, subtitles).
 */
export interface InstituteOption {
  value: string;
  short: string;
}

export const INSTITUTES: InstituteOption[] = [
  { value: 'Институт автоматики и информационных технологий', short: 'ИАИТ' },
  { value: 'Горно-металлургический институт имени О.А. Байконурова', short: 'ГМИ' },
  { value: 'Институт геологии и нефтегазового дела имени К. Турысова', short: 'ИГНГД' },
  { value: 'Институт энергетики и машиностроения имени А. Буркитбаева', short: 'ИЭМ' },
  { value: 'Институт архитектуры и строительства имени Т.К. Басенова', short: 'ИАС' },
  { value: 'Институт управления проектами имени Е. Туркебаева', short: 'ИУП' },
  { value: 'Школа «Транспортная инженерия и логистика»', short: 'ШТИЛ' },
];

export const INSTITUTE_VALUES = INSTITUTES.map((item) => item.value);

/**
 * Departments of each institute (same source as the institutes). The wizard
 * shows the departments of the chosen institute first and «Другое» last.
 */
export const DEPARTMENTS: { institute: string; name: string }[] = [
  // Институт автоматики и информационных технологий
  { institute: 'ИАИТ', name: 'Программной инженерии' },
  { institute: 'ИАИТ', name: 'Информационных систем' },
  { institute: 'ИАИТ', name: 'Кибербезопасности' },
  { institute: 'ИАИТ', name: 'Автоматизации и управления' },
  { institute: 'ИАИТ', name: 'Электроники, телекоммуникации и космических технологий' },
  { institute: 'ИАИТ', name: 'Робототехники и технических средств автоматики' },
  { institute: 'ИАИТ', name: 'Высшей математики и моделирования' },
  { institute: 'ИАИТ', name: 'Инженерной механики' },
  // Горно-металлургический институт имени О.А. Байконурова
  { institute: 'ГМИ', name: 'Горного дела' },
  { institute: 'ГМИ', name: 'Металлургии и обогащения полезных ископаемых' },
  { institute: 'ГМИ', name: 'Маркшейдерского дела и геодезии' },
  { institute: 'ГМИ', name: 'Материаловедения, нанотехнологий и инженерной физики' },
  { institute: 'ГМИ', name: 'Химических процессов и промышленной экологии' },
  { institute: 'ГМИ', name: 'Горной робототехники и автоматизации добычи' },
  // Институт геологии и нефтегазового дела имени К. Турысова
  { institute: 'ИГНГД', name: 'Геологической съёмки, поиска и разведки месторождений полезных ископаемых' },
  { institute: 'ИГНГД', name: 'Геофизики и сейсмологии' },
  { institute: 'ИГНГД', name: 'Гидрогеологии, инженерной и нефтегазовой геологии' },
  { institute: 'ИГНГД', name: 'Нефтяной инженерии' },
  { institute: 'ИГНГД', name: 'Химической и биохимической инженерии' },
  // Институт энергетики и машиностроения имени А. Буркитбаева
  { institute: 'ИЭМ', name: 'Энергетики' },
  { institute: 'ИЭМ', name: 'Машиностроения' },
  { institute: 'ИЭМ', name: 'Технологических машин и газотурбинных установок' },
  { institute: 'ИЭМ', name: 'Стандартизации, сертификации и метрологии' },
  { institute: 'ИЭМ', name: 'Робототехники и мехатроники в машиностроении' },
  { institute: 'ИЭМ', name: 'Общей физики' },
  // Институт архитектуры и строительства имени Т.К. Басенова
  { institute: 'ИАС', name: 'Архитектуры' },
  { institute: 'ИАС', name: 'Строительства и строительных материалов' },
  { institute: 'ИАС', name: 'Инженерных систем и сетей' },
  // Институт управления проектами имени Е. Туркебаева
  { institute: 'ИУП', name: 'Менеджмента и математической экономики' },
  { institute: 'ИУП', name: 'Общественных дисциплин' },
  { institute: 'ИУП', name: 'Иностранных языков' },
  { institute: 'ИУП', name: 'Казахского и русского языков' },
  { institute: 'ИУП', name: 'Физической культуры' },
];

/** Short code of an institute value (for grouping the departments). */
export function instituteShort(value: string): string {
  return INSTITUTES.find((item) => item.value === value)?.short ?? '';
}

/** Departments of the chosen institute first, the rest after it. */
export function departmentOptions(institute: string): string[] {
  const short = instituteShort(institute);
  const own = DEPARTMENTS.filter((item) => item.institute === short).map((item) => item.name);
  const rest = DEPARTMENTS.filter((item) => item.institute !== short).map((item) => item.name);
  return [...own, ...rest];
}
