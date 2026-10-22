import { deviceStorage, tg } from '@/telegram/sdk';

/**
 * Language core: no React here, so plain modules (lib/format.ts) can use it
 * without an import cycle with the provider.
 */

export type Lang = 'ru' | 'kk' | 'en';

export const LANGS: { value: Lang; label: string }[] = [
  { value: 'ru', label: 'Русский' },
  { value: 'kk', label: 'Қазақша' },
  { value: 'en', label: 'English' },
];

const LANG_KEY = 'srunal.mini.lang';

/** Stored choice wins, then the Telegram language_code, then Russian. */
export function detectLang(): Lang {
  const stored = deviceStorage.get(LANG_KEY);
  if (stored === 'ru' || stored === 'kk' || stored === 'en') return stored;
  const code = (tg?.initDataUnsafe?.user?.language_code ?? '').toLowerCase();
  if (code.startsWith('kk') || code.startsWith('kz')) return 'kk';
  if (code.startsWith('en')) return 'en';
  return 'ru';
}

export function saveLang(lang: Lang): void {
  deviceStorage.set(LANG_KEY, lang);
}

/** Russian/Kazakh/English plural picker. */
export function pluralPick(
  lang: Lang,
  count: number,
  forms: { ru: [string, string, string]; kk: string; en: [string, string] },
): string {
  if (lang === 'kk') return forms.kk;
  if (lang === 'en') return count === 1 ? forms.en[0] : forms.en[1];
  const abs = Math.abs(count);
  const mod10 = abs % 10;
  const mod100 = abs % 100;
  if (mod10 === 1 && mod100 !== 11) return forms.ru[0];
  if (mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14)) return forms.ru[1];
  return forms.ru[2];
}
