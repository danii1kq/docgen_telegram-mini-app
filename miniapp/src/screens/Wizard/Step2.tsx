import { useRef, useState } from 'react';
import type { FileMeta } from '@/api/types';
import { useT } from '@/i18n';
import { FILE_ACCEPT, FILE_MAX_COUNT, FILE_MAX_SIZE } from '@/lib/catalog';
import { formatSize } from '@/lib/format';
import { haptic } from '@/telegram/sdk';
import { Card, FieldError, Hairline, Icon, SectionLabel } from '@/components/ui';
import type { StepProps } from './Step1';

/** S04 · Wizard step 2 - topics, requirements, client materials (metadata). */
export function Step2({ draft, update, errors }: StepProps) {
  const t = useT();
  const fileInput = useRef<HTMLInputElement>(null);
  const [fileError, setFileError] = useState<string | undefined>(undefined);

  const addFiles = (files: FileList | null) => {
    if (!files || files.length === 0) return;
    const next: FileMeta[] = [...draft.attachments];
    let problem: string | undefined;
    for (const file of Array.from(files)) {
      if (next.length >= FILE_MAX_COUNT) {
        problem = t('Не больше 3 файлов');
        break;
      }
      if (file.size > FILE_MAX_SIZE) {
        problem = t('Файл больше 5 МБ: {name}', { name: file.name });
        continue;
      }
      const extension = file.name.split('.').pop()?.toLowerCase() ?? '';
      if (!FILE_ACCEPT.includes(`.${extension}`)) {
        problem = t('Этот формат не поддерживается: {name}', { name: file.name });
        continue;
      }
      next.push({ name: file.name, size: file.size, type: file.type || 'application/octet-stream' });
    }
    if (problem) {
      haptic.error();
      setFileError(problem);
    } else {
      setFileError(undefined);
      haptic.tap();
    }
    update({ attachments: next.slice(0, FILE_MAX_COUNT) });
  };

  return (
    <>
      <div className="flex flex-col" data-field="topics">
        <SectionLabel>{t('Темы работ')}</SectionLabel>
        <Card className="p-4">
          <textarea
            value={draft.topics}
            maxLength={5000}
            rows={4}
            onChange={(event) => update({ topics: event.target.value })}
            placeholder={t('Укажите тему работы')}
            className="w-full resize-none border-0 bg-transparent p-0 text-body-lg text-ink outline-none placeholder:text-hint"
          />
        </Card>
        <div className="flex items-start justify-between gap-2 px-1 pt-1.5">
          <span className="flex-1 text-caption text-ink-2">
            {t('Если работ несколько, перечислите темы по порядку, по одной в строке.')}
          </span>
          <span className="whitespace-nowrap text-caption text-ink-2">{draft.topics.length} / 5000</span>
        </div>
        <FieldError>{errors.topics}</FieldError>
      </div>

      <div className="flex flex-col">
        <SectionLabel>{t('Требования (необязательно)')}</SectionLabel>
        <Card className="p-4">
          <textarea
            value={draft.notes}
            maxLength={5000}
            rows={3}
            onChange={(event) => update({ notes: event.target.value })}
            placeholder={t('Пожелания к оформлению, структуре, источникам')}
            className="w-full resize-none border-0 bg-transparent p-0 text-body-lg text-ink outline-none placeholder:text-hint"
          />
        </Card>
      </div>

      <div className="flex flex-col">
        <SectionLabel>{t('Материалы (необязательно)')}</SectionLabel>
        <Card>
          {draft.attachments.map((file, index) => (
            <div key={`${file.name}-${index}`}>
              <div className="flex h-16 items-center gap-3 px-4">
                <span className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-accent-soft text-accent">
                  <Icon name="description" size={20} />
                </span>
                <span className="flex min-w-0 flex-1 flex-col">
                  <span className="truncate text-body-strong text-ink">{file.name}</span>
                  <span className="text-caption text-ink-2">
                    {formatSize(file.size)} · {file.name.split('.').pop()?.toUpperCase()}
                  </span>
                </span>
                <button
                  type="button"
                  aria-label={t('Удалить файл {name}', { name: file.name })}
                  onClick={() => {
                    haptic.tap();
                    update({ attachments: draft.attachments.filter((_, position) => position !== index) });
                  }}
                  className="flex size-11 items-center justify-center text-danger transition active:opacity-60"
                >
                  <Icon name="close" size={20} />
                </button>
              </div>
              <Hairline />
            </div>
          ))}

          {draft.attachments.length < FILE_MAX_COUNT ? (
            <label className="flex h-[52px] cursor-pointer items-center gap-3 px-4 transition active:bg-accent-soft/40">
              <input
                ref={fileInput}
                type="file"
                className="hidden"
                accept={FILE_ACCEPT}
                multiple
                onChange={(event) => {
                  addFiles(event.target.files);
                  if (fileInput.current) fileInput.current.value = '';
                }}
              />
              <span className="flex size-9 shrink-0 items-center justify-center text-accent">
                <Icon name="add" size={24} />
              </span>
              <span className="text-body-strong text-accent">{t('Прикрепить файл')}</span>
            </label>
          ) : null}
        </Card>
        <span className="px-1 pt-1.5 text-caption text-ink-2">
          {t('PDF, DOCX, TXT, PNG, JPG · до 5 МБ · не больше 3 файлов')}
        </span>
        <FieldError>{fileError}</FieldError>
      </div>
    </>
  );
}
