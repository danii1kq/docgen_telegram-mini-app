import type { Product, Quote } from '@/api/types';
import { useI18n } from '@/i18n';
import { productName, quoteLineLabel, quoteLineValue, unitForms, unitShort } from '@/i18n/catalog';
import { productIcon } from '@/lib/catalog';
import type { Draft } from '@/lib/draft';
import { money, unitsLabel } from '@/lib/format';
import { haptic } from '@/telegram/sdk';
import { PickerRow } from '@/components/fields';
import { Card, FieldError, Hairline, Icon, Segmented, SectionLabel, Stepper } from '@/components/ui';
import { LANGUAGES, WORK_KINDS } from './constants';

export interface StepProps {
  draft: Draft;
  update: (patch: Partial<Draft>) => void;
  errors: Record<string, string>;
}

/** S03 · Wizard step 1 - work type, language, volume, price. */
export function Step1({
  draft,
  update,
  products,
  quote,
  errors,
}: StepProps & { products: Product[]; quote?: Quote }) {
  const { t, lang } = useI18n();
  const selected = products.find((item) => item.code === draft.product) ?? products[0];
  const languageOptions = LANGUAGES.map((option) => ({ value: option.value, label: t(option.label) }));

  return (
    <>
      <div className="flex flex-col">
        <SectionLabel>{t('Вид работы')}</SectionLabel>
        <Card>
          {products.map((item, index) => {
            const active = item.code === draft.product;
            return (
              <div key={item.code}>
                {index > 0 ? <Hairline /> : null}
                <button
                  type="button"
                  data-field="product"
                  onClick={() => {
                    haptic.select();
                    const units = item.fixed
                      ? item.min
                      : Math.min(item.max, Math.max(item.min, draft.units || 10));
                    update({ product: item.code, units, works: draft.works });
                  }}
                  className={`flex h-16 w-full items-center justify-between px-4 text-left transition-colors ${
                    active ? 'bg-accent-soft/70' : 'active:bg-neutral-soft/60'
                  }`}
                >
                  <div className="flex min-w-0 items-center gap-3">
                    <span
                      className={`flex size-5 shrink-0 items-center justify-center rounded-full border-2 ${
                        active ? 'border-accent bg-accent' : 'border-outline-variant'
                      }`}
                    >
                      <span className={`size-2 rounded-full ${active ? 'bg-surface' : 'bg-transparent'}`} />
                    </span>
                    <span className="flex min-w-0 flex-col">
                      <span className="truncate text-body-strong text-ink">{productName(lang, item.code, item.name)}</span>
                      <span className="truncate text-caption text-ink-2">
                        {item.fixed
                          ? `${money(item.price ?? 0)} · ${t('фиксированная цена')}`
                          : `${money(item.price ?? 0)} / ${unitShort(lang, item.unitOne)} · ${item.min}-${item.max}`}
                      </span>
                    </span>
                  </div>
                  <Icon name={productIcon(item.code)} size={20} className={active ? 'text-accent' : 'text-ink-2'} />
                </button>
              </div>
            );
          })}
        </Card>
      </div>

      {draft.product === 'other' ? (
        <div className="flex flex-col">
          <SectionLabel>{t('Вид работы «СРС / другое»')}</SectionLabel>
          <Card>
            <PickerRow
              label={t('Тип работы')}
              value={draft.workKind}
              placeholder={t('Выберите тип')}
              options={WORK_KINDS}
              onSelect={(workKind) => update({ workKind })}
              error={errors.workKind}
              field="workKind"
            />
          </Card>
        </div>
      ) : null}

      <div className="flex flex-col">
        <SectionLabel>{t('Язык работы')}</SectionLabel>
        <Segmented
          options={languageOptions}
          value={draft.language}
          onChange={(value) => update({ language: value })}
        />
        <span className="px-1 pt-1.5 text-caption text-ink-2">{t('Документ будет оформлен на выбранном языке.')}</span>
      </div>

      <div className="flex flex-col" data-field="units">
        <SectionLabel>{t('Объём')}</SectionLabel>
        <Card>
          <Stepper
            label={t('Количество')}
            value={draft.units}
            min={selected?.min ?? 4}
            max={selected?.max ?? 40}
            display={unitsLabel(draft.units, selected)}
            disabled={selected?.fixed}
            onChange={(units) => update({ units })}
          />
        </Card>
        <span className="px-1 pt-1.5 text-caption text-ink-2">
          {selected?.fixed
            ? t('Фиксированный объём - одна работа.')
            : t('Допустимо {min}-{max} {unit} · {price}/{short}', {
                min: selected?.min ?? '',
                max: selected?.max ?? '',
                unit: unitForms(lang, selected?.unitOne)[2],
                price: money(selected?.price ?? 0),
                short: unitShort(lang, selected?.unitOne),
              })}
        </span>
        <FieldError>{errors.units}</FieldError>
      </div>

      <div className="flex flex-col">
        <SectionLabel>{t('Работ в заказе')}</SectionLabel>
        <Card>
          <Stepper
            label={t('Экземпляров')}
            value={draft.works}
            min={1}
            max={20}
            display={String(draft.works)}
            onChange={(works) => update({ works })}
          />
        </Card>
        <span className={`px-1 pt-1.5 text-caption ${draft.works >= 5 ? 'text-success' : 'text-ink-2'}`}>
          {draft.works >= 5
            ? t('Применена скидка за объём: -10 % от 5 работ, -15 % от 10, -20 % от 20')
            : t('Скидка за объём начнётся от 5 работ')}
        </span>
      </div>

      <div className="flex flex-col">
        <SectionLabel>{t('Стоимость')}</SectionLabel>
        <Card className="px-4 py-3">
          {quote ? (
            <div className="flex flex-col gap-1.5">
              {quote.lines.map((line, index) => (
                <div key={index} className="flex items-start justify-between gap-2">
                  <span className="text-body-secondary text-ink-2">{quoteLineLabel(lang, line.label)}</span>
                  <span className={`whitespace-nowrap text-body-secondary ${line.discount ? 'text-success' : 'text-ink'}`}>
                    {quoteLineValue(lang, line)}
                  </span>
                </div>
              ))}
              <div className="hairline-full my-1" />
              <div className="flex items-center justify-between">
                <span className="text-headline-md text-ink">{t('Итого')}</span>
                <span className="text-headline-md tabular-nums text-accent">{money(quote.total)}</span>
              </div>
            </div>
          ) : (
            <div className="py-2 text-body-secondary text-ink-2">{t('Считаем стоимость…')}</div>
          )}
        </Card>
        <span className="px-1 pt-1.5 text-caption text-ink-2">{t('Цена фиксируется при отправке заказа.')}</span>
      </div>
    </>
  );
}
