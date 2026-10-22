import { useState } from 'react';
import { haptic } from '@/telegram/sdk';
import { Icon } from './ui';
import { Sheet, SheetOption } from './Sheet';

/** A labelled row that opens a bottom sheet with one-tap options. */
export function PickerRow({
  label,
  value,
  placeholder,
  options,
  onSelect,
  error,
  field,
}: {
  label: string;
  value: string;
  placeholder: string;
  options: string[];
  onSelect: (value: string) => void;
  error?: string;
  field?: string;
}) {
  const [open, setOpen] = useState(false);

  return (
    <div className="flex flex-col" data-field={field}>
      <button
        type="button"
        onClick={() => {
          haptic.tap();
          setOpen(true);
        }}
        className="flex w-full items-center justify-between gap-2 px-4 py-3 text-left transition active:bg-neutral-soft/50"
      >
        <span className="flex min-w-0 flex-col">
          <span className="text-caption text-ink-2">{label}</span>
          <span className={`truncate text-body-lg ${value ? 'text-ink' : 'text-hint'}`}>{value || placeholder}</span>
        </span>
        <Icon name="chevron_right" size={20} className="shrink-0 text-ink-2" />
      </button>
      {error ? <span className="px-4 pb-1 text-caption text-danger">{error}</span> : null}

      <Sheet open={open} title={label} onClose={() => setOpen(false)}>
        {options.map((option) => (
          <SheetOption
            key={option}
            title={option}
            selected={option === value}
            onClick={() => {
              haptic.select();
              onSelect(option);
              setOpen(false);
            }}
          />
        ))}
      </Sheet>
    </div>
  );
}

/** The picker option that switches the row to a free-text input. */
const CUSTOM = '\u0000custom';

/**
 * Picker with a «Другое» escape hatch: the fixed options plus a final entry
 * that reveals an inline text input ("вписать свой вариант"). The chosen value
 * is whatever the caller stores - the option text or the typed text.
 */
export function PickerOrCustomRow({
  label,
  value,
  placeholder,
  options,
  customOptionLabel,
  customLabel,
  customPlaceholder,
  onSelect,
  error,
  field,
}: {
  label: string;
  value: string;
  placeholder: string;
  options: string[];
  customOptionLabel: string;
  customLabel: string;
  customPlaceholder: string;
  onSelect: (value: string) => void;
  error?: string;
  field?: string;
}) {
  const [open, setOpen] = useState(false);
  // A value outside the list means a custom one was typed (also after a reload).
  const [custom, setCustom] = useState(() => value !== '' && !options.includes(value));

  const pick = (option: string) => {
    haptic.select();
    setOpen(false);
    if (option === CUSTOM) {
      setCustom(true);
      onSelect('');
      return;
    }
    setCustom(false);
    onSelect(option);
  };

  return (
    <div className="flex flex-col" data-field={field}>
      <button
        type="button"
        onClick={() => {
          haptic.tap();
          setOpen(true);
        }}
        className="flex w-full items-center justify-between gap-2 px-4 py-3 text-left transition active:bg-neutral-soft/50"
      >
        <span className="flex min-w-0 flex-col">
          <span className="text-caption text-ink-2">{label}</span>
          <span className={`truncate text-body-lg ${value ? 'text-ink' : 'text-hint'}`}>
            {value || placeholder}
          </span>
        </span>
        <Icon name="chevron_right" size={20} className="shrink-0 text-ink-2" />
      </button>

      {custom ? (
        <div className="flex flex-col px-4 pb-3">
          <label className="flex flex-col gap-0.5">
            <span className="text-caption text-ink-2">{customLabel}</span>
            <input
              type="text"
              value={value}
              placeholder={customPlaceholder}
              maxLength={200}
              spellCheck={false}
              onChange={(event) => onSelect(event.target.value)}
              className="w-full border-0 bg-transparent p-0 text-body-lg text-ink outline-none placeholder:text-hint"
            />
          </label>
        </div>
      ) : null}

      {error ? <span className="px-4 pb-1 text-caption text-danger">{error}</span> : null}

      <Sheet open={open} title={label} onClose={() => setOpen(false)}>
        {options.map((option) => (
          <SheetOption key={option} title={option} selected={!custom && option === value} onClick={() => pick(option)} />
        ))}
        <SheetOption title={customOptionLabel} selected={custom} onClick={() => pick(CUSTOM)} />
      </Sheet>
    </div>
  );
}

/** A labelled text row (Telegram-style inline input). */export function TextRow({
  label,
  value,
  placeholder,
  onChange,
  onBlur,
  error,
  field,
  hint,
  type = 'text',
  inputMode,
  maxLength,
  minLength,
  uppercase = false,
}: {
  label: string;
  value: string;
  placeholder?: string;
  onChange: (value: string) => void;
  onBlur?: () => void;
  error?: string;
  field?: string;
  hint?: string;
  type?: string;
  inputMode?: 'text' | 'numeric' | 'tel';
  maxLength?: number;
  minLength?: number;
  uppercase?: boolean;
}) {
  return (
    <div className="flex flex-col px-4 py-2.5" data-field={field}>
      <label className="flex flex-col gap-0.5">
        <span className="text-caption text-ink-2">{label}</span>
        <input
          type={type}
          inputMode={inputMode}
          value={value}
          placeholder={placeholder}
          maxLength={maxLength}
          minLength={minLength}
          spellCheck={false}
          onBlur={onBlur}
          onChange={(event) => onChange(uppercase ? event.target.value.toUpperCase() : event.target.value)}
          className="w-full border-0 bg-transparent p-0 text-body-lg text-ink outline-none placeholder:text-hint"
        />
      </label>
      {error ? <span className="pt-1 text-caption text-danger">{error}</span> : null}
      {hint && !error ? <span className="pt-1 text-caption text-ink-2">{hint}</span> : null}
    </div>
  );
}
