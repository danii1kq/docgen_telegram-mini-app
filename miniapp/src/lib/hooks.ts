import { useCallback, useEffect, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { ApiError, api } from '@/api/client';
import type { PublicSettings } from '@/api/types';
import { useI18n } from '@/i18n';
import { showPopup } from '@/telegram/sdk';
import { clearDraft, isDirty, loadDraft } from './draft';

/** Loads data once (and on demand) with the standard loading/error states. */
export function useAsync<T>(loader: () => Promise<T>, deps: unknown[]): {
  data: T | undefined;
  error: string | undefined;
  loading: boolean;
  reload: () => void;
} {
  const [data, setData] = useState<T | undefined>(undefined);
  const [error, setError] = useState<string | undefined>(undefined);
  const [loading, setLoading] = useState(true);
  const version = useRef(0);

  const run = useCallback(() => {
    const current = ++version.current;
    setLoading(true);
    setError(undefined);
    loader()
      .then((value) => {
        if (current === version.current) {
          setData(value);
          setLoading(false);
        }
      })
      .catch((cause: unknown) => {
        if (current === version.current) {
          setError(cause instanceof ApiError ? cause.message : 'Не удалось загрузить данные');
          setLoading(false);
        }
      });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);

  useEffect(() => {
    run();
    return () => {
      version.current += 1;
    };
  }, [run]);

  return { data, error, loading, reload: run };
}

/** Debounces a quickly changing value (quote recalculation, design.md §12.7). */
export function useDebounced<T>(value: T, delay = 300): T {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const timer = window.setTimeout(() => setDebounced(value), delay);
    return () => window.clearTimeout(timer);
  }, [value, delay]);
  return debounced;
}

/** Interval polling that stops when `enabled` is false. */
export function usePolling(callback: () => void, intervalMs: number, enabled: boolean): void {
  const ref = useRef(callback);
  ref.current = callback;
  useEffect(() => {
    if (!enabled || intervalMs <= 0) return;
    const timer = window.setInterval(() => ref.current(), intervalMs);
    return () => window.clearInterval(timer);
  }, [intervalMs, enabled]);
}

/** Latest value in a ref (stable Telegram button handlers). */
export function useLatest<T>(value: T): { current: T } {
  const ref = useRef(value);
  ref.current = value;
  return ref;
}

/** Public settings (support handle, maintenance, queue pause). */
export function useSettings(): PublicSettings | undefined {
  const { data } = useAsync(() => api.settings(), []);
  return data;
}

/** Jumps the window back to the top when `key` changes: the wizard steps and
 * route transitions must start at the top of the new content, not where the
 * previous screen was scrolled to. */
export function useScrollTop(key: unknown): void {
  useEffect(() => {
    window.scrollTo({ top: 0, left: 0 });
  }, [key]);
}

/**
 * Entry point of the ordering flow. A saved draft is never thrown away
 * silently: the client decides whether to continue it (the wizard opens on the
 * step they stopped at) or to start over (the draft is cleared).
 */
export function useOrderStart(): () => Promise<void> {
  const { t } = useI18n();
  const navigate = useNavigate();

  return useCallback(async () => {
    const draft = loadDraft();
    if (!isDirty(draft)) {
      navigate('/new');
      return;
    }
    const answer = await showPopup({
      title: t('Незавершённый заказ'),
      message: t('У вас есть незавершённое оформление. Продолжить с того же места или начать заново?'),
      buttons: [
        { id: 'continue', text: t('Продолжить'), type: 'default' },
        { id: 'restart', text: t('Начать заново'), type: 'destructive' },
        { id: 'cancel', text: t('Отмена'), type: 'cancel' },
      ],
    });
    if (answer === 'restart') {
      clearDraft();
      navigate('/new');
      return;
    }
    if (answer === 'continue') {
      navigate('/new');
    }
  }, [t, navigate]);
}
