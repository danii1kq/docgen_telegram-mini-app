import { useEffect, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { api } from '@/api/client';
import type { Settings } from '@/api/types';
import { BackButton, MainButton } from '@/telegram/hooks';
import { haptic } from '@/telegram/sdk';
import { useAsync } from '@/lib/hooks';
import { AppHeader, Card, Hairline, Icon, Screen, SectionLabel, Skeleton, Switch } from '@/components/ui';
import { TextRow } from '@/components/fields';
import { useToast } from '@/components/Toast';

/** Operator panel · settings: requisites, support, maintenance, queue, Kaspi QR. */
export function OperatorSettingsScreen() {
  const navigate = useNavigate();
  const toast = useToast();
  const { data, error, loading, reload } = useAsync(() => api.adminSettings(), []);
  const [settings, setSettings] = useState<Settings | undefined>(undefined);
  const [dirty, setDirty] = useState(false);
  const [busy, setBusy] = useState(false);
  const fileInput = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (data && !dirty) setSettings(data);
  }, [data, dirty]);

  const update = (patch: Partial<Settings>) => {
    setSettings((current) => (current ? { ...current, ...patch } : current));
    setDirty(true);
  };

  const updateRequisites = (patch: Partial<Settings['requisites']>) => {
    setSettings((current) => (current ? { ...current, requisites: { ...current.requisites, ...patch } } : current));
    setDirty(true);
  };

  const save = async () => {
    if (!settings) return;
    setBusy(true);
    try {
      const saved = await api.adminSaveSettings(settings);
      haptic.success();
      toast.show('Настройки сохранены');
      setSettings(saved);
      setDirty(false);
      reload();
    } catch (cause) {
      haptic.error();
      toast.show(cause instanceof Error ? cause.message : 'Не удалось сохранить настройки', 'error');
    } finally {
      setBusy(false);
    }
  };

  const uploadQr = async (file: File | undefined) => {
    if (!file) return;
    setBusy(true);
    try {
      const saved = await api.adminUploadQr(file);
      haptic.success();
      toast.show('QR-код обновлён');
      setSettings(saved);
      setDirty(false);
    } catch (cause) {
      haptic.error();
      toast.show(cause instanceof Error ? cause.message : 'Не удалось загрузить QR', 'error');
    } finally {
      setBusy(false);
      if (fileInput.current) fileInput.current.value = '';
    }
  };

  return (
    <Screen header={<AppHeader title="Настройки" onBack={() => navigate('/operator')} />} bottomSpace>
      <div className="flex flex-col gap-3 pt-2">
        {loading && !settings ? (
          <div className="flex flex-col gap-2">
            <Skeleton className="h-32 w-full" />
            <Skeleton className="h-24 w-full" />
          </div>
        ) : error || !settings ? (
          <Card className="items-center gap-2 p-6">
            <Icon name="error" size={26} className="text-danger" />
            <span className="text-body-secondary text-ink-2">{error ?? 'Не удалось загрузить настройки'}</span>
            <button type="button" onClick={reload} className="text-body-secondary-strong text-accent">
              Повторить
            </button>
          </Card>
        ) : (
          <>
            <SectionLabel>Режим работы</SectionLabel>
            <Card>
              <div className="flex items-center justify-between gap-3 px-4 py-3">
                <div className="flex min-w-0 flex-col">
                  <span className="text-body-strong text-ink">Приём заказов</span>
                  <span className="text-caption text-ink-2">
                    {settings.maintenance ? 'Приостановлен (клиенты видят баннер)' : 'Открыт'}
                  </span>
                </div>
                <Switch
                  checked={!settings.maintenance}
                  label="Приём заказов"
                  onChange={(open) => update({ maintenance: !open })}
                />
              </div>
              <Hairline />
              <div className="flex items-center justify-between gap-3 px-4 py-3">
                <div className="flex min-w-0 flex-col">
                  <span className="text-body-strong text-ink">Очередь генерации</span>
                  <span className="text-caption text-ink-2">
                    {settings.queuePaused ? 'На паузе - новые заказы не берутся' : 'Работает'}
                  </span>
                </div>
                <Switch
                  checked={!settings.queuePaused}
                  label="Очередь генерации"
                  onChange={(working) => update({ queuePaused: !working })}
                />
              </div>
            </Card>

            <SectionLabel>Оплата</SectionLabel>
            <Card>
              <TextRow
                label="Ссылка на оплату Kaspi"
                value={settings.paymentLink ?? ''}
                placeholder="https://qr.kaspi.kz/…"
                onChange={(paymentLink) => update({ paymentLink })}
                hint="Кнопка «Оплатить через Kaspi» в приложении. Пусто - клиенты увидят QR-картинку ниже."
              />
            </Card>

            <SectionLabel>Kaspi QR (запасной вариант)</SectionLabel>
            <Card className="items-center gap-3 p-4">
              <div className="flex size-40 items-center justify-center overflow-hidden rounded-2xl border border-line bg-qr-plate">
                {settings.paymentQr ? (
                  <img src={settings.paymentQr} alt="Kaspi QR" className="size-full object-contain p-2" />
                ) : (
                  <Icon name="qr_code_2" size={56} className="text-outline-variant" />
                )}
              </div>
              <label className="flex h-11 w-full cursor-pointer items-center justify-center gap-2 rounded-xl bg-accent-soft text-body-secondary-strong text-accent">
                <input
                  ref={fileInput}
                  type="file"
                  accept="image/png,image/jpeg,image/webp"
                  className="hidden"
                  onChange={(event) => uploadQr(event.target.files?.[0])}
                />
                <Icon name="upload" size={20} />
                Загрузить QR (PNG/JPG, до 1,5 МБ)
              </label>
              <span className="text-caption text-ink-2">
                Показывается клиенту, только если ссылка на оплату не заполнена.
              </span>
            </Card>

            <SectionLabel>Получатель и реквизиты</SectionLabel>
            <Card>
              <TextRow
                label="Получатель на карточке"
                value={settings.paymentRecipient}
                placeholder="Даниил Л."
                onChange={(paymentRecipient) => update({ paymentRecipient })}
              />
              <Hairline />
              <TextRow
                label="Реквизиты: получатель"
                value={settings.requisites.recipient}
                placeholder="ИП «SruNal KZ»"
                onChange={(recipient) => updateRequisites({ recipient })}
              />
              <Hairline />
              <TextRow
                label="Банк"
                value={settings.requisites.bank}
                placeholder="Kaspi Bank"
                onChange={(bank) => updateRequisites({ bank })}
              />
              <Hairline />
              <TextRow
                label="Номер карты"
                value={settings.requisites.card}
                placeholder="4400 4300 0000 0000"
                onChange={(card) => updateRequisites({ card })}
              />
              <Hairline />
              <TextRow
                label="IBAN"
                value={settings.requisites.iban}
                placeholder="KZ00 0000 0000 0000"
                onChange={(iban) => updateRequisites({ iban })}
              />
              <Hairline />
              <TextRow
                label="Примечание к реквизитам"
                value={settings.requisites.note}
                placeholder="Перевод по номеру телефона"
                onChange={(note) => updateRequisites({ note })}
              />
            </Card>

            <SectionLabel>Поддержка</SectionLabel>
            <Card>
              <TextRow
                label="Telegram-контакт"
                value={settings.supportTelegram}
                placeholder="@meownuarka"
                onChange={(supportTelegram) => update({ supportTelegram })}
                hint="Показывается клиентам в приложении и в уведомлениях"
              />
            </Card>
          </>
        )}
      </div>

      <MainButton text={dirty ? 'Сохранить' : 'Без изменений'} disabled={!dirty} progress={busy} onClick={save} />
      <BackButton onClick={() => navigate('/operator')} />
    </Screen>
  );
}
