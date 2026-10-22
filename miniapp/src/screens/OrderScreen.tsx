import { useCallback, useEffect, useMemo, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { api, resultFileUrl, ApiError } from '@/api/client';
import type { Order, ResultFile } from '@/api/types';
import { MainButton, BackButton } from '@/telegram/hooks';
import { haptic, openLink, openTelegramLink, copyText } from '@/telegram/sdk';
import { useAsync, useOrderStart, usePolling, useSettings } from '@/lib/hooks';
import { isActive, PRODUCTS_FALLBACK, PROGRESS_STEPS, progressIndex, SUPPORT_HANDLE } from '@/lib/catalog';
import { formatDateTime, formatSize, money, unitsLabel, worksLabel } from '@/lib/format';
import { AppHeader, Banner, Card, Hairline, Icon, Row, Screen, Skeleton, StatusChip } from '@/components/ui';
import { Sheet } from '@/components/Sheet';
import { useFeedback } from '@/components/Toast';
import { useI18n, useT } from '@/i18n';
import { productName, progressStepLabel } from '@/i18n/catalog';
import { SenderField, senderNameError, senderNameValid } from './order/SenderForm';
import { PaymentBlock } from './order/QrPlate';

/**
 * S07/S08 · Order screen: payment first («Я оплатил»), then live status,
 * then the result. Polls every 8 s while the order is active (design.md §12.5).
 */
export function OrderScreen() {
  const { number = '' } = useParams();
  const navigate = useNavigate();
  const feedback = useFeedback();
  const { t, lang } = useI18n();
  const startOrder = useOrderStart();

  const orders = useAsync(() => api.myOrders(), []);
  const { data: order, error, loading, reload } = useAsync(() => api.order(number), [number]);
  const settings = useSettings();
  const supportHandle = settings?.supportTelegram ?? SUPPORT_HANDLE;
  const [timelineOpen, setTimelineOpen] = useState(false);
  const [sender, setSender] = useState('');
  const [senderError, setSenderError] = useState<string | undefined>(undefined);
  const [claiming, setClaiming] = useState(false);
  const [revisionOpen, setRevisionOpen] = useState(false);
  const [revisionNote, setRevisionNote] = useState('');
  const [revisionSending, setRevisionSending] = useState(false);

  // The corrections the client asked for (kept when the operator reopens the
  // request, also after a reload of the screen).
  useEffect(() => {
    if (order?.revisionNote) setRevisionNote(order.revisionNote);
  }, [order?.revisionNote]);

  const submitRevision = async () => {
    const note = revisionNote.trim();
    if (note.length < 5) {
      haptic.error();
      feedback(t('Опишите, что нужно исправить (минимум 5 символов)'), 'error');
      return;
    }
    setRevisionSending(true);
    try {
      await api.requestRevision(number, note);
      haptic.success();
      feedback(t('Правки отправлены оператору'), 'success');
      setRevisionOpen(false);
      reload();
    } catch (cause) {
      haptic.error();
      feedback(cause instanceof ApiError ? t(cause.message) : t('Не удалось отправить правки'), 'error');
    } finally {
      setRevisionSending(false);
    }
  };

  const active = order ? isActive(order.status) : false;
  usePolling(
    () => {
      reload();
    },
    8000,
    active,
  );

  // Keep the last known sender name in the field (rejected → «Я оплатил ещё раз»).
  useEffect(() => {
    if (order?.senderName) setSender(order.senderName);
  }, [order?.senderName]);

  const handleClaim = useCallback(async () => {
    const validation = senderNameError(sender);
    if (validation || !senderNameValid(sender)) {
      setSenderError(validation ?? t('Укажите отправителя платежа'));
      haptic.error();
      return;
    }
    setClaiming(true);
    setSenderError(undefined);
    try {
      await api.claim(number, sender.trim());
      haptic.success();
      feedback(t('Оплата отмечена - оператор проверяет поступление'), 'success');
      reload();
      orders.reload();
    } catch (cause) {
      const message =
        cause instanceof Error && cause.message ? cause.message : t('Не удалось отметить оплату, попробуйте ещё раз');
      setSenderError(message);
      feedback(t(message), 'error');
    } finally {
      setClaiming(false);
    }
  }, [sender, number, reload, orders, feedback, t]);

  const download = useCallback(() => {
    const url = order ? resultFileUrl(order) : undefined;
    if (!url) return;
    haptic.tap();
    openLink(url);
  }, [order]);

  // One document per ordered work: the card lists them, the main button opens
  // every link (the browser may open them as separate tabs).
  const resultFiles = useMemo<ResultFile[]>(() => {
    if (!order) return [];
    if (order.resultFiles?.length) return order.resultFiles;
    return order.resultFile ? [order.resultFile] : [];
  }, [order]);

  const downloadFile = useCallback(
    (file?: ResultFile) => {
      const url = order ? resultFileUrl(order, file) : undefined;
      if (!url) return;
      haptic.tap();
      openLink(url);
    },
    [order],
  );

  const copyNumber = useCallback(() => {
    copyText(number).then(() => feedback(t('Номер заказа скопирован'), 'success'));
  }, [number, feedback, t]);

  const inMyOrders = Boolean(orders.data?.orders.some((item) => item.number === number));

  // A deep link to a foreign or unknown order: neutral not found, no existence leak.
  if (error || (!loading && !order)) {
    return (
      <Screen header={<AppHeader title={t('Заказ')} onBack={() => navigate('/orders')} />}>
        <div className="flex flex-col items-center gap-2 px-6 py-16 text-center">
          <Icon name="search_off" size={48} className="text-outline-variant" />
          <span className="pt-2 text-headline-md text-ink">{t('Заказ не найден')}</span>
          <span className="text-body-secondary text-ink-2">{t('Проверьте номер - он указан в сообщении бота.')}</span>
        </div>
        <MainButton text={t('Мои заказы')} onClick={() => navigate('/orders')} />
        <BackButton onClick={() => navigate('/orders')} />
      </Screen>
    );
  }

  if (!order) {
    return (
      <Screen header={<AppHeader title={t('Детали заказа')} onBack={() => navigate('/orders')} />} bottomSpace>
        <div className="flex flex-col gap-3 pt-2">
          <Skeleton className="h-10 w-48" />
          <Skeleton className="h-16 w-full" />
          <Skeleton className="h-40 w-full" />
        </div>
      </Screen>
    );
  }

  const product = PRODUCTS_FALLBACK.find((item) => item.code === order.product) ?? PRODUCTS_FALLBACK[0];
  const step = progressIndex(order.status);

  const main = (() => {
    if (order.status === 'awaiting_payment' || order.status === 'payment_rejected') {
      return {
        text: order.status === 'payment_rejected' ? t('Я оплатил ещё раз') : t('Я оплатил'),
        run: handleClaim,
        disabled: claiming,
        progress: claiming,
      };
    }
    switch (order.status) {
      case 'completed':
        if (resultFiles.length === 0) return undefined;
        if (resultFiles.length === 1) {
          return { text: t('Скачать файл'), run: download, disabled: false, progress: false };
        }
        return {
          text: t('Скачать все ({n})', { n: resultFiles.length }),
          run: () => {
            resultFiles.forEach((file, index) => {
              window.setTimeout(() => downloadFile(file), index * 600);
            });
          },
          disabled: false,
          progress: false,
        };
      case 'expired':
      case 'cancelled':
        return { text: t('Оформить новый заказ'), run: () => void startOrder(), disabled: false, progress: false };
      default:
        return undefined;
    }
  })();

  return (
    <Screen header={<AppHeader title={t('Детали заказа')} onBack={() => navigate(inMyOrders ? '/orders' : '/')} />} bottomSpace>
      <div className="flex flex-col gap-3 pt-2">
        {/* Identifier + status */}
        <div className="flex items-center justify-between gap-2 px-1">
          <button type="button" onClick={copyNumber} className="flex items-center gap-2 active:opacity-60">
            <span className="font-mono text-[15px] font-medium tracking-wide text-ink">{order.number}</span>
            <Icon name="content_copy" size={16} className="text-ink-2" />
          </button>
          <StatusChip status={order.status} />
        </div>

        {/* Progress tracker */}
        <ProgressTracker step={step} />

        {/* Summary card */}
        <Card className="p-4">
          <div className="flex flex-col gap-1">
            <span className="text-body-strong text-ink">
              {productName(lang, product.code, product.name)} · {unitsLabel(order.units, product)} · {worksLabel(order.works)}
            </span>
            <span className="line-clamp-2 text-body-secondary text-ink-2">
              {order.workKind ? `${order.workKind}: ` : t('Тема: ')}
              {order.topics}
            </span>
            <div className="flex items-center justify-between pt-1">
              <span className="text-body-strong text-ink">{money(order.amount)}</span>
              <span className="text-caption text-ink-2">{formatDateTime(order.createdAt)}</span>
            </div>
          </div>
        </Card>

        {/* Status block */}
        {order.status === 'awaiting_payment' || order.status === 'payment_rejected' ? (
          <>
            {order.status === 'payment_rejected' ? (
              <Banner tone="danger" icon="error" title={t('Оплата не найдена')}>
                {order.paymentRejectedReason
                  ? t('Причина: {reason}. Проверьте данные платежа и попробуйте ещё раз.', {
                      reason: order.paymentRejectedReason,
                    })
                  : t('Проверьте данные платежа и попробуйте ещё раз либо напишите в поддержку.')}
              </Banner>
            ) : null}
            <PaymentBlock
              amount={order.amount}
              recipient={settings?.paymentRecipient}
              link={settings?.paymentLink}
              qrUrl={settings?.paymentQr ?? undefined}
            />
            <SenderField value={sender} onChange={setSender} error={senderError} />
          </>
        ) : null}

        {order.status === 'payment_claimed' ? (
          <Banner tone="warning" icon="hourglass_top" title={t('Оплата проверяется')}>
            {t('Оператор проверяет поступление - обычно это несколько минут. Мы пришлём сообщение в чат.')}
          </Banner>
        ) : null}

        {order.status === 'queued' ? (
          <Banner tone="info" icon="schedule" title={t('Заказ в работе')}>
            {settings?.queuePaused
              ? t('Заказ подтверждён и ждёт обработки - начнём, как только возобновим очередь.')
              : t('Оплата подтверждена, заказ ждёт обработки. Обычно от 3 до 20 минут.')}
          </Banner>
        ) : null}

        {order.status === 'generating' ? (
          <>
            <Banner tone="info" icon="autorenew" title={t('Готовим документ')}>
              {t('Обычно от 3 до 20 минут. Пришлём файл в чат, как только всё будет готово.')}
            </Banner>
            <Card className="gap-2 p-4">
              <Skeleton className="h-5 w-40" />
              <Skeleton className="h-4 w-56" />
              <Skeleton className="h-4 w-32" />
            </Card>
          </>
        ) : null}

        {order.status === 'failed_final' ? (
          <Banner tone="danger" icon="report" title={t('Не удалось подготовить работу')}>
            {t('Оператор уже разбирается. Пришлём сообщение, когда решим вопрос.')}
          </Banner>
        ) : null}

        {order.status === 'expired' || order.status === 'cancelled' ? (
          <Banner tone="info" icon="schedule" title={order.status === 'expired' ? t('Срок оплаты истёк') : t('Заказ отменён')}>
            {order.status === 'expired'
              ? t('Оформите новый заказ - цены и скидки сохранились.')
              : t('Если это ошибка - напишите в поддержку, поможем восстановить.')}
          </Banner>
        ) : null}

        {order.status === 'completed' && resultFiles.length > 0 ? (
          <>
            <span className="px-1 pt-1 text-section-label uppercase text-ink-2">
              {resultFiles.length > 1 ? t('Работы готовы') : t('Работа готова')}
            </span>
            <Card>
              {resultFiles.map((file, index) => (
                <div key={file.id ?? file.name}>
                  {index > 0 ? <Hairline /> : null}
                  <div className="flex items-center gap-3 p-4">
                    <span className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-accent-soft text-accent">
                      <Icon name="description" size={22} />
                    </span>
                    <div className="flex min-w-0 flex-1 flex-col">
                      <span className="truncate text-body-strong text-ink">{file.name}</span>
                      <span className="text-caption text-ink-2">
                        {formatSize(file.size)} · {t('доступен 30 дней')}
                      </span>
                    </div>
                    <button
                      type="button"
                      onClick={() => downloadFile(file)}
                      className="flex items-center gap-1 text-body-secondary-strong text-accent active:opacity-60"
                    >
                      <Icon name="download" size={20} />
                      {t('Скачать')}
                    </button>
                  </div>
                </div>
              ))}
              <Hairline />
              <div className="flex items-center gap-2 px-4 py-3 text-caption text-ink-2">
                <Icon name="verified" size={18} className="text-success" filled />
                {t('Внесём правки преподавателя бесплатно - просто напишите в поддержку.')}
              </div>
              <Hairline />
              {order.revisionRequestedAt ? (
                <div className="flex items-center gap-2 px-4 py-3 text-caption text-accent">
                  <Icon name="hourglass_top" size={18} filled />
                  {t('Правки отправлены оператору - ждём подтверждения.')}
                </div>
              ) : (
                <button
                  type="button"
                  onClick={() => {
                    haptic.tap();
                    setRevisionOpen(true);
                  }}
                  className="flex w-full items-center gap-2 px-4 py-3 text-left text-body-secondary-strong text-accent transition active:opacity-60"
                >
                  <Icon name="edit_note" size={20} />
                  {t('Нужны правки')}
                </button>
              )}
            </Card>
            <Card>
              <Row icon="star" title={t('Оставить отзыв')} onClick={() => navigate(`/reviews?order=${order.number}`)} />
            </Card>
          </>
        ) : null}

        {/* Timeline */}
        <span className="px-1 pt-1 text-section-label uppercase text-ink-2">{t('История заказа')}</span>
        <Card className="px-4 py-3">
          <Timeline order={order} open={timelineOpen} onToggle={() => setTimelineOpen((value) => !value)} />
        </Card>

        <Card>
          <Row
            icon="support_agent"
            title={t('Что-то не так?')}
            subtitle={t('Написать в поддержку {handle}', { handle: supportHandle })}
            onClick={() => {
              haptic.tap();
              openTelegramLink(`https://t.me/${supportHandle.replace('@', '')}`);
            }}
          />
        </Card>
      </div>

      {main ? (
        <MainButton text={main.text} onClick={main.run} disabled={main.disabled} progress={main.progress} />
      ) : null}
      <BackButton onClick={() => navigate(inMyOrders ? '/orders' : '/')} />

      <Sheet open={revisionOpen} title={t('Нужны правки')} onClose={() => setRevisionOpen(false)}>
        <div className="flex flex-col gap-3 px-4 pb-4">
          <span className="text-caption text-ink-2">
            {t('Опишите, что исправить: оператор подтвердит и отправит работу на доработку.')}
          </span>
          <textarea
            value={revisionNote}
            onChange={(event) => setRevisionNote(event.target.value)}
            rows={5}
            maxLength={2000}
            placeholder={t('Например: заменить рисунок 2 и добавить список литературы')}
            className="w-full resize-none rounded-xl bg-neutral-soft/60 p-3 text-body-secondary text-ink outline-none placeholder:text-hint"
          />
          <button
            type="button"
            disabled={revisionSending}
            onClick={() => void submitRevision()}
            className="h-11 rounded-xl bg-accent-soft px-4 text-body-secondary-strong text-accent transition active:scale-[0.98] disabled:opacity-50"
          >
            {revisionSending ? t('Отправляем…') : t('Отправить оператору')}
          </button>
        </div>
      </Sheet>
    </Screen>
  );
}

function ProgressTracker({ step }: { step: number }) {
  const { lang } = useI18n();
  return (
    <div className="flex items-start justify-between px-2 pb-1">
      {PROGRESS_STEPS.map((label, index) => {
        const done = index < step;
        const current = index === step - 1 && step <= 4;
        return (
          <div key={label} className="flex flex-1 flex-col items-center gap-1">
            <div className="flex w-full items-center">
              {index > 0 ? <span className={`h-0.5 flex-1 ${done || current ? 'bg-accent' : 'bg-line'}`} /> : <span className="flex-1" />}
              <span
                className={`flex size-5 items-center justify-center rounded-full border-2 ${
                  done
                    ? 'border-accent bg-accent text-btn-text'
                    : current
                      ? 'border-accent bg-surface'
                      : 'border-outline-variant bg-surface'
                }`}
              >
                {done ? <Icon name="check" size={12} /> : null}
              </span>
              {index < PROGRESS_STEPS.length - 1 ? (
                <span className={`h-0.5 flex-1 ${done ? 'bg-accent' : 'bg-line'}`} />
              ) : (
                <span className="flex-1" />
              )}
            </div>
            <span className={`text-step-label ${done || current ? 'text-ink' : 'text-ink-2'}`}>{progressStepLabel(lang, label)}</span>
          </div>
        );
      })}
    </div>
  );
}

function Timeline({ order, open, onToggle }: { order: Order; open: boolean; onToggle: () => void }) {
  const t = useT();
  const events = order.events;
  if (events.length === 0) {
    return <span className="text-body-secondary text-ink-2">{t('Событий пока нет')}</span>;
  }
  const visible = open ? events : events.slice(0, 3);

  return (
    <div className="flex flex-col">
      {visible.map((event, index) => (
        <div key={`${event.at}-${index}`} className="flex gap-3">
          <div className="flex flex-col items-center pt-1.5">
            <span className={`size-2 rounded-full ${index === 0 ? 'bg-accent' : 'bg-outline-variant'}`} />
            {index < visible.length - 1 ? <span className="w-px flex-1 bg-line" /> : null}
          </div>
          <div className="flex flex-1 items-start justify-between gap-2 pb-3">
            <span className="text-body-secondary text-ink">{event.text}</span>
            <span className="whitespace-nowrap text-caption text-ink-2">{formatDateTime(event.at)}</span>
          </div>
        </div>
      ))}
      {events.length > 3 ? (
        <button
          type="button"
          onClick={onToggle}
          className="self-start pl-5 text-caption text-accent active:opacity-60"
        >
          {open ? t('Свернуть') : t('Показать все ({n})', { n: events.length })}
        </button>
      ) : null}
    </div>
  );
}
