import { useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { api, resultFileUrl } from '@/api/client';
import { BackButton, MainButton } from '@/telegram/hooks';
import { haptic, openLink, copyText, showConfirm } from '@/telegram/sdk';
import { useAsync, usePolling } from '@/lib/hooks';
import { PRODUCTS_FALLBACK, progressIndex, PROGRESS_STEPS } from '@/lib/catalog';
import { formatDateTime, formatSize, money, unitsLabel } from '@/lib/format';
import {
  AppHeader,
  Banner,
  Card,
  FieldError,
  Hairline,
  Icon,
  Row,
  Screen,
  SectionLabel,
  Skeleton,
  StatusChip,
} from '@/components/ui';
import { Sheet, SheetOption } from '@/components/Sheet';
import { useToast } from '@/components/Toast';

const REJECT_REASONS = [
  'Сумма перевода не совпадает с чеком',
  'Отправитель платежа не указан',
  'Поступление не найдено в выписке банка',
  'Отправитель не совпадает с клиентом',
];

/**
 * Operator panel · one order: receipt check (confirm/reject), retry/cancel,
 * the operator note, the audit timeline and the live generation console.
 */
export function OperatorOrderScreen() {
  const { number = '' } = useParams();
  const navigate = useNavigate();
  const toast = useToast();

  const { data: order, error, loading, reload } = useAsync(() => api.adminOrder(number), [number]);
  const [busy, setBusy] = useState(false);
  const [checked, setChecked] = useState(false);
  const [rejectOpen, setRejectOpen] = useState(false);
  const [reason, setReason] = useState(REJECT_REASONS[0]);
  const [comment, setComment] = useState('');
  const [note, setNote] = useState('');
  const [noteSaved, setNoteSaved] = useState(true);
  const [rework, setRework] = useState('');

  useEffect(() => {
    if (order) {
      setNote(order.operatorNotes ?? '');
      setNoteSaved(true);
      setRework(order.revisionNote ?? '');
      if (order.status !== 'payment_claimed') setChecked(false);
    }
  }, [order?.number, order?.status, order?.operatorNotes, order?.revisionNote]);

  const generation = order?.status === 'generating';
  // The console stays available for finished orders: the log and the final cost
  // must remain visible after the work is delivered.
  const consoleWanted =
    Boolean(order) && ['queued', 'generating', 'completed', 'failed_final'].includes(order?.status ?? '');
  const consoleData = useAsync(
    () => (consoleWanted ? api.adminConsole(number) : Promise.resolve(undefined)),
    [number, consoleWanted],
  );
  usePolling(() => {
    reload();
    consoleData.reload();
  }, generation ? 5000 : 15000, Boolean(order) && ['generating', 'queued', 'payment_claimed'].includes(order?.status ?? ''));

  // The live console follows the tail while the operator stays at the bottom:
  // polling only appends new events, it never reloads the page.
  const consoleRef = useRef<HTMLDivElement | null>(null);
  const consoleLines = consoleData.data?.lines.length ?? 0;
  useEffect(() => {
    const el = consoleRef.current;
    if (!el) return;
    const nearBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
    if (nearBottom) el.scrollTop = el.scrollHeight;
  }, [consoleLines]);

  const product = useMemo(
    () => PRODUCTS_FALLBACK.find((item) => item.code === order?.product) ?? PRODUCTS_FALLBACK[0],
    [order?.product],
  );

  const run = async (action: () => Promise<unknown>, success: string) => {
    setBusy(true);
    try {
      await action();
      haptic.success();
      toast.show(success);
      reload();
    } catch (cause) {
      haptic.error();
      toast.show(cause instanceof Error ? cause.message : 'Не удалось выполнить действие', 'error');
    } finally {
      setBusy(false);
    }
  };

  const submitRework = async () => {
    const note = rework.trim();
    if (note.length < 5) {
      haptic.error();
      toast.show('Опишите правки для генератора (минимум 5 символов)', 'error');
      return;
    }
    const confirmed = await showConfirm(
      `Отправить ${number} на доработку? Готовый файл уйдёт агенту вместе с правками.`,
    );
    if (!confirmed) return;
    await run(() => api.adminRework(number, note), 'Заказ отправлен на доработку');
  };

  if (error) {
    return (
      <Screen header={<AppHeader title="Заказ" onBack={() => navigate('/operator/orders')} />}>
        <Card className="items-center gap-2 p-8">
          <Icon name="search_off" size={32} className="text-outline-variant" />
          <span className="text-body-secondary text-ink-2">{error}</span>
        </Card>
        <BackButton onClick={() => navigate('/operator/orders')} />
      </Screen>
    );
  }

  if (!order || loading) {
    return (
      <Screen header={<AppHeader title="Заказ" onBack={() => navigate('/operator/orders')} />}>
        <div className="flex flex-col gap-3 pt-2">
          <Skeleton className="h-16 w-full" />
          <Skeleton className="h-32 w-full" />
          <Skeleton className="h-24 w-full" />
        </div>
      </Screen>
    );
  }

  const canConfirm = order.status === 'payment_claimed';
  const canReject = order.status === 'payment_claimed';
  const canRetry = order.status === 'failed_final' || order.status === 'payment_rejected';
  const canCancel = ['awaiting_payment', 'payment_claimed', 'payment_rejected', 'queued'].includes(order.status);

  return (
    <Screen header={<AppHeader title={order.number} onBack={() => navigate('/operator/orders')} />} bottomSpace>
      <div className="flex flex-col gap-3 pt-2">
        <div className="flex items-center justify-between gap-2 px-1">
          <button
            type="button"
            onClick={() => copyText(order.number).then(() => toast.show('Номер скопирован'))}
            className="flex items-center gap-2 active:opacity-60"
          >
            <span className="font-mono text-[15px] font-medium tracking-wide text-ink">{order.number}</span>
            <Icon name="content_copy" size={16} className="text-ink-2" />
          </button>
          <StatusChip status={order.status} />
        </div>

        {/* Client + order facts */}
        <Card className="gap-2 p-4">
          <div className="flex items-center justify-between">
            <span className="text-headline-md text-ink">{money(order.amount)}</span>
            <span className="text-caption text-ink-2">{formatDateTime(order.createdAt)}</span>
          </div>
          <span className="text-body-strong text-ink">
            {product.name} · {unitsLabel(order.units, product)} · {order.works} работа
          </span>
          <span className="line-clamp-3 text-body-secondary text-ink-2">{order.topics}</span>
          {order.promoCode ? (
            <span className="text-caption text-success">
              Промокод {order.promoCode} · −{order.promoDiscountPct ?? 0} %
            </span>
          ) : null}
          {order.agentCostUsd ? (
            <span className="text-caption text-ink-2">
              Затраты генерации: ${order.agentCostUsd.toFixed(4)}
              {order.agentCostKzt ? ` · ${money(order.agentCostKzt)}` : ''}
            </span>
          ) : null}
        </Card>

        <SectionLabel>Клиент</SectionLabel>
        <Card>
          <Row
            icon="person"
            title={order.contactName || order.studentName || 'Не указан'}
            subtitle={order.contact ? `Telegram: ${order.contact}` : 'Контакт не привязан'}
          />
          {order.senderName ? (
            <>
              <Hairline />
              <Row
                icon="payments"
                iconTone="success"
                title={`Отправитель Kaspi: ${order.senderName}`}
                subtitle="Сверьте с выпиской банка"
              />
            </>
          ) : null}
          <Hairline />
          <Row
            icon="school"
            iconTone="ink-2"
            title={`${order.institute} · ${order.department}`}
            subtitle={`${order.studentName} · ${order.disciplineCode} · ${order.teacherName}`}
          />
        </Card>

        {/* Receipt verification */}
        {canConfirm || canReject ? (
          <Card className="gap-3 p-4">
            <span className="text-headline-sm text-ink">Проверка оплаты</span>
            <button
              type="button"
              onClick={() => setChecked((value) => !value)}
              className="flex items-start gap-2 text-left"
            >
              <span
                className={`mt-0.5 flex size-5 shrink-0 items-center justify-center rounded ${
                  checked ? 'bg-accent text-btn-text' : 'border border-line bg-surface'
                }`}
              >
                {checked ? <Icon name="check" size={16} /> : null}
              </span>
              <span className="text-caption text-ink">
                Я проверил(а) поступление по выписке Kaspi
              </span>
            </button>
            <div className="flex gap-2">
              <button
                type="button"
                disabled={!checked || busy}
                onClick={() => run(() => api.adminConfirm(order.number), `Оплата подтверждена: ${order.number}`)}
                className="flex h-11 flex-1 items-center justify-center gap-2 rounded-xl bg-accent text-body-secondary-strong text-btn-text transition active:scale-[0.98] disabled:opacity-40"
              >
                <Icon name="check" size={18} />
                Подтвердить
              </button>
              <button
                type="button"
                disabled={busy}
                onClick={() => setRejectOpen(true)}
                className="flex h-11 flex-1 items-center justify-center gap-2 rounded-xl bg-danger-soft text-body-secondary-strong text-danger transition active:scale-[0.98] disabled:opacity-40"
              >
                <Icon name="close" size={18} />
                Отклонить
              </button>
            </div>
          </Card>
        ) : null}

        {order.status === 'payment_rejected' && order.paymentRejectedReason ? (
          <Banner tone="danger" icon="error" title="Оплата отклонена">
            {order.paymentRejectedReason}
          </Banner>
        ) : null}

        {/* Progress + actions */}
        {canRetry || canCancel ? (
          <Card>
            {canRetry ? (
              <Row
                icon="restart_alt"
                title="Повторить генерацию"
                subtitle="Вернёт заказ в очередь"
                onClick={() => run(() => api.adminRetry(order.number), 'Заказ снова в очереди')}
              />
            ) : null}
            {canRetry && canCancel ? <Hairline /> : null}
            {canCancel ? (
              <Row
                icon="cancel"
                title="Отменить заказ"
                subtitle="Клиент получит уведомление"
                destructive
                onClick={() => run(() => api.adminCancel(order.number, 'отменён оператором'), 'Заказ отменён')}
              />
            ) : null}
          </Card>
        ) : null}

        {/* Live console */}
        {generation || consoleData.data?.lines.length ? (
          <>
            <SectionLabel
              action={
                <span className="flex items-center gap-3">
                  {consoleData.data?.costUsd ? (
                    <span className="text-caption text-ink-2">
                      ${consoleData.data.costUsd.toFixed(4)}
                      {consoleData.data.costKzt ? ` · ${money(consoleData.data.costKzt)}` : ''}
                    </span>
                  ) : null}
                  <button
                    type="button"
                    onClick={() =>
                      openLink(
                        consoleData.data?.logUrl
                          ? `${location.origin}${consoleData.data.logUrl}`
                          : api.adminConsoleLogUrl(order.number),
                      )
                    }
                    className="text-caption text-accent active:opacity-60"
                  >
                    Лог
                  </button>
                </span>
              }
            >
              Консоль генерации
            </SectionLabel>
            <Card className="p-3">
              <div
                ref={consoleRef}
                className="flex max-h-64 flex-col gap-1 overflow-y-auto font-mono text-[12px] leading-[18px]"
              >
                {consoleData.loading && !consoleData.data ? (
                  <Skeleton className="h-16 w-full" />
                ) : consoleData.data?.lines.length ? (
                  consoleData.data.lines.slice(-40).map((line, index) => (
                    <span
                      key={index}
                      className={
                        line.kind === 'error'
                          ? 'text-danger'
                          : line.kind === 'result'
                            ? 'text-success'
                            : line.kind === 'usage'
                              ? 'text-ink-2'
                              : 'text-ink'
                      }
                    >
                      {line.at ? `${line.at.slice(11, 19)} ` : ''}
                      {line.text}
                    </span>
                  ))
                ) : (
                  <span className="text-ink-2">Пока нет событий</span>
                )}
              </div>
            </Card>
          </>
        ) : null}

        {/* Result */}
        {order.resultFiles?.length || order.resultFile ? (
          <>
            <SectionLabel>
              {order.resultFiles && order.resultFiles.length > 1
                ? `Результат · ${order.resultFiles.length}`
                : 'Результат'}
            </SectionLabel>
            <Card>
              {(order.resultFiles?.length ? order.resultFiles : [order.resultFile!]).map((file, index) => (
                <div key={file.id ?? file.name}>
                  {index > 0 ? <Hairline /> : null}
                  <Row
                    icon="description"
                    title={file.name}
                    subtitle={`${formatSize(file.size)} · клиент получил файл в чат`}
                    value={
                      <button
                        type="button"
                        onClick={() => {
                          const url = resultFileUrl(order, file);
                          if (url) openLink(url);
                        }}
                        className="text-accent"
                      >
                        Скачать
                      </button>
                    }
                  />
                </div>
              ))}
            </Card>
          </>
        ) : null}

        {/* Rework: the client asked for corrections, the operator confirms them */}
        {order.status === 'completed' || order.status === 'failed_final' ? (
          <>
            <SectionLabel>Доработка</SectionLabel>
            <Card className="gap-2 p-4">
              <span className="text-caption text-ink-2">
                {order.revisionRequestedAt
                  ? 'Клиент просит правки: проверьте текст, дополните при необходимости и отправьте работу на доработку.'
                  : 'Можно вернуть готовую работу на генерацию: правки уйдут агенту вместе с текущим файлом.'}
              </span>
              <textarea
                value={rework}
                rows={4}
                maxLength={2000}
                placeholder="Что исправить: заменить рисунок 2, добавить список литературы…"
                onChange={(event) => setRework(event.target.value)}
                className="w-full resize-none border-0 bg-transparent p-0 text-body-secondary text-ink outline-none placeholder:text-hint"
              />
              <button
                type="button"
                disabled={busy || rework.trim().length < 5}
                onClick={() => void submitRework()}
                className="flex h-11 items-center justify-center gap-2 rounded-xl bg-accent-soft text-body-secondary-strong text-accent transition active:scale-[0.98] disabled:opacity-40"
              >
                <Icon name="restart_alt" size={18} />
                Отправить на доработку
              </button>
            </Card>
          </>
        ) : null}

        {/* Operator note */}
        <SectionLabel>Заметка оператора</SectionLabel>
        <Card className="gap-2 p-4">
          <textarea
            value={note}
            rows={3}
            placeholder="Внутренняя заметка: клиент не берёт трубку, сверено в Kaspi…"
            onChange={(event) => {
              setNote(event.target.value);
              setNoteSaved(false);
            }}
            onBlur={() => {
              if (noteSaved) return;
              run(() => api.adminSaveNote(order.number, note), 'Заметка сохранена');
              setNoteSaved(true);
            }}
            className="w-full resize-none border-0 bg-transparent p-0 text-body-secondary text-ink outline-none placeholder:text-hint"
          />
          <span className="text-caption text-ink-2">{noteSaved ? 'Сохраняется автоматически' : 'Сохранится при потере фокуса'}</span>
          <FieldError />
        </Card>

        {/* Timeline */}
        <SectionLabel>История</SectionLabel>
        <Card className="gap-3 p-4">
          {order.events.map((event, index) => (
            <div key={`${event.at}-${index}`} className="flex items-start justify-between gap-2">
              <div className="flex min-w-0 items-start gap-2">
                <span
                  className={`mt-1.5 size-2 shrink-0 rounded-full ${
                    event.actor === 'operator'
                      ? 'bg-accent'
                      : event.actor === 'client'
                        ? 'bg-success'
                        : 'bg-outline-variant'
                  }`}
                />
                <span className="text-body-secondary text-ink">{event.text}</span>
              </div>
              <span className="shrink-0 text-caption text-ink-2">{formatDateTime(event.at)}</span>
            </div>
          ))}
        </Card>

        <span className="px-1 text-caption text-ink-2">
          Этап: {PROGRESS_STEPS[Math.min(PROGRESS_STEPS.length - 1, Math.max(0, progressIndex(order.status) - 1))]}
        </span>
      </div>

      {/* Reject sheet */}
      <Sheet open={rejectOpen} title="Причина отклонения" onClose={() => setRejectOpen(false)}>
        {REJECT_REASONS.map((item) => (
          <SheetOption key={item} title={item} selected={item === reason} onClick={() => setReason(item)} />
        ))}
        <div className="px-4 py-2">
          <textarea
            value={comment}
            rows={2}
            placeholder="Комментарий клиенту (необязательно)"
            onChange={(event) => setComment(event.target.value)}
            className="w-full resize-none rounded-xl bg-page p-3 text-body-secondary text-ink outline-none placeholder:text-hint"
          />
        </div>
        <div className="px-4 pb-2">
          <button
            type="button"
            disabled={busy}
            onClick={async () => {
              setRejectOpen(false);
              await run(
                () => api.adminReject(order.number, comment.trim() || reason),
                `Оплата отклонена: ${order.number}`,
              );
              setComment('');
            }}
            className="flex h-11 w-full items-center justify-center gap-2 rounded-xl bg-danger text-body-secondary-strong text-white transition active:scale-[0.98] disabled:opacity-50"
          >
            <Icon name="close" size={18} />
            Отклонить оплату
          </button>
        </div>
      </Sheet>

      <MainButton
        text="Обновить карточку"
        onClick={() => {
          reload();
          consoleData.reload();
          toast.show('Обновлено');
        }}
      />
      <BackButton onClick={() => navigate('/operator/orders')} />
    </Screen>
  );
}
