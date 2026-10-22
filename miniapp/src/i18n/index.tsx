import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from 'react';
import { setFormatLang } from '@/lib/format';
import { DICT } from './dict';
import { detectLang, pluralPick, saveLang, type Lang } from './core';

/**
 * Interface language: ru (default), kk, en.
 *
 * Strings are keyed by their Russian text (gettext style): `t('Главная')`.
 * A missing translation falls back to Russian, so a partially translated build
 * never shows a raw key. The choice is stored on the device, applied to
 * `<html lang>` and pushed into the date/plural helpers of lib/format.
 */

export type { Lang } from './core';
export { LANGS, pluralPick } from './core';

/** Plain translator for non-React modules (error boundary, toasts). */
export function translateText(lang: Lang, source: string, vars?: Record<string, string | number>): string {
  const template = lang === 'ru' ? source : (DICT[lang][source] ?? source);
  if (!vars) return template;
  return template.replace(/\{(\w+)\}/g, (_, key: string) =>
    vars[key] === undefined ? `{${key}}` : String(vars[key]),
  );
}

export interface I18n {
  lang: Lang;
  setLang: (lang: Lang) => void;
  /** Translates a Russian source string; `{name}` placeholders are filled from vars. */
  t: (source: string, vars?: Record<string, string | number>) => string;
  /** Russian plural forms with per-language rules (`ru: [one, few, many]`). */
  n: (count: number, forms: { ru: [string, string, string]; kk: string; en: [string, string] }) => string;
}

const I18nContext = createContext<I18n>({
  lang: 'ru',
  setLang: () => {},
  t: (source) => source,
  n: (count, forms) => (count === 1 ? forms.en[0] : forms.en[1]),
});

function translate(lang: Lang, source: string, vars?: Record<string, string | number>): string {
  return translateText(lang, source, vars);
}

export function I18nProvider({ children }: { children: ReactNode }) {
  const [lang, setLangState] = useState<Lang>(() => detectLang());

  const setLang = useCallback((next: Lang) => {
    saveLang(next);
    setLangState(next);
  }, []);

  const t = useCallback(
    (source: string, vars?: Record<string, string | number>) => translate(lang, source, vars),
    [lang],
  );
  const n = useCallback(
    (count: number, forms: { ru: [string, string, string]; kk: string; en: [string, string] }) =>
      pluralPick(lang, count, forms),
    [lang],
  );

  // Applied during render: same value, so it is idempotent and always in sync.
  if (document.documentElement.lang !== lang) {
    document.documentElement.lang = lang;
  }
  setFormatLang(lang);

  const value = useMemo(() => ({ lang, setLang, t, n }), [lang, setLang, t, n]);

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

export function useI18n(): I18n {
  return useContext(I18nContext);
}

/** Shorthand for components that only need the translator. */
export function useT(): I18n['t'] {
  return useContext(I18nContext).t;
}
