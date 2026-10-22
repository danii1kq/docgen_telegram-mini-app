import { deviceStorage, tg, versionAtLeast } from '@/telegram/sdk';

/**
 * Interface theme: follow Telegram (default) or force light/dark.
 *
 * "auto"    - the palette comes from the Telegram theme variables, as before;
 * "light"   - fixed light palette (the design's light tokens) even if Telegram
 *             is dark;
 * "dark"    - fixed dark palette.
 *
 * The choice is stored on the device and applied through data-scheme on <html>;
 * the CSS resolves the semantic tokens per scheme.
 */

export type ThemeMode = 'auto' | 'light' | 'dark';

const THEME_KEY = 'srunal.mini.theme';

export function loadThemeMode(): ThemeMode {
  const raw = deviceStorage.get(THEME_KEY);
  return raw === 'light' || raw === 'dark' || raw === 'auto' ? raw : 'auto';
}

export function saveThemeMode(mode: ThemeMode): void {
  deviceStorage.set(THEME_KEY, mode);
}

/** Resolves the mode to a concrete scheme ("auto" follows Telegram). */
export function resolveScheme(mode: ThemeMode = loadThemeMode()): 'light' | 'dark' {
  if (mode === 'light' || mode === 'dark') return mode;
  if (tg?.colorScheme === 'dark') return 'dark';
  if (tg?.colorScheme === 'light') return 'light';
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

/**
 * Applies the stored mode: the scheme for the palette plus the client chrome
 * colours. In "auto" the Telegram theme owns them, in the forced modes the
 * fixed palette does.
 */
export function applyThemeMode(mode: ThemeMode = loadThemeMode()): void {
  const scheme = resolveScheme(mode);
  document.documentElement.dataset.scheme = scheme;
  document.documentElement.dataset.themeMode = mode;
  if (!tg) return;

  const followTelegram = mode === 'auto' && versionAtLeast('7.0');
  const bar = followTelegram ? 'secondary_bg_color' : scheme === 'dark' ? '#0E1621' : '#EFEFF4';
  try {
    // Header/background colours need 6.1; the bottom bar needs 7.10.
    if (versionAtLeast('6.1')) {
      tg.setHeaderColor(bar);
      tg.setBackgroundColor(bar);
    }
    if (versionAtLeast('7.10')) {
      tg.setBottomBarColor?.(bar);
    }
  } catch {
    /* older clients keep their defaults */
  }
}

/** Applies the stored theme and keeps "auto" in sync with Telegram. */
export function initTheme(): () => void {
  applyThemeMode();
  const client = tg;
  if (!client) return () => {};
  const onThemeChanged = () => applyThemeMode();
  client.onEvent('themeChanged', onThemeChanged);
  return () => client.offEvent('themeChanged', onThemeChanged);
}
