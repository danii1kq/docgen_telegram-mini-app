/**
 * Thin, typed wrapper around the Telegram WebApp SDK (design.md §12.2/12.3).
 *
 * Every call is guarded: outside Telegram (plain browser preview, keyboard
 * button launches without initData) the helpers degrade to safe no-ops and the
 * app shows the gate screen.
 */

export const tg: TelegramWebApp | undefined = window.Telegram?.WebApp;

/** Dev-only escape hatch: `localStorage['srunal.dev.initData']` stands in for
 * Telegram outside the messenger (`npm run dev`, browser preview). Production
 * builds read only the signed SDK payload. */
const DEV_INIT_DATA_KEY = 'srunal.dev.initData';

export function devInitData(): string {
  if (!import.meta.env.DEV) return '';
  try {
    return window.localStorage.getItem(DEV_INIT_DATA_KEY) ?? '';
  } catch {
    return '';
  }
}

/** True when the native Telegram buttons (MainButton/BackButton) are usable.
 * A plain browser still defines window.Telegram.WebApp (the SDK script always
 * loads), but without initData and with platform "unknown": there the app
 * renders its own controls, exactly like the design mockups. */
export function hasNativeButtons(): boolean {
  return Boolean(tg && tg.initData && tg.platform !== 'unknown');
}

/** Raw initData string; empty outside Telegram. */
export function initData(): string {
  const signed = tg?.initData ?? '';
  return signed !== '' ? signed : devInitData();
}

/** True when the app runs inside Telegram with a signed payload. */
export function isMiniApp(): boolean {
  return Boolean(tg?.initData);
}

export function telegramUser(): TelegramUser | undefined {
  const user = tg?.initDataUnsafe?.user;
  if (user && user.id) return user;
  // Dev harness: parse the user out of the locally signed payload.
  const raw = devInitData();
  if (!raw) return undefined;
  try {
    const parsed = new URLSearchParams(raw).get('user');
    return parsed ? (JSON.parse(parsed) as TelegramUser) : undefined;
  } catch {
    return undefined;
  }
}

/** The `startapp` parameter of a Mini App deep link (already consumed or not). */
export function startParam(): string {
  return tg?.initDataUnsafe?.start_param ?? '';
}

export function versionAtLeast(version: string): boolean {
  try {
    return tg?.isVersionAtLeast?.(version) ?? false;
  } catch {
    return false;
  }
}

/** Applies the safe areas and subscribes to their changes. The palette and the
 * client chrome colours are owned by app/theme.ts (data-scheme). */
export function bootstrapTelegram(): () => void {
  setSafeArea();
  if (!tg) return () => {};

  tg.onEvent('safeAreaChanged', setSafeArea);
  tg.onEvent('contentSafeAreaChanged', setSafeArea);

  return () => {
    tg.offEvent('safeAreaChanged', setSafeArea);
    tg.offEvent('contentSafeAreaChanged', setSafeArea);
  };
}

/** Device notch + Telegram UI insets, with hard fallbacks (design.md §12.3). */
export function setSafeArea(): void {
  // max(), not a sum: the Telegram header already sits inside the device safe
  // area, and inside a non-fullscreen Mini App (Telegram Desktop/Web) both
  // insets are 0 - a hard floor there only pushes our own header down.
  const top = Math.max(tg?.contentSafeAreaInset?.top ?? 0, tg?.safeAreaInset?.top ?? 0);
  const bottom = Math.max(tg?.contentSafeAreaInset?.bottom ?? 0, tg?.safeAreaInset?.bottom ?? 0);
  const root = document.documentElement;
  root.style.setProperty('--safe-top', `${top}px`);
  root.style.setProperty('--safe-bottom', `${bottom}px`);
}

// ---------------------------------------------------------------------------
// haptics (never the only feedback, design.md §4.7)
// ---------------------------------------------------------------------------

export const haptic = {
  tap(): void {
    if (!versionAtLeast('6.1')) return;
    tg?.HapticFeedback?.impactOccurred('light');
  },
  select(): void {
    if (!versionAtLeast('6.1')) return;
    tg?.HapticFeedback?.selectionChanged();
  },
  impact(style: 'light' | 'medium' | 'heavy' | 'rigid' | 'soft' = 'medium'): void {
    if (!versionAtLeast('6.1')) return;
    tg?.HapticFeedback?.impactOccurred(style);
  },
  success(): void {
    if (!versionAtLeast('6.1')) return;
    tg?.HapticFeedback?.notificationOccurred('success');
  },
  error(): void {
    if (!versionAtLeast('6.1')) return;
    tg?.HapticFeedback?.notificationOccurred('error');
  },
  warning(): void {
    if (!versionAtLeast('6.1')) return;
    tg?.HapticFeedback?.notificationOccurred('warning');
  },
};

// ---------------------------------------------------------------------------
// dialogs / links
// ---------------------------------------------------------------------------

export function showAlert(message: string): void {
  if (tg && versionAtLeast('6.2')) {
    tg.showAlert(message);
    return;
  }
  window.alert(message);
}

export function showConfirm(message: string): Promise<boolean> {
  if (tg && versionAtLeast('6.2')) {
    return new Promise((resolve) => tg.showConfirm(message, (confirmed) => resolve(confirmed)));
  }
  return Promise.resolve(window.confirm(message));
}

export interface PopupButton {
  id: string;
  text: string;
  type?: 'default' | 'ok' | 'close' | 'cancel' | 'destructive';
}

export function showPopup(params: { title?: string; message: string; buttons: PopupButton[] }): Promise<string | undefined> {
  if (tg && versionAtLeast('6.2')) {
    return new Promise((resolve) =>
      tg.showPopup({ title: params.title, message: params.message, buttons: params.buttons }, (id) => resolve(id)),
    );
  }
  const answer = params.buttons.map((button) => button.text).join(' / ');
  return Promise.resolve(window.confirm(`${params.message}\n(${answer})`) ? params.buttons[0]?.id : undefined);
}

export function openLink(url: string): void {
  if (tg) {
    tg.openLink(url);
    return;
  }
  window.open(url, '_blank', 'noopener');
}

export function openTelegramLink(url: string): void {
  if (tg) {
    tg.openTelegramLink(url);
    return;
  }
  window.open(url, '_blank', 'noopener');
}

export function copyText(text: string): Promise<void> {
  if (tg?.readTextFromClipboard) {
    // writeText is not exposed by the SDK; the browser API works inside the WebView.
  }
  if (navigator.clipboard?.writeText) {
    return navigator.clipboard.writeText(text);
  }
  const area = document.createElement('textarea');
  area.value = text;
  area.style.position = 'fixed';
  area.style.opacity = '0';
  document.body.appendChild(area);
  area.select();
  try {
    document.execCommand('copy');
  } finally {
    document.body.removeChild(area);
  }
  return Promise.resolve();
}

/** Warns the user before closing the app while a draft is unsaved. */
export function setClosingConfirmation(enabled: boolean): void {
  try {
    if (enabled) {
      tg?.enableClosingConfirmation();
    } else {
      tg?.disableClosingConfirmation();
    }
  } catch {
    /* older clients */
  }
}

// ---------------------------------------------------------------------------
// storage (DeviceStorage → localStorage fallback, design.md §12.6)
// ---------------------------------------------------------------------------

/** DeviceStorage ships with Bot API 9.0; older clients get localStorage. */
function deviceStorageAvailable(): boolean {
  return Boolean(tg?.DeviceStorage) && versionAtLeast('9.0');
}

/**
 * Key/value store for the draft, the templates, the theme and the language.
 *
 * Both backends are written on every set: localStorage is synchronous and always
 * there, while Telegram's DeviceStorage (Bot API 9.0) is kept in sync so the
 * data follows the user across devices. Reads prefer localStorage (the write we
 * can be sure about) and fall back to DeviceStorage for values written by an
 * older build.
 */
export const deviceStorage = {
  get(key: string): string | null {
    try {
      const local = window.localStorage.getItem(key);
      if (local !== null && local !== '') return local;
    } catch {
      /* ignore */
    }
    if (deviceStorageAvailable()) {
      try {
        const value = tg?.DeviceStorage?.getItem(key);
        if (value !== undefined && value !== null && value !== '') return value;
      } catch {
        /* ignore */
      }
    }
    return null;
  },
  set(key: string, value: string): void {
    try {
      window.localStorage.setItem(key, value);
    } catch {
      /* ignore */
    }
    if (deviceStorageAvailable()) {
      try {
        tg?.DeviceStorage?.setItem(key, value);
      } catch {
        /* ignore */
      }
    }
  },
  remove(key: string): void {
    try {
      window.localStorage.removeItem(key);
    } catch {
      /* ignore */
    }
    if (deviceStorageAvailable()) {
      try {
        tg?.DeviceStorage?.removeItem(key);
      } catch {
        /* ignore */
      }
    }
  },
};
