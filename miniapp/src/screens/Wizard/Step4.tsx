import { useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { api, ApiError } from '@/api/client';
import type { Quote } from '@/api/types';
import { useI18n } from '@/i18n';
import { productName, quoteLineLabel, quoteLineValue } from '@/i18n/catalog';
import { telegramUser } from '@/telegram/sdk';
import { money, worksLabel } from '@/lib/format';
import { PRODUCTS_FALLBACK } from '@/lib/catalog';
import { useAsync } from '@/lib/hooks';
import { FieldError, Hairline, Icon, SectionLabel, Card } from '@/components/ui';
import { TextRow } from '@/components/fields';
import type { StepProps } from './Step1';

/** S06 · Wizard step 4 - contact, promo, frozen price, consent. */
export function Step4({
  draft,
  update,
  quote,
  errors,
  onGoToStep,
  promo,
  onPromoChange,
}: StepProps & {
  quote?: Quote;
  onGoToStep: (step: number) => void;
  promo: { code: string; discountPct: number } | undefined;
  onPromoChange: (promo: { code: string; discountPct: number } | undefined) => void;
}) {
  const navigate = useNavigate();
  const { t, lang } = useI18n();
  const user = telegramUser();
  const [promoBusy, setPromoBusy] = useState(false);
  const [promoError, setPromoError] = useState<string | undefined>(undefined);

  const myOrders = useAsync(() => api.myOrders(), []);
  const product = PRODUCTS_FALLBACK.find((item) => item.code === draft.product) ?? PRODUCTS_FALLBACK[0];

  const contactHint = user?.username
    ? t('Контакт в Telegram: @{username}', { username: user.username })
    : t('Уведомления придут в этот чат');

  const subtotal = quote?.subtotal ?? 0;
  const isFirstOrder = useMemo(() => {
    const orders = myOrders.data?.orders ?? [];
    return orders.length === 0;
  }, [myOrders.data]);

  const applied = quote?.promoCode ? { code: quote.promoCode, discountPct: quote.promoPct } : promo;

  const checkPromo = async () => {
    const code = draft.promoCode.trim().toUpperCase();
    if (!code) {
      setPromoError(t('Введите промокод'));
      return;
    }
    setPromoBusy(true);
    setPromoError(undefined);
    try {
      const result = await api.promoCheck({
        code,
        product: draft.product,
        units: draft.units,
        works: draft.works,
        isFirstOrder,
        subtotal,
      });
      if (result.ok && result.promo) {
        onPromoChange({ code: result.promo.code, discountPct: result.promo.discountPct });
      } else {
        onPromoChange(undefined);
        setPromoError(result.reason ?? t('Промокод не найден'));
      }
    } catch (cause) {
      onPromoChange(undefined);
      setPromoError(cause instanceof ApiError ? cause.message : t('Не удалось проверить промокод'));
    } finally {
      setPromoBusy(false);
    }
  };

  return (
    <>
      <div className="flex flex-col">
        <SectionLabel>{t('Контактные данные')}</SectionLabel>
        <Card>
          <TextRow
            label={t('Имя для связи')}
            value={draft.contactName}
            placeholder={user ? `${user.first_name} ${user.last_name ?? ''}`.trim() : t('Имя Фамилия')}
            maxLength={120}
            onChange={(contactName) => update({ contactName })}
            error={errors.contactName}
            field="contactName"
          />
          <Hairline />
          <div className="flex items-center gap-3 px-4 py-3">
            <Icon name="send" size={22} className="text-accent" />
            <div className="flex min-w-0 flex-col">
              <span className="text-caption text-ink-2">{t('Контакт в Telegram')}</span>
              <span className="truncate text-body-lg text-accent">
                {user?.username ? `@${user.username}` : t('этот чат')}
              </span>
              <span className="text-caption text-ink-2">{contactHint}</span>
            </div>
          </div>
        </Card>
      </div>

      <div className="flex flex-col">
        <SectionLabel>{t('Промокод')}</SectionLabel>
        <Card className="gap-3 p-4">
          <div className="flex items-center gap-2 rounded-[10px] bg-page px-3 py-1.5">
            <Icon name="sell" size={20} className="text-ink-2" />
            <input
              value={draft.promoCode}
              onChange={(event) => {
                update({ promoCode: event.target.value.toUpperCase() });
                setPromoError(undefined);
              }}
              placeholder={t('Введите промокод')}
              spellCheck={false}
              className="min-w-0 flex-1 bg-transparent font-mono text-[15px] uppercase text-ink outline-none placeholder:font-sans placeholder:normal-case placeholder:text-hint"
            />
            <button
              type="button"
              onClick={checkPromo}
              disabled={promoBusy}
              className="px-2 py-1 text-caption text-accent transition active:opacity-60 disabled:opacity-50"
            >
              {promoBusy ? t('Проверяем…') : t('Применить')}
            </button>
          </div>

          {applied ? (
            <div className="fade-in flex items-center justify-between gap-2 rounded-lg bg-success-soft px-3 py-2">
              <span className="flex min-w-0 items-center gap-2">
                <Icon name="check_circle" size={18} className="text-success" />
                <span className="truncate text-chip text-success">
                  {t('Промокод {code} применён · −{pct} %', { code: applied.code, pct: applied.discountPct })}
                </span>
              </span>
              <button
                type="button"
                onClick={() => {
                  onPromoChange(undefined);
                  update({ promoCode: '' });
                  setPromoError(undefined);
                }}
                className="shrink-0 text-caption text-ink-2 transition active:text-danger"
              >
                {t('Убрать')}
              </button>
            </div>
          ) : null}
          <FieldError>{promoError}</FieldError>
        </Card>
      </div>

      <div className="flex flex-col">
        <SectionLabel
          action={
            <button
              type="button"
              onClick={() => onGoToStep(1)}
              className="text-caption text-accent transition active:opacity-60"
            >
              {t('Изменить')}
            </button>
          }
        >
          {t('Итоговая стоимость')}
        </SectionLabel>
        <Card className="px-4 py-3">
          {quote ? (
            <div className="flex flex-col gap-1.5">
              {quote.lines.map((line, index) => (
                <div key={index} className="flex items-start justify-between gap-2">
                  <span className={`text-body-secondary ${line.discount ? 'text-success' : 'text-ink-2'}`}>
                    {quoteLineLabel(lang, line.label)}
                  </span>
                  <span className={`whitespace-nowrap text-body-secondary ${line.discount ? 'text-success' : 'text-ink'}`}>
                    {quoteLineValue(lang, line)}
                  </span>
                </div>
              ))}
            </div>
          ) : (
            <div className="py-2 text-body-secondary text-ink-2">{t('Считаем стоимость…')}</div>
          )}
          <div className="hairline-full my-2" />
          <div className="flex items-baseline justify-between">
            <span className="text-headline-sm text-ink">
              {productName(lang, product.code, product.name)}, {worksLabel(draft.works)}
            </span>
            <span className="text-headline-lg tabular-nums text-accent">{money(quote?.total ?? 0)}</span>
          </div>
        </Card>
        <span className="px-1 pt-1.5 text-caption text-ink-2">{t('Цена фиксируется при отправке заказа.')}</span>
      </div>

      <div className="flex flex-col" data-field="consent">
        <button
          type="button"
          onClick={() => update({ consent: !draft.consent })}
          className="flex items-start gap-2 px-1 py-1 text-left"
        >
          <span
            className={`mt-0.5 flex size-5 shrink-0 items-center justify-center rounded ${
              draft.consent ? 'bg-accent text-btn-text' : 'border border-line bg-surface'
            }`}
          >
            {draft.consent ? <Icon name="check" size={16} /> : null}
          </span>
          <span className="text-caption leading-tight text-ink">
            {t('Согласен(а) с ')}
            <a
              href="/legal/offer"
              onClick={(event) => {
                event.preventDefault();
                navigate('/legal/offer');
              }}
              className="text-accent underline"
            >
              {t('условиями оферты')}
            </a>
            {t(' и ')}
            <a
              href="/legal/personal"
              onClick={(event) => {
                event.preventDefault();
                navigate('/legal/personal');
              }}
              className="text-accent underline"
            >
              {t('обработкой персональных данных')}
            </a>
          </span>
        </button>
        <FieldError>{errors.consent}</FieldError>
      </div>
    </>
  );
}
