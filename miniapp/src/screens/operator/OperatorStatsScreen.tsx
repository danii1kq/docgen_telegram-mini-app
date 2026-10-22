import { useNavigate } from 'react-router-dom';
import { api } from '@/api/client';
import { BackButton, MainButton } from '@/telegram/hooks';
import { useAsync } from '@/lib/hooks';
import { formatSize, money } from '@/lib/format';
import { STATUS_META } from '@/lib/catalog';
import { AppHeader, Card, Icon, Screen, SectionLabel, Skeleton } from '@/components/ui';
import { useToast } from '@/components/Toast';

/** Operator panel · dashboard: revenue, queue health and generation quality. */
export function OperatorStatsScreen() {
  const navigate = useNavigate();
  const toast = useToast();
  const { data, error, loading, reload } = useAsync(() => api.adminStats(), []);

  return (
    <Screen header={<AppHeader title="Статистика" onBack={() => navigate('/operator')} />} bottomSpace>
      <div className="flex flex-col gap-3 pt-2">
        {loading && !data ? (
          <div className="flex flex-col gap-2">
            <Skeleton className="h-24 w-full" />
            <Skeleton className="h-24 w-full" />
            <Skeleton className="h-24 w-full" />
          </div>
        ) : error || !data ? (
          <Card className="items-center gap-2 p-6">
            <Icon name="error" size={26} className="text-danger" />
            <span className="text-body-secondary text-ink-2">{error ?? 'Не удалось загрузить статистику'}</span>
            <button type="button" onClick={reload} className="text-body-secondary-strong text-accent">
              Повторить
            </button>
          </Card>
        ) : (
          <>
            {data.queueWarning ? (
              <div className="flex items-start gap-2 rounded-[14px] bg-warning-soft px-4 py-3 text-warning">
                <Icon name="warning" size={20} filled />
                <span className="text-caption">
                  Старейший заказ в очереди ждёт больше 30 минут - проверьте воркер и очередь.
                </span>
              </div>
            ) : null}

            <div className="grid grid-cols-2 gap-2">
              <Metric label="Подтверждённая выручка" value={money(data.confirmedRevenue)} tone="accent" />
              <Metric label="Выручка сегодня" value={money(data.revenueToday)} tone="success" />
              <Metric
                label="Затраты на генерацию"
                value={
                  data.agentCostUsd
                    ? `$${data.agentCostUsd.toFixed(4)} · ${money(data.agentCostKzt)}`
                    : '$0 · 0 ₸'
                }
                tone={data.agentCostUsd > 0 ? 'warning' : undefined}
              />
              <Metric
                label="Чистая выручка"
                value={money(data.netRevenue)}
                tone={data.netRevenue >= 0 ? 'success' : 'danger'}
              />
              <Metric label="Готово сегодня" value={String(data.completedToday)} />
              <Metric label="Заказов всего" value={String(data.ordersTotal)} />
              <Metric label="В очереди" value={String(data.queueDepth)} />
              <Metric
                label="Старейший в очереди"
                value={data.oldestQueuedMinutes < 0 ? '-' : `${Math.round(data.oldestQueuedMinutes)} мин`}
                tone={data.queueWarning ? 'danger' : undefined}
              />
              <Metric
                label="Средняя генерация"
                value={data.averageGenerationSeconds < 0 ? '-' : `${Math.round(data.averageGenerationSeconds / 60)} мин`}
              />
              <Metric
                label="Доля сбоев"
                value={`${data.failureRatePct.toFixed(1).replace('.', ',')} %`}
                tone={data.failureRatePct > 10 ? 'danger' : undefined}
              />
              <Metric label="Отзывы на модерации" value={String(data.pendingReviews)} tone={data.pendingReviews > 0 ? 'warning' : undefined} />
              <Metric label="Активные промокоды" value={String(data.activePromos)} />
              <Metric label="Диск" value={formatSize(data.diskUsedBytes)} />
              <Metric
                label="Курс $"
                value={data.usdRate ? `${data.usdRate.toFixed(2).replace('.', ',')} ₸` : '-'}
              />
              <Metric label="Обновлено" value={new Date(data.generatedAt).toLocaleTimeString('ru-RU').slice(0, 5)} />
            </div>

            <SectionLabel>Заказы по статусам</SectionLabel>
            <Card>
              {Object.entries(STATUS_META).map(([status, meta]) => {
                const count = data.ordersByStatus[status] ?? 0;
                return (
                  <div key={status} className="flex items-center justify-between px-4 py-2.5">
                    <span className="flex items-center gap-2 text-body-secondary text-ink">
                      <Icon name={meta.icon} size={18} filled={meta.filled} className="text-ink-2" />
                      {meta.label}
                    </span>
                    <span className="text-body-strong tabular-nums text-ink">{count}</span>
                  </div>
                );
              })}
            </Card>
          </>
        )}
      </div>

      <MainButton
        text="Обновить"
        onClick={() => {
          reload();
          toast.show('Статистика обновлена');
        }}
      />
      <BackButton onClick={() => navigate('/operator')} />
    </Screen>
  );
}

function Metric({
  label,
  value,
  tone,
}: {
  label: string;
  value: string;
  tone?: 'accent' | 'success' | 'warning' | 'danger';
}) {
  const color =
    tone === 'accent'
      ? 'text-accent'
      : tone === 'success'
        ? 'text-success'
        : tone === 'warning'
          ? 'text-warning'
          : tone === 'danger'
            ? 'text-danger'
            : 'text-ink';
  return (
    <Card className="gap-0.5 px-4 py-3">
      <span className={`text-headline-md tabular-nums ${color}`}>{value}</span>
      <span className="text-caption leading-tight text-ink-2">{label}</span>
    </Card>
  );
}
