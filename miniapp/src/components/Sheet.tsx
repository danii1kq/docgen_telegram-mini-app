import { useEffect, type ReactNode } from 'react';
import { useT } from '@/i18n';
import { haptic } from '@/telegram/sdk';
import { Hairline, Icon } from './ui';

/** Bottom sheet (option pickers). Telegram-native look, one open at a time. */
export function Sheet({
  open,
  title,
  onClose,
  children,
}: {
  open: boolean;
  title?: string;
  onClose: () => void;
  children: ReactNode;
}) {
  const t = useT();
  useEffect(() => {
    if (!open) return;
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [open, onClose]);

  if (!open) return null;

  return (
    <div className="fixed inset-0 z-[70] flex items-end justify-center">
      <button
        type="button"
        aria-label={t('Закрыть')}
        className="absolute inset-0 bg-black/40"
        onClick={() => {
          haptic.tap();
          onClose();
        }}
      />
      <div
        role="dialog"
        aria-modal="true"
        className="fade-in relative w-full max-w-[480px] rounded-t-2xl bg-surface pb-[var(--safe-bottom)]"
      >
        <div className="flex h-14 items-center justify-between px-4">
          <span className="text-headline-md text-ink">{title ?? ''}</span>
          <button
            type="button"
            aria-label={t('Закрыть')}
            onClick={onClose}
            className="flex size-11 items-center justify-center text-ink-2"
          >
            <Icon name="close" size={22} />
          </button>
        </div>
        <Hairline />
        <div className="max-h-[60vh] overflow-y-auto py-1">{children}</div>
      </div>
    </div>
  );
}

export function SheetOption({
  title,
  subtitle,
  selected,
  onClick,
}: {
  title: string;
  subtitle?: string;
  selected: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="flex w-full items-center gap-3 px-4 py-3 text-left transition active:bg-neutral-soft/60"
    >
      <span className="flex min-w-0 flex-1 flex-col">
        <span className="truncate text-body-strong text-ink">{title}</span>
        {subtitle ? <span className="truncate text-caption text-ink-2">{subtitle}</span> : null}
      </span>
      {selected ? <Icon name="check" size={22} className="text-accent" /> : null}
    </button>
  );
}
