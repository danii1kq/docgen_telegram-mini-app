import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { BackButton } from '@/telegram/hooks';
import { haptic, openTelegramLink } from '@/telegram/sdk';
import { APP_VERSION, BOT_USERNAME, BOT_URL } from '@/lib/app';
import { SUPPORT_HANDLE } from '@/lib/catalog';
import { useSettings } from '@/lib/hooks';
import { useT } from '@/i18n';
import { AppHeader, Card, Hairline, Icon, Screen, SectionLabel } from '@/components/ui';

/** Built inside the component so every question goes through the translator. */
function faqItems(t: (source: string) => string): { question: string; answer: string }[] {
  return [
    {
      question: t('Как оплатить заказ?'),
      answer: t('Откройте заказ, отсканируйте Kaspi QR, переведите сумму и нажмите «Я оплатил», указав имя отправителя как в Kaspi.'),
    },
    {
      question: t('Сколько ждать готовый файл?'),
      answer: t('Обычно от 3 до 20 минут после подтверждения оплаты оператором. Файл придёт в этот чат.'),
    },
    {
      question: t('Оплата не подтвердилась'),
      answer: t('Проверьте имя отправителя - оно должно совпадать с Kaspi (например, «Ануар А.»). Затем нажмите «Я оплатил ещё раз» или напишите в поддержку.'),
    },
    {
      question: t('Нужны правки'),
      answer: t('Замечания преподавателя вносим бесплатно - напишите в поддержку и приложите комментарии.'),
    },
    {
      question: t('Как изменить тему после оплаты?'),
      answer: t('Напишите в поддержку до начала генерации - оператор обновит данные заказа.'),
    },
    {
      question: t('Как получить чек?'),
      answer: t('Напишите в поддержку, укажите номер заказа - пришлём чек.'),
    },
  ];
}

/** S12 · Support: reach a human + the top questions + documents. */
export function SupportScreen() {
  const navigate = useNavigate();
  const t = useT();
  const settings = useSettings();
  const supportHandle = settings?.supportTelegram ?? SUPPORT_HANDLE;
  const [open, setOpen] = useState<number | null>(0);
  const faq = faqItems(t);

  const openSupport = () => {
    haptic.tap();
    openTelegramLink(`https://t.me/${supportHandle.replace('@', '')}`);
  };

  return (
    <Screen header={<AppHeader title={t('Поддержка')} onBack={() => navigate(-1)} />}>
      <div className="flex flex-col gap-3 pt-2">
        <Card>
          <button
            type="button"
            onClick={openSupport}
            className="flex items-center gap-3 px-4 py-3 text-left transition active:bg-neutral-soft/60"
          >
            <span className="flex size-10 shrink-0 items-center justify-center rounded-full bg-accent-soft text-accent">
              <Icon name="support_agent" size={22} />
            </span>
            <span className="flex min-w-0 flex-1 flex-col">
              <span className="text-body-strong text-ink">{t('Написать оператору')}</span>
              <span className="text-body-secondary text-ink-2">{t('Обычно отвечаем в течение часа')}</span>
            </span>
            <span className="flex items-center gap-1">
              <span className="rounded-full bg-success-soft px-2 py-0.5 text-step-label text-success">{t('онлайн')}</span>
              <Icon name="chevron_right" size={20} className="text-outline-variant" />
            </span>
          </button>
        </Card>

        <SectionLabel>{t('Частые вопросы')}</SectionLabel>
        <Card>
          {faq.map((item, index) => (
            <div key={item.question}>
              {index > 0 ? <Hairline /> : null}
              <button
                type="button"
                onClick={() => {
                  haptic.tap();
                  setOpen(open === index ? null : index);
                }}
                className="flex w-full items-center gap-2 px-4 py-3 text-left transition active:bg-neutral-soft/60"
              >
                <span className="flex-1 text-body-secondary-strong text-ink">{item.question}</span>
                <Icon name={open === index ? 'expand_less' : 'expand_more'} size={22} className="text-ink-2" />
              </button>
              {open === index ? (
                <p className="fade-in px-4 pb-3 text-body-secondary text-ink-2">{item.answer}</p>
              ) : null}
            </div>
          ))}
        </Card>

        <SectionLabel>{t('Документы')}</SectionLabel>
        <Card>
          <Row label={t('Публичная оферта')} onClick={() => navigate('/legal/offer')} />
          <Hairline />
          <Row label={t('Обработка персональных данных')} onClick={() => navigate('/legal/personal')} />
          <Hairline />
          <Row label={t('Политика конфиденциальности')} onClick={() => navigate('/legal/privacy')} />
        </Card>

        <div className="flex flex-col items-center gap-1 pt-4 text-center">
          <span className="text-caption text-ink-2">
            {t('SruNal KZ v{version} · бот @{bot}', { version: APP_VERSION, bot: BOT_USERNAME })}
          </span>
          <button
            type="button"
            onClick={() => {
              haptic.tap();
              openTelegramLink(BOT_URL);
            }}
            className="text-caption text-accent"
          >
            {supportHandle}
          </button>
        </div>
      </div>
      <BackButton onClick={() => navigate(-1)} />
    </Screen>
  );
}

function Row({ label, onClick }: { label: string; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="flex min-h-[52px] w-full items-center gap-2 px-4 text-left transition active:bg-neutral-soft/60"
    >
      <span className="flex-1 text-body-secondary text-ink">{label}</span>
      <Icon name="chevron_right" size={20} className="text-outline-variant" />
    </button>
  );
}
