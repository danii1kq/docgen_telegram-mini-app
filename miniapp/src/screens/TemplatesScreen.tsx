import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useI18n } from '@/i18n';
import { BackButton, MainButton } from '@/telegram/hooks';
import { haptic, showConfirm } from '@/telegram/sdk';
import { applyPresetToDraft, loadPresets, removePreset, type AcademicPreset } from '@/lib/draft';
import { useOrderStart } from '@/lib/hooks';
import { AppHeader, Card, EmptyState, Hairline, Icon, Screen, SectionLabel } from '@/components/ui';
import { useFeedback } from '@/components/Toast';

/**
 * «Мои шаблоны»: the saved title-page sets live on the device (they are not
 * order data), so this screen lists them, applies one to the draft or deletes
 * it. Saving happens on wizard step 3.
 */
export function TemplatesScreen() {
  const navigate = useNavigate();
  const { t } = useI18n();
  const feedback = useFeedback();
  const startOrder = useOrderStart();
  const [presets, setPresets] = useState<AcademicPreset[]>(() => loadPresets());

  const apply = (preset: AcademicPreset) => {
    applyPresetToDraft(preset, 3);
    haptic.select();
    feedback(t('Шаблон применён к черновику'), 'success');
    navigate('/new');
  };

  const remove = async (preset: AcademicPreset) => {
    const confirmed = await showConfirm(t('Удалить шаблон «{name}»?', { name: preset.name }));
    if (!confirmed) return;
    setPresets(removePreset(preset.name));
    haptic.success();
    feedback(t('Шаблон удалён'), 'success');
  };

  return (
    <Screen header={<AppHeader title={t('Мои шаблоны')} onBack={() => navigate(-1)} />} bottomSpace>
      <div className="flex flex-col gap-3 pt-2">
        {presets.length === 0 ? (
          <Card>
            <EmptyState icon="bookmark" title={t('Пока нет шаблонов')} text={t('Заполните данные титульного листа в шаге 3 и сохраните их как шаблон.')} />
          </Card>
        ) : (
          <>
            <SectionLabel>{t('Сохранённые шаблоны')}</SectionLabel>
            <Card>
              {presets.map((preset, index) => (
                <div key={preset.name}>
                  {index > 0 ? <Hairline /> : null}
                  <div className="flex items-center gap-1">
                    <button
                      type="button"
                      onClick={() => apply(preset)}
                      className="flex min-w-0 flex-1 items-center gap-3 px-4 py-3 text-left transition active:bg-neutral-soft/60"
                    >
                      <span className="flex size-9 shrink-0 items-center justify-center rounded-full bg-accent-soft text-accent">
                        <Icon name="school" size={20} />
                      </span>
                      <span className="flex min-w-0 flex-1 flex-col">
                        <span className="truncate text-body-strong text-ink">{preset.name}</span>
                        <span className="truncate text-body-secondary text-ink-2">
                          {preset.studentName} · {preset.disciplineCode} · {preset.teacherName}
                        </span>
                        {preset.department ? (
                          <span className="truncate text-caption text-ink-2">{preset.department}</span>
                        ) : null}
                      </span>
                    </button>
                    <button
                      type="button"
                      aria-label={t('Удалить')}
                      onClick={() => void remove(preset)}
                      className="mr-2 flex size-10 shrink-0 items-center justify-center text-danger transition active:opacity-60"
                    >
                      <Icon name="delete" size={20} />
                    </button>
                  </div>
                </div>
              ))}
            </Card>
            <span className="px-1 text-caption text-ink-2">{t('Нажмите шаблон, чтобы подставить данные в оформление.')}</span>
          </>
        )}
      </div>

      <MainButton text={t('Заказать')} onClick={() => void startOrder()} />
      <BackButton onClick={() => navigate(-1)} />
    </Screen>
  );
}
