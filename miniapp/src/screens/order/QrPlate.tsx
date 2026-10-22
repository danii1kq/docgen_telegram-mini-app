import { useEffect, useState } from 'react';
import { haptic, openLink } from '@/telegram/sdk';
import { money } from '@/lib/format';
import { Card, Icon, Skeleton } from '@/components/ui';
import { useT } from '@/i18n';

/**
 * S07 · Payment block: two ways to pay.
 *
 *  1. the red «Оплатить через Kaspi» button - the operator's qr.kaspi.kz link,
 *     which hands the payment off to the Kaspi app on a phone;
 *  2. the QR itself - the uploaded Kaspi QR image, which is the only option on a
 *     desktop (where the deep link cannot open the app).
 *
 * Recipient and amount always stay real text next to the QR (design.md §11:
 * never baked into an image).
 */
export function PaymentBlock({
  amount,
  recipient,
  link,
  qrUrl,
}: {
  amount: number;
  recipient?: string;
  link?: string;
  qrUrl?: string;
}) {
  const t = useT();
  const payLink = (link ?? '').trim();
  const qrSrc = (qrUrl ?? '').trim() || '/api/settings/payment-qr';
  const [qrReady, setQrReady] = useState<boolean | undefined>(undefined);

  useEffect(() => {
    let alive = true;
    fetch(qrSrc, { method: 'HEAD' })
      .then((response) => {
        if (alive) setQrReady(response.ok);
      })
      .catch(() => {
        if (alive) setQrReady(false);
      });
    return () => {
      alive = false;
    };
  }, [qrSrc]);

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-col px-1">
        <span className="text-headline-md text-ink">{t('Оплата')}</span>
        <span className="pt-0.5 text-body-secondary text-ink-2">
          {money(amount)}
          {recipient ? ` · ${t('получатель {name}', { name: recipient })}` : ''}
        </span>
      </div>

      {payLink ? (
        <>
          <button
            type="button"
            onClick={() => {
              haptic.impact('medium');
              openLink(payLink);
            }}
            className="flex h-12 w-full items-center justify-center gap-2 rounded-xl bg-kaspi text-button text-white shadow-[0_4px_12px_rgba(241,70,53,0.28)] transition active:scale-[0.98]"
          >
            <Icon name="qr_code_2" size={22} />
            {t('Оплатить через Kaspi')}
          </button>
          <span className="px-1 text-caption text-ink-2">
            {t('Кнопка открывает приложение Kaspi. Если она не сработала (например, на компьютере) - отсканируйте QR ниже в приложении Kaspi.')}
          </span>
        </>
      ) : null}

      <Card className="items-center gap-2 px-4 py-4">
        <span className="text-caption text-ink-2">
          {payLink ? t('Или отсканируйте QR в приложении Kaspi') : t('Отсканируйте QR в приложении Kaspi')}
        </span>
        <div className="flex size-[188px] items-center justify-center overflow-hidden rounded-2xl border border-line bg-qr-plate">
          {qrReady === undefined ? (
            <Skeleton className="size-40" />
          ) : qrReady ? (
            <img src={qrSrc} alt="Kaspi QR" className="size-full object-contain p-2" loading="lazy" />
          ) : (
            <PlaceholderPlate />
          )}
        </div>
        <span className="text-caption text-ink-2">{t('Сумма к оплате: {amount}', { amount: money(amount) })}</span>
      </Card>

      <div className="flex flex-col gap-1 px-1">
        <Step index={1} text={t('Откройте Kaspi.kz - Платежи - по QR')} />
        <Step
          index={2}
          text={`${t('Проверьте сумму {amount}', { amount: money(amount) })}${recipient ? ` ${t('и получателя {name}', { name: recipient })}` : ''}`}
        />
        <Step index={3} text={t('Вернитесь сюда и нажмите «Я оплатил»')} />
      </div>
    </div>
  );
}

function Step({ index, text }: { index: number; text: string }) {
  return (
    <div className="flex items-start gap-2">
      <span className="flex size-5 shrink-0 items-center justify-center rounded-full bg-accent-soft text-step-label font-semibold text-accent">
        {index}
      </span>
      <span className="text-body-secondary text-ink-2">{text}</span>
    </div>
  );
}

/** Vector fallback plate (the Stitch mock's QR illustration). */
function PlaceholderPlate() {
  const rects: [number, number, number, number][] = [
    [10, 10, 35, 35],
    [95, 10, 35, 35],
    [10, 95, 35, 35],
  ];
  const dots: [number, number, number, number][] = [
    [22, 22, 11, 11],
    [107, 22, 11, 11],
    [22, 107, 11, 11],
    [52, 15, 6, 6],
    [64, 15, 6, 6],
    [76, 22, 6, 6],
    [52, 32, 6, 6],
    [68, 32, 12, 6],
    [15, 52, 6, 6],
    [27, 52, 6, 6],
    [42, 52, 6, 6],
    [58, 48, 6, 14],
    [74, 52, 8, 6],
    [92, 52, 6, 6],
    [115, 52, 12, 6],
    [15, 68, 8, 6],
    [35, 68, 18, 6],
    [62, 68, 16, 6],
    [88, 68, 12, 6],
    [110, 68, 8, 6],
    [15, 80, 6, 6],
    [45, 80, 8, 6],
    [65, 80, 8, 8],
    [85, 80, 6, 6],
    [105, 80, 16, 6],
    [52, 98, 6, 6],
    [64, 98, 14, 6],
    [88, 98, 8, 6],
    [106, 98, 6, 14],
    [120, 98, 8, 6],
    [52, 112, 14, 6],
    [76, 112, 6, 14],
    [92, 118, 12, 6],
    [115, 118, 12, 10],
  ];
  return (
    <svg viewBox="0 0 140 140" className="size-full p-3 text-ink" fill="currentColor" aria-label="QR">
      {rects.map(([x, y, w, h], index) => (
        <rect key={`r${index}`} x={x} y={y} width={w} height={h} fill="none" stroke="currentColor" strokeWidth={7} />
      ))}
      {dots.map(([x, y, w, h], index) => (
        <rect key={`d${index}`} x={x} y={y} width={w} height={h} />
      ))}
    </svg>
  );
}
