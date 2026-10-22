/** Money / date / plural rendering, mirroring the backend's format package. */

import type { Lang } from '@/i18n/core';
import { pluralPick } from '@/i18n/core';
import { unitForms } from '@/i18n/catalog';

const NBSP = '\u00A0';

/** Interface language for dates and unit words; set by the I18n provider. */
let LANG: Lang = 'ru';

export function setFormatLang(lang: Lang): void {
  LANG = lang;
}

/** 1240 → "1 240 ₸" (non-breaking spaces, never "1000₸"). */
export function money(amount: number): string {
  const grouped = Math.round(amount)
    .toString()
    .replace(/\B(?=(\d{3})+(?!\d))/g, NBSP);
  return `${grouped}${NBSP}₸`;
}

const MONTHS_SHORT: Record<Lang, string[]> = {
  ru: ['янв', 'фев', 'мар', 'апр', 'мая', 'июн', 'июл', 'авг', 'сен', 'окт', 'ноя', 'дек'],
  kk: ['қаң', 'ақп', 'нау', 'сәу', 'мам', 'мау', 'шіл', 'там', 'қыр', 'қаз', 'қар', 'жел'],
  en: ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'],
};

const MONTHS_FULL: Record<Lang, string[]> = {
  ru: ['Январь', 'Февраль', 'Март', 'Апрель', 'Май', 'Июнь', 'Июль', 'Август', 'Сентябрь', 'Октябрь', 'Ноябрь', 'Декабрь'],
  kk: ['Қаңтар', 'Ақпан', 'Наурыз', 'Сәуір', 'Мамыр', 'Маусым', 'Шілде', 'Тамыз', 'Қыркүйек', 'Қазан', 'Қараша', 'Желтоқсан'],
  en: ['January', 'February', 'March', 'April', 'May', 'June', 'July', 'August', 'September', 'October', 'November', 'December'],
};

const TODAY: Record<Lang, string> = { ru: 'сегодня', kk: 'бүгін', en: 'today' };
const YESTERDAY: Record<Lang, string> = { ru: 'вчера', kk: 'кеше', en: 'yesterday' };

/** Display timezone: Asia/Almaty (design.md §10.3). */
const TZ = 'Asia/Almaty';

const dayFormatter = new Intl.DateTimeFormat('ru-RU', {
  timeZone: TZ,
  day: 'numeric',
  month: 'numeric',
  year: 'numeric',
});

export function formatTime(iso: string): string {
  const parts = timeParts(iso);
  return parts ? `${String(parts.hour).padStart(2, '0')}:${String(parts.minute).padStart(2, '0')}` : '';
}

/** "17 окт, 14:05" / "сегодня, 14:05" / "вчера, 09:40". */
export function formatDateTime(iso: string): string {
  const date = parse(iso);
  if (!date) return '';
  const time = formatTime(iso);
  const days = dayDiff(date);
  if (days === 0) return `${TODAY[LANG]}, ${time}`;
  if (days === 1) return `${YESTERDAY[LANG]}, ${time}`;
  return `${formatDay(date)}, ${time}`;
}

/** "17 окт" - day and short month in the display timezone. */
export function formatDay(iso: string | Date): string {
  const date = typeof iso === 'string' ? parse(iso) : iso;
  if (!date || Number.isNaN(date.getTime())) return '';
  const day = Number(dayFormatter.format(date).split('.')[0]);
  return `${day} ${MONTHS_SHORT[LANG][monthOf(date)]}`;
}

/** "окт 2026" group labels for the orders list. */
export function monthLabel(iso: string): string {
  const date = parse(iso);
  if (!date) return '';
  return `${MONTHS_FULL[LANG][monthOf(date)]} ${yearOf(date)}`;
}

/** "412 КБ" / "1,2 МБ". */
export function formatSize(bytes: number): string {
  const sep = LANG === 'en' ? '.' : ',';
  const b = LANG === 'en' ? 'B' : 'Б';
  const kb = LANG === 'en' ? 'KB' : 'КБ';
  const mb = LANG === 'en' ? 'MB' : 'МБ';
  if (bytes < 1024) return `${bytes} ${b}`;
  if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} ${kb}`;
  return `${(bytes / (1024 * 1024)).toFixed(1).replace('.', sep)} ${mb}`;
}

/** Russian plural forms: plural(5, 'страница', 'страницы', 'страниц'). */
export function plural(count: number, one: string, few: string, many: string): string {
  const mod100 = Math.abs(count) % 100;
  const mod10 = mod100 % 10;
  if (mod100 >= 11 && mod100 <= 14) return many;
  if (mod10 === 1) return one;
  if (mod10 >= 2 && mod10 <= 4) return few;
  return many;
}

export function unitsLabel(count: number, product: { unitOne: string; unitFew: string; unitMany: string } | undefined): string {
  if (!product) return String(count);
  if (LANG === 'ru') return `${count} ${plural(count, product.unitOne, product.unitFew, product.unitMany)}`;
  const forms = unitForms(LANG, product.unitOne);
  const word = pluralPick(LANG, count, {
    ru: [product.unitOne, product.unitFew, product.unitMany],
    kk: forms[0],
    en: [forms[0], forms[1]],
  });
  return `${count} ${word}`;
}

export function worksLabel(count: number): string {
  const forms = unitForms(LANG, 'работа');
  const word = pluralPick(LANG, count, {
    ru: ['работа', 'работы', 'работ'],
    kk: forms[0],
    en: [forms[0], forms[1]],
  });
  return `${count} ${word}`;
}

// --- internals -------------------------------------------------------------

function parse(iso: string): Date | undefined {
  if (!iso) return undefined;
  const date = new Date(iso);
  return Number.isNaN(date.getTime()) ? undefined : date;
}

interface TimeParts {
  hour: number;
  minute: number;
  day: number;
  month: number;
  year: number;
}

const timeFormatter = new Intl.DateTimeFormat('ru-RU', {
  timeZone: TZ,
  hour: '2-digit',
  minute: '2-digit',
  day: '2-digit',
  month: '2-digit',
  year: 'numeric',
  hour12: false,
});

function timeParts(iso: string): { hour: number; minute: number } | undefined {
  const date = parse(iso);
  if (!date) return undefined;
  const parts = timeFormatter.formatToParts(date);
  const pick = (type: string) => Number(parts.find((part) => part.type === type)?.value ?? '0');
  return { hour: pick('hour'), minute: pick('minute') };
}

function zonedParts(date: Date): TimeParts {
  const parts = timeFormatter.formatToParts(date);
  const pick = (type: string) => Number(parts.find((part) => part.type === type)?.value ?? '0');
  return {
    hour: pick('hour'),
    minute: pick('minute'),
    day: pick('day'),
    month: pick('month'),
    year: pick('year'),
  };
}

function monthOf(date: Date): number {
  return Math.max(0, Math.min(11, zonedParts(date).month - 1));
}

function yearOf(date: Date): number {
  return zonedParts(date).year;
}

/** Calendar-day difference (in the display timezone) between today and the date. */
function dayDiff(date: Date): number {
  const now = zonedParts(new Date());
  const then = zonedParts(date);
  const current = Date.UTC(now.year, now.month - 1, now.day);
  const target = Date.UTC(then.year, then.month - 1, then.day);
  return Math.round((current - target) / (24 * 60 * 60 * 1000));
}
