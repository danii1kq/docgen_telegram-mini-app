import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { api } from '@/api/client';
import type { PricingConfig } from '@/api/types';
import { BackButton, MainButton } from '@/telegram/hooks';
import { haptic } from '@/telegram/sdk';
import { useAsync } from '@/lib/hooks';
import { PRODUCTS_FALLBACK } from '@/lib/catalog';
import { money } from '@/lib/format';
import { AppHeader, Card, Hairline, Icon, Screen, SectionLabel, Skeleton, Switch } from '@/components/ui';
import { useToast } from '@/components/Toast';

/** Operator panel · pricing: unit prices, volume tiers and product availability. */
export function OperatorPricingScreen() {
  const navigate = useNavigate();
  const toast = useToast();
  const { data, error, loading, reload } = useAsync(() => api.adminPricing(), []);
  const [config, setConfig] = useState<PricingConfig | undefined>(undefined);
  const [busy, setBusy] = useState(false);
  const [dirty, setDirty] = useState(false);

  useEffect(() => {
    if (data && !dirty) setConfig(structuredClone(data));
  }, [data, dirty]);

  const update = (patch: (current: PricingConfig) => PricingConfig) => {
    setConfig((current) => (current ? patch(current) : current));
    setDirty(true);
  };

  const save = async () => {
    if (!config) return;
    setBusy(true);
    try {
      const saved = await api.adminSavePricing(config);
      haptic.success();
      toast.show('Тарифы сохранены');
      setConfig(structuredClone(saved));
      setDirty(false);
      reload();
    } catch (cause) {
      haptic.error();
      toast.show(cause instanceof Error ? cause.message : 'Не удалось сохранить тарифы', 'error');
    } finally {
      setBusy(false);
    }
  };

  return (
    <Screen header={<AppHeader title="Тарифы" onBack={() => navigate('/operator')} />} bottomSpace>
      <div className="flex flex-col gap-3 pt-2">
        {loading && !config ? (
          <div className="flex flex-col gap-2">
            <Skeleton className="h-32 w-full" />
            <Skeleton className="h-24 w-full" />
          </div>
        ) : error || !config ? (
          <Card className="items-center gap-2 p-6">
            <Icon name="error" size={26} className="text-danger" />
            <span className="text-body-secondary text-ink-2">{error ?? 'Не удалось загрузить тарифы'}</span>
            <button type="button" onClick={reload} className="text-body-secondary-strong text-accent">
              Повторить
            </button>
          </Card>
        ) : (
          <>
            <div className="flex items-start gap-2 rounded-[14px] bg-warning-soft px-4 py-3 text-warning">
              <Icon name="info" size={20} filled />
              <span className="text-caption">
                Цены замораживаются в момент создания заказа: правки не меняют существующие заказы.
              </span>
            </div>

            <SectionLabel>Виды работ</SectionLabel>
            {config.products.map((product) => {
              const meta = PRODUCTS_FALLBACK.find((item) => item.code === product.code);
              return (
                <Card key={product.code} className="gap-2 p-4">
                  <div className="flex items-center justify-between">
                    <span className="text-body-strong text-ink">{meta?.name ?? product.code}</span>
                    <Switch
                      checked={product.active}
                      label={`${meta?.name ?? product.code} активен`}
                      onChange={(active) =>
                        update((current) => ({
                          ...current,
                          products: current.products.map((item) =>
                            item.code === product.code ? { ...item, active } : item,
                          ),
                        }))
                      }
                    />
                  </div>
                  <div className="flex gap-2">
                    <NumberField
                      label={meta?.fixed ? 'Цена, ₸' : 'Цена за единицу, ₸'}
                      value={product.unitPrice}
                      onChange={(value) =>
                        update((current) => ({
                          ...current,
                          products: current.products.map((item) =>
                            item.code === product.code ? { ...item, unitPrice: value } : item,
                          ),
                        }))
                      }
                    />
                    <NumberField
                      label="Мин. объём"
                      value={product.min}
                      onChange={(value) =>
                        update((current) => ({
                          ...current,
                          products: current.products.map((item) =>
                            item.code === product.code ? { ...item, min: value } : item,
                          ),
                        }))
                      }
                    />
                    <NumberField
                      label="Макс. объём"
                      value={product.max}
                      onChange={(value) =>
                        update((current) => ({
                          ...current,
                          products: current.products.map((item) =>
                            item.code === product.code ? { ...item, max: value } : item,
                          ),
                        }))
                      }
                    />
                  </div>
                </Card>
              );
            })}

            <SectionLabel
              action={
                <button
                  type="button"
                  onClick={() =>
                    update((current) => ({
                      ...current,
                      discounts: [...current.discounts, { min: 5, pct: 10 }],
                    }))
                  }
                  className="text-caption text-accent"
                >
                  Добавить порог
                </button>
              }
            >
              Скидки за объём
            </SectionLabel>
            <Card>
              {config.discounts.length === 0 ? (
                <div className="p-4 text-body-secondary text-ink-2">Скидок нет</div>
              ) : (
                config.discounts.map((discount, index) => (
                  <div key={`${discount.min}-${index}`}>
                    {index > 0 ? <Hairline /> : null}
                    <div className="flex items-center gap-2 px-4 py-3">
                      <span className="text-body-secondary text-ink-2">от</span>
                      <input
                        value={discount.min}
                        inputMode="numeric"
                        onChange={(event) =>
                          update((current) => ({
                            ...current,
                            discounts: current.discounts.map((item, position) =>
                              position === index
                                ? { ...item, min: clampInt(event.target.value, 1, 1000) }
                                : item,
                            ),
                          }))
                        }
                        className="w-16 rounded-lg bg-page px-2 py-1 text-center text-body-secondary tabular-nums text-ink outline-none"
                      />
                      <span className="text-body-secondary text-ink-2">работ →</span>
                      <input
                        value={discount.pct}
                        inputMode="numeric"
                        onChange={(event) =>
                          update((current) => ({
                            ...current,
                            discounts: current.discounts.map((item, position) =>
                              position === index
                                ? { ...item, pct: clampInt(event.target.value, 1, 90) }
                                : item,
                            ),
                          }))
                        }
                        className="w-14 rounded-lg bg-page px-2 py-1 text-center text-body-secondary tabular-nums text-ink outline-none"
                      />
                      <span className="text-body-secondary text-ink-2">%</span>
                      <button
                        type="button"
                        aria-label="Удалить порог"
                        onClick={() =>
                          update((current) => ({
                            ...current,
                            discounts: current.discounts.filter((_, position) => position !== index),
                          }))
                        }
                        className="ml-auto flex size-8 items-center justify-center text-danger"
                      >
                        <Icon name="close" size={18} />
                      </button>
                    </div>
                  </div>
                ))
              )}
            </Card>

            <span className="px-1 text-caption text-ink-2">
              Пример: реферат 10 страниц по текущей цене - {money(priceFor(config, 'essay'))}
            </span>
          </>
        )}
      </div>

      <MainButton text={dirty ? 'Сохранить тарифы' : 'Без изменений'} disabled={!dirty} progress={busy} onClick={save} />
      <BackButton onClick={() => navigate('/operator')} />
    </Screen>
  );
}

function NumberField({
  label,
  value,
  onChange,
}: {
  label: string;
  value: number;
  onChange: (value: number) => void;
}) {
  return (
    <label className="flex min-w-0 flex-1 flex-col gap-0.5 rounded-xl bg-page px-3 py-2">
      <span className="truncate text-caption text-ink-2">{label}</span>
      <input
        value={value}
        inputMode="numeric"
        onChange={(event) => onChange(clampInt(event.target.value, 0, 10_000_000))}
        className="w-full bg-transparent text-body-strong tabular-nums text-ink outline-none"
      />
    </label>
  );
}

function clampInt(value: string, min: number, max: number): number {
  const parsed = Number.parseInt(value.replace(/\D/g, ''), 10);
  if (Number.isNaN(parsed)) return min;
  return Math.min(max, Math.max(min, parsed));
}

function priceFor(config: PricingConfig, code: string): number {
  const product = config.products.find((item) => item.code === code);
  return product ? product.unitPrice : 0;
}
