import type { Lang } from './core';

/**
 * Language-specific catalogue pieces: product names, unit words and status
 * labels. The server always speaks Russian (prices, limits, messages), so the
 * client translates the fixed vocabulary here, keyed by the API codes.
 */

const PRODUCT_NAME: Record<string, Record<Lang, string>> = {
  essay: { ru: 'Реферат', kk: 'Реферат', en: 'Essay' },
  lab: { ru: 'Лабораторная', kk: 'Зертханалық жұмыс', en: 'Lab report' },
  presentation: { ru: 'Презентация', kk: 'Презентация', en: 'Presentation' },
  other: { ru: 'Другой док.', kk: 'Басқа жұмыс', en: 'Other work' },
};

/** Unit words per language, keyed by the Russian singular from the API. */
const UNITS: Record<string, Record<Lang, [string, string, string]>> = {
  страница: { ru: ['страница', 'страницы', 'страниц'], kk: ['бет', 'бет', 'бет'], en: ['page', 'pages', 'pages'] },
  слайд: { ru: ['слайд', 'слайда', 'слайдов'], kk: ['слайд', 'слайд', 'слайд'], en: ['slide', 'slides', 'slides'] },
  работа: { ru: ['работа', 'работы', 'работ'], kk: ['жұмыс', 'жұмыс', 'жұмыс'], en: ['work', 'works', 'works'] },
};

const UNIT_SHORT: Record<string, Record<Lang, string>> = {
  страница: { ru: 'стр.', kk: 'бет', en: 'p.' },
  слайд: { ru: 'слайд', kk: 'слайд', en: 'sl.' },
  работа: { ru: 'раб.', kk: 'жұмыс', en: 'w.' },
};

const STATUS_LABEL: Record<string, Record<Lang, string>> = {
  awaiting_payment: { ru: 'Ожидает оплату', kk: 'Төлемді күтуде', en: 'Awaiting payment' },
  payment_claimed: { ru: 'Оплата проверяется', kk: 'Төлем тексерілуде', en: 'Checking the payment' },
  payment_rejected: { ru: 'Оплата отклонена', kk: 'Төлем қабылданбады', en: 'Payment rejected' },
  queued: { ru: 'В очереди', kk: 'Кезекте', en: 'Queued' },
  generating: { ru: 'Готовим работу', kk: 'Жұмыс дайындалуда', en: 'Preparing the work' },
  completed: { ru: 'Готово', kk: 'Дайын', en: 'Ready' },
  failed_final: { ru: 'Проблема', kk: 'Мәселе', en: 'Problem' },
  expired: { ru: 'Истёк', kk: 'Мерзімі өтті', en: 'Expired' },
  cancelled: { ru: 'Отменён', kk: 'Тоқтатылды', en: 'Cancelled' },
};

/** The 4-step progress tracker of the order screen. */
const PROGRESS_STEP: Record<string, Record<Lang, string>> = {
  Оплата: { ru: 'Оплата', kk: 'Төлем', en: 'Payment' },
  Проверка: { ru: 'Проверка', kk: 'Тексеру', en: 'Check' },
  'В работе': { ru: 'В работе', kk: 'Жұмыста', en: 'In progress' },
  Готово: { ru: 'Готово', kk: 'Дайын', en: 'Ready' },
};

export function productName(lang: Lang, code: string, serverName?: string): string {
  const map = PRODUCT_NAME[code];
  if (!map) return serverName ?? code;
  return map[lang] ?? map.ru;
}

export function unitForms(lang: Lang, unitOne: string | undefined): [string, string, string] {
  const key = (unitOne ?? 'работа').trim();
  const map = UNITS[key];
  if (!map) return [key, key, key];
  return map[lang] ?? map.ru;
}

export function unitShort(lang: Lang, unitOne: string | undefined): string {
  const key = (unitOne ?? 'работа').trim();
  const map = UNIT_SHORT[key];
  if (!map) return key;
  return map[lang] ?? map.ru;
}

export function statusLabel(lang: Lang, status: string): string {
  const map = STATUS_LABEL[status];
  if (!map) return status;
  return map[lang] ?? map.ru;
}

export function progressStepLabel(lang: Lang, ruLabel: string): string {
  const map = PROGRESS_STEP[ruLabel];
  if (!map) return ruLabel;
  return map[lang] ?? map.ru;
}

// ---------------------------------------------------------------------------
// Quote lines: the backend builds them in Russian (`pricing.go`), the client
// translates the fixed vocabulary (product, unit words, labels) and keeps the
// numbers untouched.
// ---------------------------------------------------------------------------

/** Unit titles as they appear in the volume line ("Слайды (6 × 240 ₸)"). */
const UNIT_TITLE: Record<string, Record<Lang, string>> = {
  страницы: { ru: 'Страницы', kk: 'Беттер', en: 'Pages' },
  слайды: { ru: 'Слайды', kk: 'Слайдтар', en: 'Slides' },
  работы: { ru: 'Работы', kk: 'Жұмыстар', en: 'Works' },
};

const QUOTE_LABEL: Record<string, Record<Lang, string>> = {
  Объём: { ru: 'Объём', kk: 'Көлемі', en: 'Volume' },
  'Форматирование по ГОСТ РК': {
    ru: 'Форматирование по ГОСТ РК',
    kk: 'ГОСТ РК бойынша рәсімдеу',
    en: 'ГОСТ RK formatting',
  },
  'Количество работ': { ru: 'Количество работ', kk: 'Жұмыс саны', en: 'Number of works' },
  'Скидка за объём': { ru: 'Скидка за объём', kk: 'Көлем жеңілдігі', en: 'Volume discount' },
};

const BASE_SUFFIX = ' (базовый тариф)';

export function quoteLineLabel(lang: Lang, label: string): string {
  if (lang === 'ru') return label;

  if (label.endsWith(BASE_SUFFIX)) {
    const name = label.slice(0, -BASE_SUFFIX.length);
    const code = Object.keys(PRODUCT_NAME).find((key) => PRODUCT_NAME[key].ru === name);
    const translated = code ? PRODUCT_NAME[code][lang] : name;
    return translated + (lang === 'kk' ? ' (базалық тариф)' : ' (base rate)');
  }

  if (label.startsWith('Промокод ')) {
    return (lang === 'kk' ? 'Промокод ' : 'Promo code ') + label.slice('Промокод '.length);
  }

  for (const [title, map] of Object.entries(UNIT_TITLE)) {
    if (label.startsWith(`${title} (`)) {
      const rest = label
        .slice(title.length)
        .replace(/ × (\d+) работ/, (_match, count: string) => ` × ${count} ${lang === 'kk' ? 'жұмыс' : 'works'}`);
      return map[lang] + rest;
    }
  }

  return QUOTE_LABEL[label]?.[lang] ?? label;
}

const QUOTE_VALUE: Record<string, Record<Lang, string>> = {
  'По заданию методички': {
    ru: 'По заданию методички',
    kk: 'Әдістемелік нұсқаулық бойынша',
    en: 'As per the manual',
  },
  'Включено (0 ₸)': { ru: 'Включено (0 ₸)', kk: 'Қосылған (0 ₸)', en: 'Included (0 ₸)' },
};

export function quoteLineValue(lang: Lang, line: { label: string; value: string }): string {
  if (lang === 'ru') return line.value;

  const mapped = QUOTE_VALUE[line.value];
  if (mapped) return mapped[lang];

  const pieces = /^(\d+) шт\.$/.exec(line.value);
  if (pieces) return lang === 'kk' ? `${pieces[1]} дана` : `${pieces[1]} pcs.`;

  if (line.label.endsWith(BASE_SUFFIX)) {
    const parts = line.value.split(' / ');
    if (parts.length === 2) return `${parts[0]} / ${unitShort(lang, parts[1])}`;
  }

  return line.value;
}
