import { useState } from 'react';
import { useT } from '@/i18n';
import { haptic } from '@/telegram/sdk';
import { loadPresets, savePreset, type AcademicPreset } from '@/lib/draft';
import { Card, Hairline, Icon, SectionLabel } from '@/components/ui';
import { PickerOrCustomRow, TextRow } from '@/components/fields';
import { useFeedback } from '@/components/Toast';
import { departmentOptions, INSTITUTE_VALUES } from './constants';
import type { StepProps } from './Step1';

/** S05 · Wizard step 3 - the title-page data (saved profiles + the form). */
export function Step3({ draft, update, errors }: StepProps) {
  const t = useT();
  const feedback = useFeedback();
  const [presets, setPresets] = useState<AcademicPreset[]>(() => loadPresets());

  const applyPreset = (preset: AcademicPreset) => {
    haptic.select();
    update({
      institute: preset.institute,
      department: preset.department,
      studentName: preset.studentName,
      disciplineCode: preset.disciplineCode,
      teacherName: preset.teacherName,
    });
  };

  const rememberCurrent = () => {
    const name = `${draft.institute || t('Профиль')} · ${draft.department || t('без кафедры')}`;
    const next = savePreset({
      name,
      institute: draft.institute,
      department: draft.department,
      studentName: draft.studentName,
      disciplineCode: draft.disciplineCode,
      teacherName: draft.teacherName,
    });
    setPresets(next);
    haptic.success();
    feedback(t('Шаблон сохранён'), 'success');
  };

  // A profile worth keeping needs the student, the programme code and a place.
  const canRemember = Boolean(
    draft.studentName.trim() && draft.disciplineCode.trim() && (draft.institute.trim() || draft.department.trim()),
  );

  return (
    <>
      {presets.length > 0 ? (
        <div className="flex flex-col">
          <SectionLabel>{t('Шаблоны данных')}</SectionLabel>
          <Card>
            {presets.map((preset, index) => (
              <div key={preset.name}>
                {index > 0 ? <Hairline /> : null}
                <button
                  type="button"
                  onClick={() => applyPreset(preset)}
                  className="flex w-full items-center gap-3 px-4 py-3 text-left transition active:bg-accent-soft/40"
                >
                  <span className="flex size-9 shrink-0 items-center justify-center rounded-full bg-accent-soft text-accent">
                    <Icon name="school" size={20} />
                  </span>
                  <span className="flex min-w-0 flex-1 flex-col">
                    <span className="truncate text-body-strong text-ink">{preset.name}</span>
                    <span className="truncate text-body-secondary text-ink-2">
                      {preset.studentName} · {preset.disciplineCode} · {preset.teacherName}
                    </span>
                  </span>
                  <Icon name="chevron_right" size={20} className="text-outline-variant" />
                </button>
              </div>
            ))}
          </Card>
          <span className="px-1 pt-1.5 text-caption text-ink-2">{t('Выберите сохранённый профиль или заполните поля ниже.')}</span>
        </div>
      ) : null}

      <div className="flex flex-col">
        <SectionLabel>{t('Учебные данные')}</SectionLabel>
        <Card>
          <PickerOrCustomRow
            label={t('Институт *')}
            value={draft.institute}
            placeholder={t('Выберите институт')}
            options={INSTITUTE_VALUES}
            customOptionLabel={t('Другое (вписать свой вариант)')}
            customLabel={t('Свой вариант института')}
            customPlaceholder={t('Например: Институт автоматики и информационных технологий')}
            onSelect={(institute) => update({ institute })}
            error={errors.institute}
            field="institute"
          />
          <Hairline />
          <PickerOrCustomRow
            label={t('Кафедра *')}
            value={draft.department}
            placeholder={t('Выберите кафедру')}
            options={departmentOptions(draft.institute)}
            customOptionLabel={t('Другое (вписать свой вариант)')}
            customLabel={t('Свой вариант кафедры')}
            customPlaceholder={t('Например: Программной инженерии')}
            onSelect={(department) => update({ department })}
            error={errors.department}
            field="department"
          />
          <Hairline />
          <TextRow
            label={t('ФИО студента *')}
            value={draft.studentName}
            maxLength={200}
            onChange={(studentName) => update({ studentName })}
            error={errors.studentName}
            field="studentName"
          />
          <Hairline />
          <TextRow
            label={t('Шифр ОП *')}
            value={draft.disciplineCode}
            placeholder="6B07329"
            maxLength={200}
            onChange={(disciplineCode) => update({ disciplineCode })}
            error={errors.disciplineCode}
            field="disciplineCode"
          />
          <Hairline />
          <TextRow
            label={t('ФИО преподавателя *')}
            value={draft.teacherName}
            maxLength={200}
            onChange={(teacherName) => update({ teacherName })}
            error={errors.teacherName}
            field="teacherName"
          />
        </Card>
      </div>

      <div className="flex flex-col gap-1.5">
        <button
          type="button"
          onClick={rememberCurrent}
          disabled={!canRemember}
          className="flex items-center gap-3 rounded-[14px] bg-surface px-4 py-3 text-left transition active:opacity-80 disabled:opacity-40"
        >
          <span className="flex size-9 shrink-0 items-center justify-center rounded-full bg-accent-soft text-accent">
            <Icon name="add" size={20} />
          </span>
          <span className="text-body-secondary-strong text-accent">{t('Сохранить текущие данные как шаблон')}</span>
        </button>
        {!canRemember ? (
          <span className="px-1 text-caption text-ink-2">
            {t('Заполните ФИО студента, шифр ОП и институт или кафедру - и шаблон можно будет сохранить.')}
          </span>
        ) : null}
      </div>
    </>
  );
}
