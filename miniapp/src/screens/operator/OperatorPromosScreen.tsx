import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { api } from '@/api/client';
import type { ProductCode, PromoCode, PromoInput } from '@/api/types';
import { BackButton, MainButton } from '@/telegram/hooks';
import { haptic, showConfirm } from '@/telegram/sdk';
import { useAsync } from '@/lib/hooks';
import { PRODUCTS_FALLBACK } from '@/lib/catalog';
import { formatDay, money } from '@/lib/format';
import { AppHeader, Card, EmptyState, Hairline, Icon, Row, Screen, Skeleton, Switch } from '@/components/ui';
import { Sheet, SheetOption } from '@/components/Sheet';
import { useToast } from '@/components/Toast';

const EMPTY_PROMO: PromoInput = {
  code: '',
  discountPct: 10,
  appliesTo: 'any',
  products: [],
  minAmount: null,
  minUnits: null,
  usageLimit: null,
  expiresAt: null,
  active: true,
};

/** Operator panel · promo codes (list, create, edit, enable/disable, delete). */
export function OperatorPromosScreen() {
  const navigate = useNavigate();
  const toast = useToast();
  const { data, error, loading, reload } = useAsync(() => api.adminPromos(), []);
  const [editing, setEditing] = useState<{ id?: string; input: PromoInput } | undefined>(undefined);
  const [busy, setBusy] = useState(false);

  const promos = data ?? [];

  const save = async () => {
    if (!editing) return;
    const input = editing.input;
    if (!input.code.trim()) {
      toast.show('Укажите код промокода', 'error');
      return;
    }
    if (input.discountPct < 1 || input.discountPct > 100) {
      toast.show('Скидка должна быть от 1 до 100 %', 'error');
      return;
    }
    setBusy(true);
    try {
      const payload: PromoInput = { ...input, code: input.code.trim().toUpperCase() };
      if (editing.id) {
        await api.adminUpdatePromo(editing.id, payload);
      } else {
        await api.adminCreatePromo(payload);
      }
      haptic.success();
      toast.show(editing.id ? 'Промокод обновлён' : 'Промокод создан');
      setEditing(undefined);
      reload();
    } catch (cause) {
      haptic.error();
      toast.show(cause instanceof Error ? cause.message : 'Не удалось сохранить промокод', 'error');
    } finally {
      setBusy(false);
    }
  };

  const toggleActive = async (promo: PromoCode) => {
    try {
      await api.adminUpdatePromo(promo.id, { active: !promo.active });
      reload();
    } catch (cause) {
      toast.show(cause instanceof Error ? cause.message : 'Не удалось переключить промокод', 'error');
    }
  };

  const remove = async (promo: PromoCode) => {
    const confirmed = await showConfirm(`Удалить промокод ${promo.code}? Действующие заказы он не затронет.`);
    if (!confirmed) return;
    try {
      await api.adminDeletePromo(promo.id);
      haptic.success();
      toast.show('Промокод удалён');
      reload();
    } catch (cause) {
      toast.show(cause instanceof Error ? cause.message : 'Не удалось удалить промокод', 'error');
    }
  };

  return (
    <Screen header={<AppHeader title="Промокоды" onBack={() => navigate('/operator')} />} bottomSpace>
      <div className="flex flex-col gap-3 pt-2">
        {loading && !data ? (
          <div className="flex flex-col gap-2">
            <Skeleton className="h-20 w-full" />
            <Skeleton className="h-20 w-full" />
          </div>
        ) : error ? (
          <Card className="items-center gap-2 p-6">
            <Icon name="error" size={26} className="text-danger" />
            <span className="text-body-secondary text-ink-2">{error}</span>
            <button type="button" onClick={reload} className="text-body-secondary-strong text-accent">
              Повторить
            </button>
          </Card>
        ) : promos.length === 0 ? (
          <Card>
            <EmptyState icon="sell" title="Промокодов нет" text="Создайте первый код кнопкой ниже." />
          </Card>
        ) : (
          promos.map((promo) => (
            <Card key={promo.id}>
              <div className="flex items-center justify-between gap-3 p-4">
                <div className="flex min-w-0 flex-col">
                  <span className="flex items-center gap-2">
                    <span className="font-mono text-[15px] font-medium text-ink">{promo.code}</span>
                    <span className={`text-body-secondary-strong ${promo.active ? 'text-success' : 'text-ink-2'}`}>
                      −{promo.discountPct} %
                    </span>
                  </span>
                  <span className="text-caption text-ink-2">
                    {promo.appliesTo === 'first' ? 'первый заказ' : 'любой заказ'}
                    {promo.products.length ? ` · ${promo.products.map(labels).join(', ')}` : ' · все виды'}
                    {promo.minUnits ? ` · от ${promo.minUnits} стр./слайдов` : ''}
                    {promo.minAmount ? ` · от ${money(promo.minAmount)}` : ''}
                  </span>
                  <span className="text-caption text-ink-2">
                    использован {promo.usedCount}
                    {promo.usageLimit ? ` из ${promo.usageLimit}` : ''}
                    {promo.expiresAt ? ` · до ${formatDay(promo.expiresAt)}` : ''}
                  </span>
                </div>
                <Switch checked={promo.active} onChange={() => toggleActive(promo)} label={`Промокод ${promo.code}`} />
              </div>
              <Hairline />
              <Row
                icon="edit"
                iconTone="ink-2"
                title="Изменить"
                onClick={() =>
                  setEditing({
                    id: promo.id,
                    input: {
                      code: promo.code,
                      discountPct: promo.discountPct,
                      appliesTo: promo.appliesTo,
                      products: promo.products,
                      minAmount: promo.minAmount,
                      minUnits: promo.minUnits,
                      usageLimit: promo.usageLimit,
                      expiresAt: promo.expiresAt ? promo.expiresAt.slice(0, 10) : null,
                      active: promo.active,
                    },
                  })
                }
              />
              <Hairline />
              <Row icon="delete" title="Удалить" destructive onClick={() => remove(promo)} />
            </Card>
          ))
        )}
      </div>

      <MainButton text="Новый промокод" onClick={() => setEditing({ input: { ...EMPTY_PROMO } })} />

      <Sheet
        open={Boolean(editing)}
        title={editing?.id ? `Промокод ${editing.input.code}` : 'Новый промокод'}
        onClose={() => setEditing(undefined)}
      >
        {editing ? (
          <div className="flex flex-col gap-2 px-4 pb-3">
            <PromoField
              label="Код"
              value={editing.input.code}
              mono
              onChange={(value) => setEditing({ ...editing, input: { ...editing.input, code: value.toUpperCase() } })}
            />
            <PromoField
              label="Скидка, %"
              value={String(editing.input.discountPct)}
              numeric
              onChange={(value) =>
                setEditing({ ...editing, input: { ...editing.input, discountPct: clampInt(value, 0, 100) } })
              }
            />
            <div className="flex flex-col gap-1 py-1">
              <span className="text-caption text-ink-2">Действует на</span>
              <div className="flex gap-2">
                {(['any', 'first'] as const).map((value) => (
                  <button
                    key={value}
                    type="button"
                    onClick={() => setEditing({ ...editing, input: { ...editing.input, appliesTo: value } })}
                    className={
                      editing.input.appliesTo === value
                        ? 'h-9 flex-1 rounded-[10px] bg-accent-soft text-body-secondary-strong text-accent'
                        : 'h-9 flex-1 rounded-[10px] bg-neutral-soft text-body-secondary text-ink-2'
                    }
                  >
                    {value === 'any' ? 'Любой заказ' : 'Первый заказ'}
                  </button>
                ))}
              </div>
            </div>
            <span className="px-1 pt-1 text-caption text-ink-2">Виды работ (пусто - на все)</span>
            <SheetOption
              title="Реферат"
              selected={editing.input.products.includes('essay')}
              onClick={() => setEditing({ ...editing, input: { ...editing.input, products: toggleProduct(editing.input.products, 'essay') } })}
            />
            <SheetOption
              title="Лабораторная"
              selected={editing.input.products.includes('lab')}
              onClick={() => setEditing({ ...editing, input: { ...editing.input, products: toggleProduct(editing.input.products, 'lab') } })}
            />
            <SheetOption
              title="Презентация"
              selected={editing.input.products.includes('presentation')}
              onClick={() =>
                setEditing({ ...editing, input: { ...editing.input, products: toggleProduct(editing.input.products, 'presentation') } })
              }
            />
            <SheetOption
              title="Другой документ (СРС/СРСП)"
              selected={editing.input.products.includes('other')}
              onClick={() => setEditing({ ...editing, input: { ...editing.input, products: toggleProduct(editing.input.products, 'other') } })}
            />
            <PromoField
              label="Минимальный объём (стр./слайды)"
              value={editing.input.minUnits === null ? '' : String(editing.input.minUnits)}
              numeric
              onChange={(value) =>
                setEditing({
                  ...editing,
                  input: { ...editing.input, minUnits: value.trim() === '' ? null : clampInt(value, 0, 1000) },
                })
              }
            />
            <PromoField
              label="Минимальная сумма, ₸"
              value={editing.input.minAmount === null ? '' : String(editing.input.minAmount)}
              numeric
              onChange={(value) =>
                setEditing({
                  ...editing,
                  input: { ...editing.input, minAmount: value.trim() === '' ? null : clampInt(value, 0, 10_000_000) },
                })
              }
            />
            <PromoField
              label="Лимит использований"
              value={editing.input.usageLimit === null ? '' : String(editing.input.usageLimit)}
              numeric
              onChange={(value) =>
                setEditing({
                  ...editing,
                  input: { ...editing.input, usageLimit: value.trim() === '' ? null : clampInt(value, 0, 100000) },
                })
              }
            />
            <PromoField
              label="Действует до (ГГГГ-ММ-ДД)"
              value={editing.input.expiresAt ?? ''}
              onChange={(value) =>
                setEditing({ ...editing, input: { ...editing.input, expiresAt: value.trim() === '' ? null : value.trim() } })
              }
            />
            <button
              type="button"
              disabled={busy}
              onClick={save}
              className="mt-1 flex h-11 w-full items-center justify-center rounded-xl bg-accent text-body-secondary-strong text-btn-text transition active:scale-[0.98] disabled:opacity-50"
            >
              {editing.id ? 'Сохранить' : 'Создать промокод'}
            </button>
          </div>
        ) : null}
      </Sheet>
      <BackButton onClick={() => navigate('/operator')} />
    </Screen>
  );
}

function PromoField({
  label,
  value,
  onChange,
  mono = false,
  numeric = false,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  mono?: boolean;
  numeric?: boolean;
}) {
  return (
    <label className="flex flex-col gap-0.5 rounded-xl bg-page px-3 py-2">
      <span className="text-caption text-ink-2">{label}</span>
      <input
        value={value}
        inputMode={numeric ? 'numeric' : 'text'}
        spellCheck={false}
        onChange={(event) => onChange(event.target.value)}
        className={`w-full bg-transparent text-body-secondary text-ink outline-none ${mono ? 'font-mono uppercase' : ''}`}
      />
    </label>
  );
}

function clampInt(value: string, min: number, max: number): number {
  const parsed = Number.parseInt(value.replace(/\D/g, ''), 10);
  if (Number.isNaN(parsed)) return min;
  return Math.min(max, Math.max(min, parsed));
}

function toggleProduct(list: ProductCode[], code: ProductCode): ProductCode[] {
  return list.includes(code) ? list.filter((item) => item !== code) : [...list, code];
}

function labels(code: ProductCode): string {
  return PRODUCTS_FALLBACK.find((product) => product.code === code)?.name ?? code;
}
