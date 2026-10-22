import { useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { api } from '@/api/client';
import type { OrderStatus } from '@/api/types';
import { BackButton, MainButton } from '@/telegram/hooks';
import { useAsync } from '@/lib/hooks';
import { formatDay, money } from '@/lib/format';
import {
  AppHeader,
  Card,
  EmptyState,
  FilterChip,
  Hairline,
  Icon,
  Screen,
  Skeleton,
  StatusChip,
} from '@/components/ui';
import { useToast } from '@/components/Toast';

/** Filters of the registry: exact statuses the API accepts, plus "all". */
const FILTERS: { value: '' | OrderStatus; label: string }[] = [
  { value: 'payment_claimed', label: 'Проверка' },
  { value: 'queued', label: 'В очереди' },
  { value: 'generating', label: 'Генерация' },
  { value: 'completed', label: 'Готово' },
  { value: 'failed_final', label: 'Проблемы' },
  { value: 'payment_rejected', label: 'Отклонено' },
  { value: 'awaiting_payment', label: 'Ждут оплаты' },
];

/** Operator panel · order registry (`payment_claimed` pinned first). */
export function OperatorOrdersScreen() {
  const navigate = useNavigate();
  const toast = useToast();
  const [status, setStatus] = useState<'' | OrderStatus>('payment_claimed');
  const [query, setQuery] = useState('');
  const [search, setSearch] = useState('');

  const { data, error, loading, reload } = useAsync(
    () => api.adminOrders({ status: status || undefined, q: search || undefined, limit: 100 }),
    [status, search],
  );

  const orders = data?.orders ?? [];
  const counts = data?.counts ?? {};

  const kpi = useMemo(
    () => [
      { label: 'На проверке', value: counts['payment_claimed'] ?? 0, tone: 'warning' as const },
      { label: 'В работе', value: (counts['queued'] ?? 0) + (counts['generating'] ?? 0), tone: 'accent' as const },
      { label: 'Готово', value: counts['completed'] ?? 0, tone: 'success' as const },
      { label: 'Проблемы', value: counts['failed_final'] ?? 0, tone: 'danger' as const },
    ],
    [counts],
  );

  return (
    <Screen header={<AppHeader title="Заказы" onBack={() => navigate('/operator')} />} bottomSpace>
      <div className="flex flex-col gap-3 pt-2">
        <div className="grid grid-cols-4 gap-2">
          {kpi.map((item) => (
            <div key={item.label} className="card flex flex-col items-center gap-0.5 px-1 py-2">
              <span
                className={`text-headline-md tabular-nums ${
                  item.tone === 'warning'
                    ? 'text-warning'
                    : item.tone === 'success'
                      ? 'text-success'
                      : item.tone === 'danger'
                        ? 'text-danger'
                        : 'text-accent'
                }`}
              >
                {item.value}
              </span>
              <span className="text-center text-[11px] leading-tight text-ink-2">{item.label}</span>
            </div>
          ))}
        </div>

        <div className="flex items-center gap-2 rounded-[10px] bg-surface px-3 py-2">
          <Icon name="search" size={20} className="text-ink-2" />
          <input
            value={query}
            placeholder="Номер, тема, клиент"
            onChange={(event) => setQuery(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === 'Enter') setSearch(query.trim());
            }}
            className="min-w-0 flex-1 bg-transparent text-body-secondary text-ink outline-none placeholder:text-hint"
          />
          {query ? (
            <button
              type="button"
              aria-label="Найти"
              onClick={() => setSearch(query.trim())}
              className="text-caption text-accent"
            >
              Найти
            </button>
          ) : null}
          {search ? (
            <button
              type="button"
              aria-label="Сбросить"
              onClick={() => {
                setSearch('');
                setQuery('');
              }}
              className="flex size-6 items-center justify-center text-ink-2"
            >
              <Icon name="close" size={18} />
            </button>
          ) : null}
        </div>

        <div className="flex flex-wrap gap-2">
          <FilterChip active={status === ''} onClick={() => setStatus('')}>
            Все
          </FilterChip>
          {FILTERS.map((filter) => (
            <FilterChip key={filter.value} active={status === filter.value} onClick={() => setStatus(filter.value)}>
              {filter.label}
            </FilterChip>
          ))}
        </div>

        {loading && !data ? (
          <div className="flex flex-col gap-2">
            <Skeleton className="h-20 w-full" />
            <Skeleton className="h-20 w-full" />
            <Skeleton className="h-20 w-full" />
          </div>
        ) : error ? (
          <Card className="items-center gap-2 p-6">
            <Icon name="error" size={26} className="text-danger" />
            <span className="text-body-secondary text-ink-2">{error}</span>
            <button type="button" onClick={reload} className="text-body-secondary-strong text-accent">
              Повторить
            </button>
          </Card>
        ) : orders.length === 0 ? (
          <Card>
            <EmptyState icon="inbox" title="Заказов нет" text="Выберите другой фильтр или снимите поиск." />
          </Card>
        ) : (
          <Card>
            {orders.map((order, index) => (
              <div key={order.number}>
                {index > 0 ? <Hairline /> : null}
                <button
                  type="button"
                  onClick={() => navigate(`/operator/orders/${order.number}`)}
                  className="flex w-full flex-col gap-1 px-4 py-3 text-left transition active:bg-neutral-soft/60"
                >
                  <div className="flex items-center justify-between gap-2">
                    <span className="min-w-0 flex-1 truncate font-mono text-[15px] font-medium text-ink">
                      {order.number}
                    </span>
                    <span className="flex shrink-0 items-center gap-1.5">
                      {order.revisionRequestedAt ? (
                        <span className="inline-flex items-center gap-1 rounded-full bg-accent-soft px-2 py-0.5 text-[12px] font-semibold text-accent">
                          <Icon name="edit_note" size={14} />
                          Правки
                        </span>
                      ) : null}
                      <StatusChip status={order.status} size="sm" />
                    </span>
                  </div>
                  <div className="flex items-center justify-between gap-2">
                    <span className="truncate text-body-secondary text-ink-2">
                      {money(order.amount)} · {order.contactName || order.studentName || 'без имени'}
                    </span>
                    <span className="shrink-0 text-caption text-ink-2">{formatDay(order.createdAt)}</span>
                  </div>
                </button>
              </div>
            ))}
          </Card>
        )}

        <span className="px-1 text-caption text-ink-2">
          {data ? `${orders.length} из ${data.total} · обновлено ${new Date().toLocaleTimeString('ru-RU').slice(0, 5)}` : ''}
        </span>
      </div>

      <MainButton
        text="Обновить"
        onClick={() => {
          reload();
          toast.show('Список обновлён');
        }}
      />
      <BackButton onClick={() => navigate('/operator')} />
    </Screen>
  );
}
