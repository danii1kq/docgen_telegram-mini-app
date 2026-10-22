import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react';
import { haptic } from '@/telegram/sdk';

/** Lightweight toast (success/error feedback, design.md §12.5). */
interface ToastState {
  text: string;
  tone: 'success' | 'error';
  id: number;
}

interface ToastContextValue {
  show: (text: string, tone?: 'success' | 'error') => void;
}

const ToastContext = createContext<ToastContextValue>({ show: () => {} });

export function useToast(): ToastContextValue {
  return useContext(ToastContext);
}

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toast, setToast] = useState<ToastState | null>(null);
  const timer = useRef<number | undefined>(undefined);

  const show = useCallback((text: string, tone: 'success' | 'error' = 'success') => {
    window.clearTimeout(timer.current);
    setToast({ text, tone, id: Date.now() });
    timer.current = window.setTimeout(() => setToast(null), 3200);
  }, []);

  useEffect(() => () => window.clearTimeout(timer.current), []);

  const value = useMemo(() => ({ show }), [show]);

  return (
    <ToastContext.Provider value={value}>
      {children}
      {toast ? (
        <div
          key={toast.id}
          className="pointer-events-none fixed inset-x-0 z-[60] flex justify-center px-4"
          style={{ bottom: 'calc(var(--safe-bottom) + 72px)' }}
        >
          <div
            className={`fade-in max-w-[440px] rounded-xl px-4 py-2.5 text-caption text-white shadow-lg ${
              toast.tone === 'success' ? 'bg-success' : 'bg-danger'
            }`}
            role="status"
          >
            {toast.text}
          </div>
        </div>
      ) : null}
    </ToastContext.Provider>
  );
}

/** Fires success/error haptics together with a toast. */
export function useFeedback(): (text: string, tone?: 'success' | 'error') => void {
  const { show } = useToast();
  return useCallback(
    (text: string, tone: 'success' | 'error' = 'success') => {
      if (tone === 'success') haptic.success();
      else haptic.error();
      show(text, tone);
    },
    [show],
  );
}
