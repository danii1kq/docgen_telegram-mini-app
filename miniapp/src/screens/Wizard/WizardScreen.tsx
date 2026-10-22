import { useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { api, ApiError } from '@/api/client';
import type { Product, Quote } from '@/api/types';
import { useT } from '@/i18n';
import { BackButton, MainButton } from '@/telegram/hooks';
import { haptic, setClosingConfirmation, showPopup } from '@/telegram/sdk';
import { useDebounced, useScrollTop } from '@/lib/hooks';
import { PRODUCTS_FALLBACK } from '@/lib/catalog';
import { isDirty, loadDraft, saveDraft, toOrderForm, clearDraft, type Draft } from '@/lib/draft';
import { money } from '@/lib/format';
import { AppHeader, Icon, Screen, StepProgress } from '@/components/ui';
import { Step1 } from './Step1';
import { Step2 } from './Step2';
import { Step3 } from './Step3';
import { Step4 } from './Step4';

/**
 * S03-S06 · Order wizard: 4 steps on one route, the draft lives on the device
 * and every price comes from POST /api/quote. The MainButton is the primary
 * action, the Telegram BackButton steps back, and a dirty draft arms the
 * closing confirmation.
 */
export function WizardScreen() {
  const navigate = useNavigate();
  const t = useT();

  const [draft, setDraftState] = useState<Draft>(() => loadDraft());
  const [step, setStep] = useState(() => Math.min(4, Math.max(1, loadDraft().step)));
  // A draft that was started earlier and left at a later step: say so instead of
  // silently opening step 1 (the client thought nothing had been saved).
  const [restored, setRestored] = useState(() => {
    const saved = loadDraft();
    return isDirty(saved) && saved.step > 1;
  });
  const [fields, setFields] = useState<Record<string, string>>({});
  const [banner, setBanner] = useState<string | undefined>(undefined);
  const [submitting, setSubmitting] = useState(false);
  const idempotencyKey = useRef<string>(crypto.randomUUID());

  const [catalogue, setCatalogue] = useState<Product[]>(PRODUCTS_FALLBACK);
  const [quote, setQuote] = useState<Quote | undefined>(undefined);
  const [promo, setPromo] = useState<{ code: string; discountPct: number } | undefined>(undefined);

  useEffect(() => {
    api
      .products()
      .then((items) => {
        setCatalogue(items);
      })
      .catch(() => {
        /* the fallback catalogue keeps the wizard usable */
      });
  }, []);

  const update = (patch: Partial<Draft>) => {
    setDraftState((current) => ({ ...current, ...patch }));
  };

  // Persist the form and the step (the draft continues where it was left) plus
  // the closing confirmation while it is dirty.
  useEffect(() => {
    saveDraft({ ...draft, step });
  }, [draft, step]);

  const goToStep = (next: number) => {
    setRestored(false);
    setStep(next);
  };

  const dirty = useMemo(() => isDirty(draft), [draft]);
  useEffect(() => {
    setClosingConfirmation(dirty);
    return () => setClosingConfirmation(false);
  }, [dirty]);

  // Jump back to the top on every step change: the new step starts at its
  // heading instead of wherever the previous step was scrolled to.
  useScrollTop(step);

  // Live quote (debounced, server-side pricing). The applied promo takes part
  // in the calculation so the summary matches what the order will be created
  // with (design S06: base − volume − promo = total).
  const quoteKey = `${draft.product}|${draft.units}|${draft.works}|${promo?.code ?? ''}`;
  const debouncedKey = useDebounced(quoteKey, 300);
  useEffect(() => {
    const [product, units, works, promoCode] = debouncedKey.split('|');
    let alive = true;
    api
      .quote({ product, units: Number(units), works: Number(works), promo: promoCode || null })
      .then((value) => {
        if (alive) setQuote(value);
      })
      .catch(() => {});
    return () => {
      alive = false;
    };
  }, [debouncedKey]);

  const leave = async () => {
    if (step > 1) {
      haptic.tap();
      goToStep(step - 1);
      return;
    }
    if (dirty) {
      const answer = await showPopup({
        title: t('Выйти из оформления?'),
        message: t('Черновик сохранится - вы сможете вернуться к нему позже.'),
        buttons: [
          { id: 'stay', text: t('Остаться'), type: 'cancel' },
          { id: 'leave', text: t('Выйти'), type: 'destructive' },
        ],
      });
      if (answer !== 'leave') return;
    }
    setClosingConfirmation(false);
    navigate('/');
  };

  const validateStep = (current: Draft, target: number): Record<string, string> => {
    const errors: Record<string, string> = {};
    if (target >= 2) {
      if (current.product === 'other' && !current.workKind.trim()) errors.workKind = t('Укажите вид работы');
      if (!current.units || current.units < 1) errors.units = t('Укажите объём');
    }
    if (target >= 3) {
      if (current.topics.trim().length < 3) errors.topics = t('Укажите тему работы (минимум 3 символа)');
    }
    if (target >= 4) {
      if (!current.institute.trim()) errors.institute = t('Укажите институт');
      if (!current.department.trim()) errors.department = t('Укажите кафедру');
      if (!current.studentName.trim()) errors.studentName = t('Укажите ФИО студента');
      if (!current.disciplineCode.trim()) errors.disciplineCode = t('Укажите шифр ОП');
      if (!current.teacherName.trim()) errors.teacherName = t('Укажите ФИО преподавателя');
    }
    return errors;
  };

  const goNext = () => {
    const errors = validateStep(draft, step + 1);
    setFields(errors);
    setBanner(undefined);
    if (Object.keys(errors).length > 0) {
      haptic.error();
      const first = Object.keys(errors)[0];
      requestAnimationFrame(() => {
        document.querySelector<HTMLElement>(`[data-field="${first}"]`)?.scrollIntoView({ block: 'center' });
      });
      return;
    }
    haptic.tap();
    goToStep(Math.min(4, step + 1));
  };

  const submit = async () => {
    const errors = validateStep(draft, 4);
    if (!draft.consent) errors.consent = t('Нужно согласие, чтобы создать заказ');
    if (draft.contactName.trim().length < 2) errors.contactName = t('Укажите имя для связи');
    setFields(errors);
    if (Object.keys(errors).length > 0) {
      haptic.error();
      return;
    }

    setSubmitting(true);
    setBanner(undefined);
    try {
      const promo = draft.promoCode.trim() ? draft.promoCode.trim().toUpperCase() : null;
      const result = await api.createOrder(toOrderForm(draft, promo), idempotencyKey.current);
      if (result.promoDropReason) {
        setBanner(t('Промокод не применён: {reason}', { reason: result.promoDropReason }));
      }
      setClosingConfirmation(false);
      clearDraft();
      haptic.success();
      navigate(`/order/${result.number}`, { replace: true });
    } catch (cause) {
      setSubmitting(false);
      if (cause instanceof ApiError) {
        if (cause.status === 401) {
          setBanner(t('Откройте приложение заново через бота'));
        } else if (cause.fields) {
          setFields(cause.fields);
          haptic.error();
          const first = Object.keys(cause.fields)[0];
          requestAnimationFrame(() => {
            document.querySelector<HTMLElement>(`[data-field="${first}"]`)?.scrollIntoView({ block: 'center' });
          });
        } else {
          setBanner(cause.message);
        }
      } else {
        setBanner(t('Не удалось создать заказ, попробуйте ещё раз'));
      }
    }
  };

  const stepTitles = [t('Вид работы'), t('Тема и материалы'), t('Титульный лист'), t('Подтверждение')];
  // The live server quote (debounced 300 ms) rides on the primary button, so the
  // price is visible on every step - not only on the final summary.
  const price = quote ? ` · ${money(quote.total)}` : '';
  const mainText = step === 4 ? `${t('Перейти к оплате')}${price}` : `${t('Далее')}${price}`;

  return (
    <Screen
      header={<AppHeader title={t('Мастер заказа')} onBack={leave} />}
      bottomSpace
    >
      <div className="flex flex-col gap-1 px-1 pb-2 pt-1">
        <div className="flex items-center justify-between">
          <h2 className="text-headline-lg text-ink">{stepTitles[step - 1]}</h2>
          <span className="text-caption text-ink-2">{t('Шаг {n} из 4', { n: step })}</span>
        </div>
        {restored ? <span className="text-caption text-accent">{t('Черновик восстановлен')}</span> : null}
        <StepProgress step={step} />
      </div>

      {banner ? (
        <div className="fade-in flex items-start gap-2 rounded-[14px] bg-danger-soft px-4 py-3 text-danger">
          <Icon name="error" size={20} filled />
          <span className="text-caption">{banner}</span>
        </div>
      ) : null}

      <div className="flex flex-col gap-4 pt-2">
        {step === 1 ? <Step1 draft={draft} update={update} products={catalogue} quote={quote} errors={fields} /> : null}
        {step === 2 ? <Step2 draft={draft} update={update} errors={fields} /> : null}
        {step === 3 ? <Step3 draft={draft} update={update} errors={fields} /> : null}
        {step === 4 ? (
          <Step4
            draft={draft}
            update={update}
            quote={quote}
            errors={fields}
            onGoToStep={goToStep}
            promo={promo}
            onPromoChange={(next) => {
              setPromo(next);
              update({ promoCode: next?.code ?? '' });
            }}
          />
        ) : null}
      </div>

      <MainButton text={mainText} progress={submitting} disabled={submitting} onClick={step === 4 ? submit : goNext} />
      <BackButton onClick={leave} />
    </Screen>
  );
}

/** The wizard's academic data lives on the device if the client returns later. */
export function readWizardDraft(): Draft {
  return loadDraft();
}
