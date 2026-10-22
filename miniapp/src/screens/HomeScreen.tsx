import { useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import { api } from '@/api/client';
import type { Order } from '@/api/types';
import { MainButton } from '@/telegram/hooks';
import { telegramUser } from '@/telegram/sdk';
import { useAsync, useOrderStart, useSettings } from '@/lib/hooks';
import { useOperator } from '@/app/session';
import { PRODUCTS_FALLBACK } from '@/lib/catalog';
import { money, worksLabel } from '@/lib/format';
import { APP_VERSION } from '@/lib/app';
import { useI18n, useT } from '@/i18n';
import { productName } from '@/i18n/catalog';
import {
  AppHeader,
  Avatar,
  Banner,
  Card,
  EmptyState,
  Hairline,
  Icon,
  Row,
  Screen,
  SectionLabel,
  Skeleton,
  StatusChip,
} from '@/components/ui';

/**
 * S02 · Home - the hub: who you are, what is happening with your orders and the
 * way into the wizard. The price calculator lives in the wizard itself (step 1),
 * so this screen stays a simple list.
 */
export function HomeScreen() {
  const navigate = useNavigate();
  const user = telegramUser();
  const operator = useOperator();
  const t = useT();

  const orders = useAsync(() => api.myOrders(), []);
  const reviews = useAsync(() => api.reviews(100), []);
  const settings = useSettings();

  // Real aggregate from the published reviews - never a hardcoded value.
  const reviewAverage = useMemo(() => {
    const list = reviews.data ?? [];
    if (list.length === 0) return undefined;
    return list.reduce((sum, review) => sum + review.rating, 0) / list.length;
  }, [reviews.data]);

  const allOrders = orders.data?.orders ?? [];
  const shown = allOrders.slice(0, 3);
  const maintenance = settings?.maintenance ?? false;
  const startOrder = useOrderStart();

  return (
    <Screen
      header={
        <AppHeader
          title={t('Главная')}
          leadingIcon="school"
          trailing={
            <button
              type="button"
              aria-label={t('Профиль')}
              onClick={() => navigate('/profile')}
              className="flex size-9 items-center justify-center rounded-full transition active:opacity-70"
            >
              <Avatar photoUrl={user?.photo_url} name={user?.first_name} />
            </button>
          }
        />
      }
      bottomSpace
    >
      <div className="flex flex-col gap-4 pt-2">
        {/* Greeting */}
        <div className="flex items-start justify-between gap-3 px-1 pt-1">
          <div className="flex min-w-0 flex-col">
            <h2 className="truncate text-headline-lg text-ink">
              {t('Привет, {name} 👋', { name: user?.first_name ?? t('студент') })}
            </h2>
            <p className="mt-0.5 text-body-secondary text-ink-2">{t('Работы по стандартам Satbayev University')}</p>
          </div>
          <span className="flex shrink-0 items-center gap-1 rounded-full border border-accent/20 bg-accent-soft px-2.5 py-1 text-[12px] font-semibold text-accent">
            <Icon name="verified" size={15} filled />
            SruNal KZ
          </span>
        </div>

        {maintenance ? (
          <Banner tone="warning" icon="campaign" title={t('Приём заказов временно приостановлен')}>
            {t('Загляните позже или напишите в поддержку - вернёмся к работе совсем скоро.')}
          </Banner>
        ) : null}

        {/* Orders */}
        <div className="flex flex-col">
          <SectionLabel
            action={
              <button
                type="button"
                onClick={() => navigate('/orders')}
                className="text-body-secondary text-accent transition active:opacity-60"
              >
                {t('Все ›')}
              </button>
            }
          >
            {t('Последние заказы')}
          </SectionLabel>
          {orders.loading && !orders.data ? (
            <Card className="gap-2 p-4">
              <Skeleton className="h-5 w-32" />
              <Skeleton className="h-4 w-48" />
            </Card>
          ) : allOrders.length === 0 ? (
            <Card>
              <EmptyState
                icon="receipt_long"
                title={t('Пока нет заказов')}
                text={t('Оформите первую работу - это займёт около трёх минут.')}
              />
            </Card>
          ) : (
            <Card>
              {shown.map((order, index) => (
                <div key={order.number}>
                  {index > 0 ? <Hairline /> : null}
                  <OrderRow order={order} onClick={() => navigate(`/order/${order.number}`)} />
                </div>
              ))}
            </Card>
          )}
        </div>

        {/* More */}
        <div className="flex flex-col">
          <SectionLabel>{t('Ещё')}</SectionLabel>
          <Card>
            {operator ? (
              <>
                <Row
                  icon="admin_panel_settings"
                  title={t('Панель оператора')}
                  subtitle={operator.name}
                  onClick={() => navigate('/operator')}
                />
                <Hairline />
              </>
            ) : null}
            <Row
              icon="receipt_long"
              title={t('Мои заказы')}
              value={allOrders.length > 0 ? String(allOrders.length) : undefined}
              onClick={() => navigate('/orders')}
            />
            <Hairline />
            <Row
              icon="bookmark"
              title={t('Мои шаблоны')}
              subtitle={t('Титульные листы для быстрого оформления')}
              onClick={() => navigate('/templates')}
            />
            <Hairline />
            <Row
              icon="star"
              title={t('Отзывы')}
              value={
                reviewAverage !== undefined
                  ? `${reviewAverage.toFixed(1).replace('.', ',')} · ${reviews.data?.length}`
                  : undefined
              }
              onClick={() => navigate('/reviews')}
            />
            <Hairline />
            <Row icon="support_agent" title={t('Поддержка')} value={t('онлайн')} onClick={() => navigate('/support')} />
            <Hairline />
            <Row icon="description" title={t('Документы')} onClick={() => navigate('/legal/offer')} />
            <Hairline />
            <Row icon="info" title={t('О сервисе')} value={`v${APP_VERSION}`} onClick={() => navigate('/profile')} />
          </Card>
        </div>
      </div>

      <MainButton text={t('Заказать')} disabled={maintenance} onClick={() => void startOrder()} />
    </Screen>
  );
}

function OrderRow({ order, onClick }: { order: Order; onClick: () => void }) {
  const { lang } = useI18n();
  const product = PRODUCTS_FALLBACK.find((item) => item.code === order.product) ?? PRODUCTS_FALLBACK[0];
  return (
    <button
      type="button"
      onClick={onClick}
      className="flex w-full flex-col gap-1 px-4 py-3 text-left transition active:bg-neutral-soft/60"
    >
      <div className="flex items-center justify-between gap-2">
        <span className="flex min-w-0 items-center gap-2">
          <Icon name="receipt_long" size={18} className="shrink-0 text-ink-2" />
          <span className="truncate font-mono text-[15px] font-medium tracking-wide text-ink">{order.number}</span>
        </span>
        <StatusChip status={order.status} size="sm" />
      </div>
      <div className="flex items-center justify-between gap-2 pt-0.5">
        <span className="truncate text-body-secondary text-ink-2">
          {productName(lang, order.product, product.name)} · {worksLabel(order.works)} · {money(order.amount)}
        </span>
        <Icon name="chevron_right" size={20} className="shrink-0 text-outline-variant" />
      </div>
    </button>
  );
}
