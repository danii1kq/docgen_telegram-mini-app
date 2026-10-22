import { useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { api } from '@/api/client';
import type { Order } from '@/api/types';
import { BackButton, MainButton } from '@/telegram/hooks';
import { useAsync, useOrderStart } from '@/lib/hooks';
import { ARCHIVE_STATUSES, DONE_STATUSES, isActive, PRODUCTS_FALLBACK } from '@/lib/catalog';
import { formatDay, money, monthLabel, unitsLabel } from '@/lib/format';
import { useI18n, useT } from '@/i18n';
import { productName } from '@/i18n/catalog';
import { AppHeader, Card, EmptyState, FilterChip, Hairline, Icon, Screen, Skeleton, StatusChip } from '@/components/ui';

type Filter = 'all' | 'active' | 'done' | 'archive';

/** S09 · My orders - everything this chat has ordered, grouped by month. */
export function MyOrdersScreen() {
  const navigate = useNavigate();
  const t = useT();
  const startOrder = useOrderStart();
  const { data, error, loading, reload } = useAsync(() => api.myOrders(), []);
  const [filter, setFilter] = useState<Filter>('all');

  const orders = data?.orders ?? [];
  const archiveNeeded = orders.length > 20;

  const filtered = useMemo(() => {
    switch (filter) {
      case 'active':
        return orders.filter((order) => isActive(order.status));
      case 'done':
        return orders.filter((order) => DONE_STATUSES.includes(order.status));
      case 'archive':
        return orders.filter((order) => ARCHIVE_STATUSES.includes(order.status));
      default:
        return orders;
    }
  }, [orders, filter]);

  const groups = useMemo(() => {
    const map = new Map<string, Order[]>();
    for (const order of filtered) {
      const key = monthLabel(order.createdAt);
      const list = map.get(key) ?? [];
      list.push(order);
      map.set(key, list);
    }
    return Array.from(map.entries());
  }, [filtered]);

  return (
    <Screen header={<AppHeader title={t('Мои заказы')} onBack={() => navigate('/')} />} bottomSpace>
      <div className="flex flex-col gap-3 pt-2">
        <h2 className="px-1 text-headline-lg text-ink">{t('Мои заказы')}</h2>

        <div className="flex gap-2 overflow-x-auto px-1 pb-1">
          <FilterChip active={filter === 'all'} onClick={() => setFilter('all')}>
            {t('Все')}
          </FilterChip>
          <FilterChip active={filter === 'active'} onClick={() => setFilter('active')}>
            {t('Активные')}
          </FilterChip>
          <FilterChip active={filter === 'done'} onClick={() => setFilter('done')}>
            {t('Готовые')}
          </FilterChip>
          {archiveNeeded ? (
            <FilterChip active={filter === 'archive'} onClick={() => setFilter('archive')}>
              {t('Архив')}
            </FilterChip>
          ) : null}
        </div>

        {loading && !data ? (
          <div className="flex flex-col gap-3">
            <Skeleton className="h-20 w-full" />
            <Skeleton className="h-20 w-full" />
            <Skeleton className="h-20 w-full" />
          </div>
        ) : error ? (
          <Card className="items-center gap-2 p-6">
            <Icon name="error" size={28} className="text-danger" />
            <span className="text-body-secondary text-ink-2">{t('Не удалось загрузить заказы')}</span>
            <button type="button" onClick={reload} className="text-body-secondary-strong text-accent">
              {t('Повторить')}
            </button>
          </Card>
        ) : filtered.length === 0 ? (
          <Card>
            <EmptyState
              icon="receipt_long"
              title={filter === 'all' ? t('Пока нет заказов') : t('Здесь пока пусто')}
              text={filter === 'all' ? t('Оформите первую работу - это займёт около трёх минут.') : t('Попробуйте другой фильтр.')}
            />
          </Card>
        ) : (
          groups.map(([month, list]) => (
            <div key={month} className="flex flex-col">
              <span className="px-1 pb-1.5 text-section-label uppercase text-ink-2">{month}</span>
              <Card>
                {list.map((order, index) => (
                  <div key={order.number}>
                    {index > 0 ? <Hairline /> : null}
                    <OrderRow order={order} onClick={() => navigate(`/order/${order.number}`)} />
                  </div>
                ))}
              </Card>
            </div>
          ))
        )}
      </div>

      <MainButton text={t('Создать заказ')} onClick={() => void startOrder()} />
      <BackButton onClick={() => navigate('/')} />
    </Screen>
  );
}

function OrderRow({ order, onClick }: { order: Order; onClick: () => void }) {
  const { lang } = useI18n();
  const product = PRODUCTS_FALLBACK.find((item) => item.code === order.product) ?? PRODUCTS_FALLBACK[0];
  return (
    <button type="button" onClick={onClick} className="flex items-center gap-3 px-4 py-3 text-left transition active:bg-neutral-soft/60">
      <span className="flex size-9 shrink-0 items-center justify-center rounded-full bg-accent-soft text-accent">
        <Icon name={isActive(order.status) ? 'autorenew' : 'done_all'} size={20} />
      </span>
      <span className="flex min-w-0 flex-1 flex-col gap-1">
        <span className="flex items-center justify-between gap-2">
          <span className="font-mono text-[15px] font-medium text-ink">{order.number}</span>
          <StatusChip status={order.status} size="sm" />
        </span>
        <span className="truncate text-body-secondary text-ink-2">
          {productName(lang, product.code, product.name)} · {order.units > 1 ? `${unitsLabel(order.units, product)} · ` : ''}
          {money(order.amount)}
        </span>
      </span>
      <span className="flex shrink-0 flex-col items-end">
        <span className="text-caption text-ink-2">{formatDay(order.createdAt)}</span>
        <Icon name="chevron_right" size={20} className="text-outline-variant" />
      </span>
    </button>
  );
}
