import { useNavigate } from 'react-router-dom';
import { api } from '@/api/client';
import { BackButton } from '@/telegram/hooks';
import { useAsync } from '@/lib/hooks';
import { money } from '@/lib/format';
import { useOperator } from '@/app/session';
import { AppHeader, Card, Hairline, Icon, Row, Screen, SectionLabel } from '@/components/ui';

/** Operator panel · hub: queue, content, money and service settings. */
export function OperatorHubScreen() {
  const navigate = useNavigate();
  const operator = useOperator();
  const stats = useAsync(() => api.adminStats(), []);

  const claimed = stats.data?.ordersByStatus['payment_claimed'] ?? 0;
  const queue = stats.data?.queueDepth ?? 0;

  return (
    <Screen header={<AppHeader title="Панель оператора" onBack={() => navigate('/')} />}>
      <div className="flex flex-col gap-3 pt-2">
        <Card className="p-4">
          <div className="flex items-center gap-3">
            <span className="flex size-10 items-center justify-center rounded-full bg-accent-soft text-accent">
              <Icon name="admin_panel_settings" size={22} />
            </span>
            <div className="flex min-w-0 flex-col">
              <span className="truncate text-headline-sm text-ink">{operator?.name ?? 'Оператор'}</span>
              <span className="truncate text-caption text-accent">{operator?.id ?? ''}</span>
            </div>
          </div>
          {stats.data ? (
            <div className="flex gap-4 pt-3">
              <div className="flex flex-col">
                <span className="text-headline-md tabular-nums text-warning">{claimed}</span>
                <span className="text-caption text-ink-2">на проверке</span>
              </div>
              <div className="flex flex-col">
                <span className="text-headline-md tabular-nums text-accent">{queue}</span>
                <span className="text-caption text-ink-2">в очереди</span>
              </div>
              <div className="flex flex-col">
                <span className="text-headline-md tabular-nums text-success">{money(stats.data.revenueToday)}</span>
                <span className="text-caption text-ink-2">сегодня</span>
              </div>
            </div>
          ) : null}
        </Card>

        {stats.data?.queueWarning ? (
          <div className="flex items-start gap-2 rounded-[14px] bg-warning-soft px-4 py-3 text-warning">
            <Icon name="warning" size={20} filled />
            <span className="text-caption">Очередь ждёт дольше обычного - проверьте заказы в работе.</span>
          </div>
        ) : null}

        <SectionLabel>Работа</SectionLabel>
        <Card>
          <Row
            icon="receipt_long"
            title="Заказы"
            subtitle="Реестр, проверка оплат, генерация"
            value={claimed > 0 ? <span className="text-warning">{claimed}</span> : undefined}
            onClick={() => navigate('/operator/orders')}
          />
          <Hairline />
          <Row
            icon="reviews"
            title="Отзывы"
            subtitle="Модерация"
            value={stats.data?.pendingReviews ? <span className="text-warning">{stats.data.pendingReviews}</span> : undefined}
            onClick={() => navigate('/operator/reviews')}
          />
        </Card>

        <SectionLabel>Деньги</SectionLabel>
        <Card>
          <Row icon="sell" title="Промокоды" subtitle="Скидки и лимиты" onClick={() => navigate('/operator/promos')} />
          <Hairline />
          <Row icon="payments" title="Тарифы" subtitle="Цены и скидки за объём" onClick={() => navigate('/operator/pricing')} />
          <Hairline />
          <Row icon="insights" title="Статистика" subtitle="Выручка, очередь, сбои" onClick={() => navigate('/operator/stats')} />
        </Card>

        <SectionLabel>Сервис</SectionLabel>
        <Card>
          <Row
            icon="settings"
            title="Настройки"
            subtitle="Kaspi QR, поддержка, режим работы"
            onClick={() => navigate('/operator/settings')}
          />
        </Card>

        <span className="px-1 pt-2 text-caption text-ink-2">
          Панель работает прямо в Telegram: доступ проверяется по привязанному чату оператора.
        </span>
      </div>
      <BackButton onClick={() => navigate('/')} />
    </Screen>
  );
}
