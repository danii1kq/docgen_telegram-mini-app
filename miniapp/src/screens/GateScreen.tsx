import { haptic, openTelegramLink } from '@/telegram/sdk';
import { BOT_URL } from '@/lib/app';
import { useT } from '@/i18n';
import { Icon } from '@/components/ui';

/**
 * S01 · Gate screen: `initData` only exists inside Telegram, so a plain
 * browser preview gets an explanation instead of a broken flow.
 */
export function GateScreen() {
  const t = useT();
  return (
    <div className="flex min-h-screen flex-col items-center justify-center gap-3 px-8 text-center">
      <span className="flex size-16 items-center justify-center rounded-2xl bg-accent-soft text-accent">
        <Icon name="school" size={36} />
      </span>
      <h1 className="pt-2 text-headline-xl text-ink">SruNal KZ</h1>
      <p className="text-body-secondary text-ink-2">
        {t('Приложение работает внутри Telegram: заказы, оплата по Kaspi QR и готовые документы приходят в чат с ботом.')}
      </p>
      <button
        type="button"
        onClick={() => {
          haptic.tap();
          openTelegramLink(BOT_URL);
        }}
        className="mt-3 flex h-12 items-center justify-center gap-2 rounded-xl bg-accent px-6 text-button text-btn-text transition active:scale-[0.98]"
      >
        <Icon name="send" size={20} />
        {t('Открыть в Telegram')}
      </button>
      <span className="pt-2 text-caption text-ink-2">
        {t('Адрес бота - t.me/{bot}', { bot: BOT_URL.split('/').pop() ?? '' })}
      </span>
    </div>
  );
}
