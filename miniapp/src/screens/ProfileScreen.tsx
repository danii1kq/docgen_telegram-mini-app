import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { BackButton } from '@/telegram/hooks';
import { haptic, openTelegramLink, showConfirm, telegramUser } from '@/telegram/sdk';
import { APP_VERSION, BOT_USERNAME, BOT_URL } from '@/lib/app';
import { SUPPORT_HANDLE } from '@/lib/catalog';
import { clearDraft, isDirty, loadDraft } from '@/lib/draft';
import { useSettings } from '@/lib/hooks';
import { useOperator } from '@/app/session';
import { applyThemeMode, loadThemeMode, saveThemeMode, type ThemeMode } from '@/app/theme';
import { LANGS, useI18n } from '@/i18n';
import { Sheet, SheetOption } from '@/components/Sheet';
import { AppHeader, Avatar, Card, Hairline, Icon, Row, Screen, SectionLabel } from '@/components/ui';
import { useFeedback } from '@/components/Toast';

/** S14 · Profile: identity, interface settings, app facts, documents, data reset. */
export function ProfileScreen() {
  const navigate = useNavigate();
  const user = telegramUser();
  const feedback = useFeedback();
  const settings = useSettings();
  const operator = useOperator();
  const { t, lang, setLang } = useI18n();
  const supportHandle = settings?.supportTelegram ?? SUPPORT_HANDLE;
  const [draftDirty, setDraftDirty] = useState(() => isDirty(loadDraft()));
  const [theme, setTheme] = useState<ThemeMode>(() => loadThemeMode());
  const [themeOpen, setThemeOpen] = useState(false);
  const [langOpen, setLangOpen] = useState(false);

  const themeLabel: Record<ThemeMode, string> = {
    auto: t('Как в Telegram'),
    light: t('Светлая'),
    dark: t('Тёмная'),
  };

  const reset = async () => {
    const confirmed = await showConfirm(t('Очистить черновик заказа? Заполненные данные будут удалены.'));
    if (!confirmed) return;
    clearDraft();
    setDraftDirty(false);
    haptic.success();
    feedback(t('Черновик очищен'), 'success');
  };

  const pickTheme = (mode: ThemeMode) => {
    saveThemeMode(mode);
    applyThemeMode(mode);
    setTheme(mode);
    haptic.select();
    setThemeOpen(false);
  };

  const pickLang = (next: typeof lang) => {
    setLang(next);
    haptic.select();
    setLangOpen(false);
  };

  return (
    <Screen header={<AppHeader title={t('Профиль')} onBack={() => navigate(-1)} />}>
      <div className="flex flex-col gap-3 pt-2">
        <Card>
          <div className="flex items-center gap-3 p-4">
            <Avatar photoUrl={user?.photo_url} name={user?.first_name} size={48} />
            <div className="flex min-w-0 flex-col">
              <span className="truncate text-headline-md text-ink">
                {user ? `${user.first_name} ${user.last_name ?? ''}`.trim() : t('Гость')}
              </span>
              <span className="truncate text-body-secondary text-accent">
                {user?.username ? `@${user.username}` : 'Telegram'}
              </span>
            </div>
          </div>
        </Card>

        <SectionLabel>{t('Уведомления')}</SectionLabel>
        <Card>
          <Row
            icon="notifications"
            title={t('Уведомления в боте')}
            value={<span className="text-success">{t('Включены')}</span>}
            subtitle={t('Управляются командами /start и /stop в чате с @{bot}', { bot: BOT_USERNAME })}
          />
        </Card>

        <SectionLabel>{t('Приложение')}</SectionLabel>
        <Card>
          <Row icon="palette" title={t('Тема')} value={themeLabel[theme]} onClick={() => setThemeOpen(true)} />
          <Hairline />
          <Row
            icon="language"
            title={t('Язык интерфейса')}
            value={lang === 'ru' ? 'Русский' : lang === 'kk' ? 'Қазақша' : 'English'}
            onClick={() => setLangOpen(true)}
          />
          <Hairline />
          <Row icon="info" title={t('Версия')} value={`v${APP_VERSION}`} />
        </Card>

        <SectionLabel>{t('Документы')}</SectionLabel>
        <Card>
          <Row icon="description" title={t('Публичная оферта')} onClick={() => navigate('/legal/offer')} />
          <Hairline />
          <Row icon="shield" title={t('Персональные данные')} onClick={() => navigate('/legal/personal')} />
          <Hairline />
          <Row icon="lock" title={t('Политика конфиденциальности')} onClick={() => navigate('/legal/privacy')} />
        </Card>

        {operator ? (
          <>
            <SectionLabel>{t('Оператор')}</SectionLabel>
            <Card>
              <Row
                icon="admin_panel_settings"
                title={t('Панель оператора')}
                subtitle={`${operator.name} · ${operator.id}`}
                onClick={() => navigate('/operator')}
              />
            </Card>
          </>
        ) : null}

        <SectionLabel>{t('Черновик')}</SectionLabel>
        <Card>
          <Row
            icon="edit_note"
            title={draftDirty ? t('Очистить черновик') : t('Черновик пуст')}
            subtitle={
              draftDirty ? t('Данные оформления сохранены на устройстве') : t('Нет незавершённых заказов')
            }
            destructive={draftDirty}
            disabled={!draftDirty}
            onClick={draftDirty ? reset : undefined}
          />
        </Card>

        <div className="flex flex-col items-center gap-1 pt-4 text-center">
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
          <span className="flex items-center gap-1 text-caption text-ink-2">
            <Icon name="verified" size={16} className="text-success" filled />
            SruNal KZ · Satbayev University
          </span>
        </div>
      </div>

      <Sheet open={themeOpen} title={t('Выберите тему')} onClose={() => setThemeOpen(false)}>
        {(['auto', 'light', 'dark'] as ThemeMode[]).map((mode) => (
          <SheetOption
            key={mode}
            title={themeLabel[mode]}
            subtitle={
              mode === 'auto' ? t('Тема приложения следует настройке Telegram') : undefined
            }
            selected={theme === mode}
            onClick={() => pickTheme(mode)}
          />
        ))}
      </Sheet>

      <Sheet open={langOpen} title={t('Выберите язык')} onClose={() => setLangOpen(false)}>
        {LANGS.map((option) => (
          <SheetOption
            key={option.value}
            title={option.label}
            selected={lang === option.value}
            onClick={() => pickLang(option.value)}
          />
        ))}
      </Sheet>

      <BackButton onClick={() => navigate(-1)} />
    </Screen>
  );
}
