import type { ReactNode } from 'react';
import { useState } from 'react';
import { STATUS_META, type StatusMeta } from '@/lib/catalog';
import { useI18n, useT } from '@/i18n';
import { statusLabel } from '@/i18n/catalog';
import { hasNativeButtons } from '@/telegram/sdk';
import type { OrderStatus } from '@/api/types';

/** Telegram avatar with a graceful fallback to initials (a broken photo used to
 * render as a black circle). */
export function Avatar({
  photoUrl,
  name,
  size = 32,
}: {
  photoUrl?: string;
  name?: string;
  size?: number;
}) {
  const [broken, setBroken] = useState(false);
  const initial = (name ?? 'S').trim().slice(0, 1).toUpperCase();
  const style = { width: size, height: size };

  if (photoUrl && !broken) {
    return (
      <img
        src={photoUrl}
        alt=""
        style={style}
        onError={() => setBroken(true)}
        className="shrink-0 rounded-full object-cover"
      />
    );
  }
  return (
    <span
      style={style}
      className="flex shrink-0 items-center justify-center rounded-full bg-accent-soft font-semibold text-accent"
    >
      {initial}
    </span>
  );
}

/** Material Symbols glyph. */
export function Icon({
  name,
  size = 22,
  filled = false,
  className = '',
}: {
  name: string;
  size?: number;
  filled?: boolean;
  className?: string;
}) {
  return (
    <span
      aria-hidden="true"
      className={`icon ${filled ? 'icon-filled' : ''} ${className}`}
      style={{ fontSize: size }}
    >
      {name}
    </span>
  );
}

/** Fixed Telegram-style header with an optional leading control. */
export function AppHeader({
  title,
  onBack,
  trailing,
  leadingIcon,
}: {
  title: string;
  onBack?: () => void;
  trailing?: ReactNode;
  leadingIcon?: string;
}) {
  const t = useT();
  // Inside Telegram the native BackButton owns "back"; the in-content arrow is
  // the browser/mock affordance (design.md §12.5).
  const showInlineBack = Boolean(onBack) && !hasNativeButtons();
  const showLeadingIcon = Boolean(leadingIcon) && !showInlineBack;
  return (
    <header className="fixed inset-x-0 top-0 z-50 bg-page/90 shadow-[0_1px_8px_rgba(0,0,0,0.04)] backdrop-blur-xl">
      <div
        className="mx-auto flex h-14 w-full max-w-[480px] items-center justify-between px-4"
        style={{ paddingTop: 'var(--safe-top)', height: 'calc(3.5rem + var(--safe-top))' }}
      >
        <div className="flex min-w-0 items-center gap-1">
          {showInlineBack ? (
            <button
              type="button"
              aria-label={t('Назад')}
              onClick={onBack}
              className="-ml-1 flex size-11 items-center justify-center text-accent transition-opacity active:opacity-60"
            >
              <Icon name="arrow_back_ios_new" size={22} />
            </button>
          ) : showLeadingIcon ? (
            <span className="flex size-11 items-center justify-center text-accent">
              <Icon name={leadingIcon ?? 'school'} size={22} />
            </span>
          ) : null}
          <h1 className="truncate text-headline-md text-ink">{title}</h1>
        </div>
        <div className="flex items-center gap-2">{trailing}</div>
      </div>
    </header>
  );
}

/** Page body: keeps the fixed header and the bottom CTA clear. */
export function Screen({ children, header, bottomSpace = false }: { children: ReactNode; header?: ReactNode; bottomSpace?: boolean }) {
  return (
    <>
      {header}
      <main
        className="mx-auto flex w-full max-w-[480px] flex-1 flex-col bg-page px-4"
        style={{ paddingTop: 'calc(3.5rem + var(--safe-top))' }}
      >
        <div className={`flex w-full flex-col ${bottomSpace ? 'pb-32' : 'pb-8'}`}>{children}</div>
      </main>
    </>
  );
}

export function SectionLabel({ children, action }: { children: ReactNode; action?: ReactNode }) {
  return (
    <div className="flex items-center justify-between px-1 pb-1.5 pt-2">
      <span className="text-section-label uppercase text-ink-2">{children}</span>
      {action}
    </div>
  );
}

export function Card({ children, className = '' }: { children: ReactNode; className?: string }) {
  return <div className={`card flex flex-col ${className}`}>{children}</div>;
}

export function Hairline() {
  return <div className="hairline" aria-hidden="true" />;
}

/** Telegram-style tappable row. */
export function Row({
  icon,
  iconTone = 'accent',
  title,
  subtitle,
  value,
  onClick,
  destructive = false,
  trailing,
  disabled = false,
}: {
  icon?: string;
  iconTone?: 'accent' | 'ink-2' | 'danger' | 'success';
  title: string;
  subtitle?: string;
  value?: ReactNode;
  onClick?: () => void;
  destructive?: boolean;
  trailing?: ReactNode;
  disabled?: boolean;
}) {
  const tone =
    destructive || iconTone === 'danger'
      ? 'text-danger'
      : iconTone === 'ink-2'
        ? 'text-ink-2'
        : iconTone === 'success'
          ? 'text-success'
          : 'text-accent';

  const content = (
    <>
      {icon ? (
        <span className={`flex w-9 shrink-0 items-center justify-center ${tone}`}>
          <Icon name={icon} size={22} />
        </span>
      ) : null}
      <div className="flex min-w-0 flex-1 flex-col py-2.5">
        <span className={`truncate text-body-strong ${destructive ? 'text-danger' : 'text-ink'}`}>{title}</span>
        {subtitle ? <span className="truncate text-caption text-ink-2">{subtitle}</span> : null}
      </div>
      {value !== undefined ? <span className="shrink-0 pl-2 text-body-secondary text-ink-2">{value}</span> : null}
      {trailing ?? (onClick ? <Icon name="chevron_right" size={20} className="shrink-0 pl-1 text-outline-variant" /> : null)}
    </>
  );

  if (!onClick) {
    return <div className="flex min-h-[52px] items-center gap-2 px-4">{content}</div>;
  }

  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      className="flex min-h-[52px] w-full items-center gap-2 px-4 text-left transition-colors active:bg-neutral-soft/60 disabled:opacity-50"
    >
      {content}
    </button>
  );
}

export function StatusChip({ status, size = 'md' }: { status: OrderStatus; size?: 'sm' | 'md' }) {
  const { lang } = useI18n();
  const meta: StatusMeta = STATUS_META[status] ?? { label: status, icon: 'schedule', tone: 'neutral' };
  const label = statusLabel(lang, status) || meta.label;
  const tones: Record<StatusMeta['tone'], string> = {
    accent: 'bg-accent-soft text-accent',
    success: 'bg-success-soft text-success',
    warning: 'bg-warning-soft text-warning',
    danger: 'bg-danger-soft text-danger',
    neutral: 'bg-neutral-soft text-neutral',
  };
  const padding = size === 'sm' ? 'px-2 py-0.5 text-[12px]' : 'px-2.5 py-1 text-chip';
  return (
    <span className={`inline-flex shrink-0 items-center gap-1 rounded-full ${padding} ${tones[meta.tone]}`}>
      <Icon name={meta.icon} size={size === 'sm' ? 14 : 15} filled={meta.filled} />
      <span className="whitespace-nowrap">{label}</span>
    </span>
  );
}

/** 4-segment wizard progress bar. */
export function StepProgress({ step, total = 4 }: { step: number; total?: number }) {
  return (
    <div className="grid w-full gap-2 pt-1" style={{ gridTemplateColumns: `repeat(${total}, minmax(0, 1fr))` }}>
      {Array.from({ length: total }, (_, index) => (
        <div
          key={index}
          className={`h-1 rounded-full transition-all duration-300 ${index < step ? 'bg-accent' : 'bg-line'}`}
        />
      ))}
    </div>
  );
}

/** Telegram-style stepper (− value +). */
export function Stepper({
  value,
  onChange,
  min,
  max,
  label,
  display,
  disabled = false,
}: {
  value: number;
  onChange: (next: number) => void;
  min: number;
  max: number;
  label: string;
  display: string;
  disabled?: boolean;
}) {
  const step = (delta: number) => {
    const next = Math.min(max, Math.max(min, value + delta));
    if (next !== value) onChange(next);
  };
  const t = useT();
  // "10 страниц" needs a wider cell than "1" - the narrower the plate, the more
  // room the label gets on a 360px screen (no wrapped "Работ в заказе").
  const wide = display.length > 3;
  return (
    <div className="flex items-center justify-between gap-2 px-4 py-2">
      <span className="min-w-0 flex-1 text-body-lg text-ink">{label}</span>
      <div
        className={`flex shrink-0 items-center rounded-lg bg-neutral-soft/70 p-0.5 ${
          disabled ? 'pointer-events-none opacity-40' : ''
        }`}
      >
        <button
          type="button"
          aria-label={t('Меньше: {label}', { label })}
          onClick={() => step(-1)}
          className="flex size-10 items-center justify-center rounded-lg text-ink transition active:scale-95 active:bg-outline-variant/30"
        >
          <Icon name="remove" size={20} />
        </button>
        <span
          className={`${wide ? 'w-[74px]' : 'w-9'} select-none whitespace-nowrap text-center text-body-secondary-strong tabular-nums text-ink`}
        >
          {display}
        </span>
        <button
          type="button"
          aria-label={t('Больше: {label}', { label })}
          onClick={() => step(1)}
          className="flex size-10 items-center justify-center rounded-lg text-ink transition active:scale-95 active:bg-outline-variant/30"
        >
          <Icon name="add" size={20} />
        </button>
      </div>
    </div>
  );
}

export function Segmented<T extends string>({
  options,
  value,
  onChange,
}: {
  options: readonly { value: T; label: string }[];
  value: T;
  onChange: (next: T) => void;
}) {
  return (
    <div
      className="grid gap-0.5 rounded-xl bg-outline-variant/40 p-0.5"
      style={{ gridTemplateColumns: `repeat(${options.length}, minmax(0, 1fr))` }}
    >
      {options.map((option) => {
        const active = option.value === value;
        return (
          <button
            key={option.value}
            type="button"
            onClick={() => onChange(option.value)}
            className={
              active
                ? 'flex h-9 items-center justify-center rounded-[10px] bg-surface text-body-secondary-strong text-ink shadow-[0_1px_4px_rgba(0,0,0,0.08)]'
                : 'flex h-9 items-center justify-center rounded-[10px] text-body-secondary text-ink-2'
            }
          >
            {option.label}
          </button>
        );
      })}
    </div>
  );
}

export function FilterChip({ active, children, onClick }: { active: boolean; children: ReactNode; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={
        active
          ? 'h-8 shrink-0 rounded-full bg-accent-soft px-3.5 text-caption font-semibold text-accent'
          : 'h-8 shrink-0 rounded-full bg-neutral-soft px-3.5 text-caption text-ink-2'
      }
    >
      {children}
    </button>
  );
}

export function Switch({ checked, onChange, label }: { checked: boolean; onChange: (next: boolean) => void; label: string }) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      onClick={() => onChange(!checked)}
      className={`relative h-7 w-12 shrink-0 rounded-full transition-colors ${checked ? 'bg-accent' : 'bg-outline-variant'}`}
    >
      <span
        className={`absolute top-0.5 size-6 rounded-full bg-white shadow transition-all ${checked ? 'left-[22px]' : 'left-0.5'}`}
      />
    </button>
  );
}

export function Banner({
  tone,
  icon,
  title,
  children,
  action,
}: {
  tone: 'info' | 'success' | 'warning' | 'danger';
  icon: string;
  title?: string;
  children?: ReactNode;
  action?: ReactNode;
}) {
  const tones = {
    info: 'bg-accent-soft text-accent',
    success: 'bg-success-soft text-success',
    warning: 'bg-warning-soft text-warning',
    danger: 'bg-danger-soft text-danger',
  } as const;
  return (
    <div className={`fade-in flex items-start gap-2 rounded-[14px] px-4 py-3 ${tones[tone]}`}>
      <Icon name={icon} size={20} filled />
      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
        {title ? <span className="text-body-secondary-strong">{title}</span> : null}
        {children ? <span className="text-caption opacity-90">{children}</span> : null}
        {action ? <span className="pt-1">{action}</span> : null}
      </div>
    </div>
  );
}

export function Skeleton({ className = '' }: { className?: string }) {
  return <div className={`skeleton ${className}`} />;
}

export function EmptyState({
  icon,
  title,
  text,
  action,
}: {
  icon: string;
  title: string;
  text?: string;
  action?: ReactNode;
}) {
  return (
    <div className="flex flex-col items-center gap-1 px-6 py-10 text-center">
      <Icon name={icon} size={48} className="text-outline-variant" />
      <span className="pt-2 text-headline-md text-ink">{title}</span>
      {text ? <span className="text-body-secondary text-ink-2">{text}</span> : null}
      {action ? <div className="pt-3">{action}</div> : null}
    </div>
  );
}

export function SecondaryButton({
  children,
  onClick,
  full = false,
  disabled = false,
}: {
  children: ReactNode;
  onClick: () => void;
  full?: boolean;
  disabled?: boolean;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      className={`h-11 items-center justify-center rounded-xl bg-accent-soft px-4 text-body-secondary-strong text-accent transition active:scale-[0.98] disabled:opacity-50 ${full ? 'flex w-full' : 'inline-flex'}`}
    >
      {children}
    </button>
  );
}

/** Inline field error (server `fields` map to the same inputs). */
export function FieldError({ children }: { children?: string }) {
  if (!children) return null;
  return <span className="px-1 pt-1 text-caption text-danger">{children}</span>;
}
