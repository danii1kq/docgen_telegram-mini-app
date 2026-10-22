import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { api } from '@/api/client';
import type { Review } from '@/api/types';
import { BackButton } from '@/telegram/hooks';
import { haptic } from '@/telegram/sdk';
import { useAsync } from '@/lib/hooks';
import { formatDateTime } from '@/lib/format';
import { AppHeader, Card, EmptyState, FilterChip, Hairline, Icon, Screen, Skeleton } from '@/components/ui';
import { useToast } from '@/components/Toast';

const TABS: { value: '' | 'pending' | 'published' | 'rejected'; label: string }[] = [
  { value: 'pending', label: 'На модерации' },
  { value: 'published', label: 'Опубликованные' },
  { value: 'rejected', label: 'Отклонённые' },
  { value: '', label: 'Все' },
];

/** Operator panel · review moderation. */
export function OperatorReviewsScreen() {
  const navigate = useNavigate();
  const toast = useToast();
  const [status, setStatus] = useState<'' | 'pending' | 'published' | 'rejected'>('pending');
  const [busy, setBusy] = useState('');

  const { data, error, loading, reload } = useAsync(() => api.adminReviews(status), [status]);
  const reviews = data?.reviews ?? [];
  const counts = data?.counts ?? {};

  const moderate = async (review: Review, next: 'published' | 'rejected' | 'pending') => {
    setBusy(review.id);
    try {
      await api.adminModerateReview(review.id, next);
      haptic.success();
      toast.show(
        next === 'published' ? 'Отзыв опубликован' : next === 'rejected' ? 'Отзыв отклонён' : 'Отзыв возвращён на модерацию',
      );
      reload();
    } catch (cause) {
      haptic.error();
      toast.show(cause instanceof Error ? cause.message : 'Не удалось изменить отзыв', 'error');
    } finally {
      setBusy('');
    }
  };

  return (
    <Screen header={<AppHeader title="Отзывы" onBack={() => navigate('/operator')} />} bottomSpace>
      <div className="flex flex-col gap-3 pt-2">
        <div className="flex flex-wrap gap-2">
          {TABS.map((tab) => (
            <FilterChip key={tab.value} active={status === tab.value} onClick={() => setStatus(tab.value)}>
              {tab.label}
              {tab.value && counts[tab.value] ? ` · ${counts[tab.value]}` : ''}
            </FilterChip>
          ))}
        </div>

        {loading && !data ? (
          <div className="flex flex-col gap-2">
            <Skeleton className="h-28 w-full" />
            <Skeleton className="h-28 w-full" />
          </div>
        ) : error ? (
          <Card className="items-center gap-2 p-6">
            <Icon name="error" size={26} className="text-danger" />
            <span className="text-body-secondary text-ink-2">{error}</span>
            <button type="button" onClick={reload} className="text-body-secondary-strong text-accent">
              Повторить
            </button>
          </Card>
        ) : reviews.length === 0 ? (
          <Card>
            <EmptyState icon="reviews" title="Отзывов нет" text="Здесь появятся отзывы клиентов после выполнения заказов." />
          </Card>
        ) : (
          reviews.map((review) => (
            <Card key={review.id} className="gap-2 p-4">
              <div className="flex items-center justify-between gap-2">
                <span className="flex items-center gap-0.5">
                  {[1, 2, 3, 4, 5].map((star) => (
                    <Icon
                      key={star}
                      name="star"
                      size={15}
                      filled={star <= Math.round(review.rating)}
                      className={star <= Math.round(review.rating) ? 'text-warning' : 'text-outline-variant'}
                    />
                  ))}
                </span>
                <span className="text-caption text-ink-2">{formatDateTime(review.createdAt)}</span>
              </div>
              <p className="text-body-secondary text-ink">{review.text}</p>
              <span className="text-caption text-ink-2">
                {review.author} · {review.institute} · заказ {review.orderNumber}
              </span>
              <Hairline />
              <div className="flex gap-2">
                {review.status !== 'published' ? (
                  <button
                    type="button"
                    disabled={busy === review.id}
                    onClick={() => moderate(review, 'published')}
                    className="flex h-10 flex-1 items-center justify-center gap-1.5 rounded-xl bg-success-soft text-body-secondary-strong text-success disabled:opacity-50"
                  >
                    <Icon name="check" size={18} />
                    Опубликовать
                  </button>
                ) : null}
                {review.status !== 'rejected' ? (
                  <button
                    type="button"
                    disabled={busy === review.id}
                    onClick={() => moderate(review, 'rejected')}
                    className="flex h-10 flex-1 items-center justify-center gap-1.5 rounded-xl bg-danger-soft text-body-secondary-strong text-danger disabled:opacity-50"
                  >
                    <Icon name="close" size={18} />
                    Отклонить
                  </button>
                ) : (
                  <button
                    type="button"
                    disabled={busy === review.id}
                    onClick={() => moderate(review, 'pending')}
                    className="flex h-10 flex-1 items-center justify-center gap-1.5 rounded-xl bg-neutral-soft text-body-secondary-strong text-ink-2 disabled:opacity-50"
                  >
                    <Icon name="undo" size={18} />
                    Вернуть на модерацию
                  </button>
                )}
              </div>
            </Card>
          ))
        )}
      </div>
      <BackButton onClick={() => navigate('/operator')} />
    </Screen>
  );
}
