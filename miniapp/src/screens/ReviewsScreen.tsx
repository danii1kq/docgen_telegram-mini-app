import { useMemo, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { api, ApiError } from '@/api/client';
import { BackButton, MainButton } from '@/telegram/hooks';
import { haptic, telegramUser } from '@/telegram/sdk';
import { useAsync } from '@/lib/hooks';
import { PRODUCTS_FALLBACK } from '@/lib/catalog';
import { formatDay } from '@/lib/format';
import { useI18n } from '@/i18n';
import { productName } from '@/i18n/catalog';
import { AppHeader, Card, EmptyState, FieldError, Icon, Screen, Skeleton } from '@/components/ui';
import { useFeedback } from '@/components/Toast';

/** S10/S11 · Reviews: the moderated feed plus the form for a real order. */
export function ReviewsScreen() {
  const navigate = useNavigate();
  const { t, n, lang } = useI18n();
  const [search] = useSearchParams();
  const orderNumber = search.get('order');
  const user = telegramUser();
  const feedback = useFeedback();

  const reviews = useAsync(() => api.reviews(), []);
  const order = useAsync(
    () => (orderNumber ? api.order(orderNumber) : Promise.resolve(undefined)),
    [orderNumber],
  );
  const [rating, setRating] = useState(0);
  const [text, setText] = useState('');
  const [publish, setPublish] = useState(true);
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState(false);
  const [errors, setErrors] = useState<Record<string, string>>({});

  const average = useMemo(() => {
    const list = reviews.data ?? [];
    if (list.length === 0) return undefined;
    const sum = list.reduce((acc, review) => acc + review.rating, 0);
    return sum / list.length;
  }, [reviews.data]);

  const submit = async () => {
    if (!orderNumber || rating < 1 || text.trim().length < 10) return;
    setBusy(true);
    setErrors({});
    try {
      await api.addReview({
        orderNumber,
        author: publish ? (user?.first_name ?? t('Клиент SruNal')) : t('Клиент SruNal'),
        institute: order.data?.institute || 'Satbayev Univ',
        department: order.data?.department || '-',
        discipline: order.data?.disciplineCode || order.data?.department || '-',
        product: order.data?.product ?? 'essay',
        rating,
        text: text.trim(),
      });
      haptic.success();
      feedback(t('Спасибо! Отзыв отправлен на проверку'), 'success');
      setDone(true);
    } catch (cause) {
      haptic.error();
      if (cause instanceof ApiError && cause.fields) {
        setErrors(cause.fields);
      }
      feedback(cause instanceof Error ? t(cause.message) : t('Не удалось отправить отзыв'), 'error');
    } finally {
      setBusy(false);
    }
  };

  const showForm = Boolean(orderNumber) && !done;

  return (
    <Screen header={<AppHeader title={showForm ? t('Отзыв о работе') : t('Отзывы')} onBack={() => navigate(-1)} />} bottomSpace>
      <div className="flex flex-col gap-3 pt-2">
        {showForm ? (
          <>
            <span className="px-1 text-headline-lg text-ink">{t('Отзыв о работе')}</span>
            <Card className="p-4">
              <span className="text-body-secondary text-ink-2">{t('Заказ {number}', { number: orderNumber ?? '' })}</span>
              {order.data ? (
                <span className="text-caption text-ink-2">
                  {productName(lang, order.data.product, PRODUCTS_FALLBACK.find((item) => item.code === order.data?.product)?.name ?? '')} ·{' '}
                  {formatDay(order.data.createdAt)}
                </span>
              ) : null}
            </Card>
            <Card className="items-center gap-2 p-4">
              <span className="text-caption text-ink-2">{t('Оценка')}</span>
              <div className="flex gap-1">
                {[1, 2, 3, 4, 5].map((star) => (
                  <button
                    key={star}
                    type="button"
                    aria-label={t('Оценка {n}', { n: star })}
                    onClick={() => {
                      haptic.select();
                      setRating(star);
                    }}
                    className="flex size-11 items-center justify-center"
                  >
                    <Icon
                      name="star"
                      size={32}
                      filled={star <= rating}
                      className={star <= rating ? 'text-warning' : 'text-outline-variant'}
                    />
                  </button>
                ))}
              </div>
              <FieldError>{t(errors.rating)}</FieldError>
            </Card>
            <Card className="p-4">
              <textarea
                value={text}
                rows={5}
                maxLength={1000}
                onChange={(event) => setText(event.target.value)}
                placeholder={t('Что понравилось, что улучшить')}
                className="w-full resize-none border-0 bg-transparent p-0 text-body-lg text-ink outline-none placeholder:text-hint"
              />
              <span className="self-end text-caption text-ink-2">{text.length} / 1000</span>
            </Card>
            <FieldError>{t(errors.text)}</FieldError>
            <button
              type="button"
              onClick={() => setPublish((value) => !value)}
              className="flex items-center gap-2 px-1 text-left"
            >
              <span
                className={`flex size-5 items-center justify-center rounded ${publish ? 'bg-accent text-btn-text' : 'border border-line bg-surface'}`}
              >
                {publish ? <Icon name="check" size={16} /> : null}
              </span>
              <span className="text-caption text-ink">
                {t('Публиковать от имени {name}', { name: user?.first_name ?? t('Клиент SruNal') })}
              </span>
            </button>
            <span className="px-1 text-caption text-ink-2">{t('Отзыв появится в списке после проверки оператором.')}</span>
          </>
        ) : done ? (
          <Card>
            <EmptyState icon="check_circle" title={t('Спасибо!')} text={t('Отзыв отправлен на проверку и скоро появится в списке.')} />
          </Card>
        ) : (
          <>
            {average !== undefined ? (
              <div className="flex items-end gap-2 px-1">
                <span className="text-[28px] font-semibold leading-none text-ink">
                  {average.toFixed(1).replace('.', ',')}
                </span>
                <div className="flex items-center gap-0.5 pb-1">
                  {[1, 2, 3, 4, 5].map((star) => (
                    <Icon
                      key={star}
                      name="star"
                      size={22}
                      filled={star <= Math.round(average)}
                      className={star <= Math.round(average) ? 'text-warning' : 'text-outline-variant'}
                    />
                  ))}
                </div>
                <span className="pb-1 text-caption text-ink-2">
                  {reviews.data?.length ?? 0}{' '}
                  {n(reviews.data?.length ?? 0, { ru: ['отзыв', 'отзыва', 'отзывов'], kk: 'пікір', en: ['review', 'reviews'] })}
                </span>
              </div>
            ) : null}

            {reviews.loading && !reviews.data ? (
              <div className="flex flex-col gap-3">
                <Skeleton className="h-24 w-full" />
                <Skeleton className="h-24 w-full" />
                <Skeleton className="h-24 w-full" />
              </div>
            ) : (reviews.data ?? []).length === 0 ? (
              <Card>
                <EmptyState icon="star" title={t('Отзывов пока нет')} text={t('Станьте первым - оставьте отзыв после выполнения заказа.')} />
              </Card>
            ) : (
              <div className="flex flex-col gap-3">
                {(reviews.data ?? []).map((review) => (
                  <Card key={review.id} className="gap-1 p-4">
                    <div className="flex items-center gap-0.5">
                      {[1, 2, 3, 4, 5].map((star) => (
                        <Icon
                          key={star}
                          name="star"
                          size={18}
                          filled={star <= Math.round(review.rating)}
                          className={star <= Math.round(review.rating) ? 'text-warning' : 'text-outline-variant'}
                        />
                      ))}
                    </div>
                    <p className="text-body-secondary text-ink">{review.text}</p>
                    <span className="pt-1 text-caption text-ink-2">
                      {review.author} · {review.institute} · {formatDay(review.createdAt)}
                    </span>
                  </Card>
                ))}
              </div>
            )}
          </>
        )}
      </div>

      {showForm ? (
        <MainButton
          text={t('Отправить отзыв')}
          disabled={rating < 1 || text.trim().length < 10 || busy}
          progress={busy}
          onClick={submit}
        />
      ) : null}
      <BackButton onClick={() => navigate(-1)} />
    </Screen>
  );
}
