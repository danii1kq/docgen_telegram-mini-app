import { Card, Icon } from '@/components/ui';
import { useT } from '@/i18n';

/**
 * «Отправитель платежа» field (S07). The CTA lives on the MainButton, so this
 * component only owns the Kaspi sender name and its hint/error copy.
 *
 * Rule (mirrors validate.SenderName): at least two words, the last one is a
 * single letter with a dot - «Ануар А.».
 */
export function senderNameError(value: string): string | undefined {
  const trimmed = value.trim();
  if (!trimmed) return undefined; // empty is a "not yet", not an error
  const words = trimmed.split(/\s+/);
  if (words.length < 2) {
    return 'Укажите имя и фамилию, как в Kaspi: «Ануар А.»';
  }
  const last = words[words.length - 1].replace(/\./g, '');
  if (last.length > 2) {
    return 'Фамилия в Kaspi сокращена одной буквой: «Ануар А.»';
  }
  return undefined;
}

export function senderNameValid(value: string): boolean {
  const trimmed = value.trim();
  return trimmed.length > 0 && senderNameError(trimmed) === undefined;
}

export function SenderField({
  value,
  onChange,
  error,
}: {
  value: string;
  onChange: (next: string) => void;
  error?: string;
}) {
  const t = useT();
  const hint = error
    ? t(error)
    : t('Как в Kaspi - имя и фамилия одной буквой: по этому имени оператор найдёт платёж.');
  const invalid = Boolean(error);
  return (
    <div className="flex flex-col gap-1 px-1">
      <label htmlFor="sender-name" className="text-caption text-ink-2">
        {t('Отправитель платежа *')}
      </label>
      <Card className="flex-row items-center gap-3 px-4 py-3">
        <Icon name="person" size={22} className="text-accent" />
        <input
          id="sender-name"
          value={value}
          autoComplete="name"
          placeholder="Ануар А."
          onChange={(event) => onChange(event.target.value)}
          className="min-w-0 flex-1 border-0 bg-transparent p-0 text-body-lg text-ink outline-none placeholder:text-hint"
        />
      </Card>
      <span className={`text-caption ${invalid ? 'text-danger' : 'text-ink-2'}`}>{hint}</span>
    </div>
  );
}
