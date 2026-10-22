import { initData } from '@/telegram/sdk';
import type {
  APIErrorPayload,
  AdminStats,
  ClientSession,
  CreateOrderResult,
  MyOrders,
  Order,
  OrderConsole,
  OrderForm,
  OrderListPage,
  PricingConfig,
  Product,
  PromoCheck,
  PromoCode,
  PromoInput,
  PublicSettings,
  Quote,
  ResultFile,
  Review,
  ReviewInput,
  ReviewListPage,
  Settings,
} from './types';

/**
 * The single API seam: every request carries the raw initData header, which
 * the backend validates (HMAC-SHA256 with the bot token) - no login code, no
 * bearer token inside the Mini App.
 */

const BASE = '/api';

export class ApiError extends Error {
  status: number;
  code: string;
  fields?: Record<string, string>;
  retryAfter?: number;

  constructor(status: number, payload: APIErrorPayload, retryAfter?: number) {
    super(payload.message);
    this.name = 'ApiError';
    this.status = status;
    this.code = payload.code;
    this.fields = payload.fields;
    this.retryAfter = retryAfter;
  }
}

interface RequestOptions {
  method?: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE';
  body?: unknown;
  idempotencyKey?: string;
}

async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const headers: Record<string, string> = {
    Accept: 'application/json',
  };
  const data = initData();
  if (data) {
    headers['X-Telegram-Init-Data'] = data;
  }
  if (options.body !== undefined) {
    headers['Content-Type'] = 'application/json';
  }
  if (options.idempotencyKey) {
    headers['Idempotency-Key'] = options.idempotencyKey;
  }

  let response: Response;
  try {
    response = await fetch(`${BASE}${path}`, {
      method: options.method ?? 'GET',
      headers,
      body: options.body === undefined ? undefined : JSON.stringify(options.body),
    });
  } catch {
    throw new ApiError(0, { code: 'network', message: 'Нет соединения - проверьте интернет' });
  }

  const retryAfterHeader = response.headers.get('Retry-After');
  const retryAfter = retryAfterHeader ? Number(retryAfterHeader) : undefined;

  if (response.status === 204) {
    return undefined as T;
  }

  let payload: unknown = null;
  try {
    payload = await response.json();
  } catch {
    payload = null;
  }

  if (!response.ok) {
    const envelope = (payload as { error?: APIErrorPayload } | null)?.error;
    throw new ApiError(
      response.status,
      envelope ?? { code: 'internal', message: 'Не удалось выполнить запрос, попробуйте ещё раз' },
      retryAfter,
    );
  }
  return payload as T;
}

export const api = {
  me(): Promise<ClientSession> {
    return request<ClientSession>('/auth/me');
  },

  products(): Promise<Product[]> {
    return request<{ products: Product[] }>('/products').then((data) => data.products);
  },

  settings(): Promise<PublicSettings> {
    return request<PublicSettings>('/settings');
  },

  quote(input: { product: string; works: number; units: number; promo?: string | null }): Promise<Quote> {
    const body = {
      product: input.product,
      works: input.works,
      units: input.units,
      // The server re-prices with its own table; the code only names the promo.
      promo: input.promo ? { code: input.promo } : null,
    };
    return request<Quote>('/quote', { method: 'POST', body });
  },

  promoCheck(input: {
    code: string;
    product: string;
    units: number;
    works: number;
    isFirstOrder: boolean;
    subtotal: number;
  }): Promise<PromoCheck> {
    return request<PromoCheck>('/promos/check', { method: 'POST', body: input });
  },

  createOrder(form: OrderForm, idempotencyKey: string): Promise<CreateOrderResult> {
    return request<CreateOrderResult>('/orders', { method: 'POST', body: form, idempotencyKey });
  },

  myOrders(): Promise<MyOrders> {
    return request<MyOrders>('/me/orders');
  },

  order(number: string): Promise<Order> {
    return request<Order>(`/orders/${encodeURIComponent(number)}`);
  },

  claim(number: string, senderName: string, token?: string): Promise<Order> {
    return request<Order>(`/orders/${encodeURIComponent(number)}/claim`, {
      method: 'POST',
      body: token ? { token, senderName } : { senderName },
    });
  },

  reviews(limit = 50): Promise<Review[]> {
    return request<Review[]>(`/reviews?status=published&limit=${limit}`);
  },

  addReview(input: ReviewInput): Promise<Review> {
    return request<Review>('/reviews', { method: 'POST', body: input });
  },

  // -------------------------------------------------------------------------
  // Operator panel. The same initData header authorizes these routes: the
  // backend checks that the chat is bound to an operator (service
  // AdminSessionFromInitData), so no separate login is needed inside Telegram.
  // -------------------------------------------------------------------------

  adminOrders(filter: { status?: string; q?: string; sort?: string; limit?: number } = {}): Promise<OrderListPage> {
    const params = new URLSearchParams();
    if (filter.status) params.set('status', filter.status);
    if (filter.q) params.set('q', filter.q);
    if (filter.sort) params.set('sort', filter.sort);
    params.set('limit', String(filter.limit ?? 100));
    return request<OrderListPage>(`/admin/orders?${params.toString()}`);
  },

  adminOrder(number: string): Promise<Order> {
    return request<Order>(`/admin/orders/${encodeURIComponent(number)}`);
  },

  adminConfirm(number: string): Promise<Order> {
    return request<Order>(`/admin/orders/${encodeURIComponent(number)}/confirm`, { method: 'POST', body: {} });
  },

  adminReject(number: string, reason: string): Promise<Order> {
    return request<Order>(`/admin/orders/${encodeURIComponent(number)}/reject`, {
      method: 'POST',
      body: { reason },
    });
  },

  adminCancel(number: string, reason = ''): Promise<Order> {
    return request<Order>(`/admin/orders/${encodeURIComponent(number)}/cancel`, {
      method: 'POST',
      body: { reason },
    });
  },

  adminRetry(number: string): Promise<Order> {
    return request<Order>(`/admin/orders/${encodeURIComponent(number)}/retry`, { method: 'POST', body: {} });
  },

  /** «Нужны правки»: the client asks for corrections on a finished order. */
  requestRevision(number: string, note: string): Promise<Order> {
    return request<Order>(`/orders/${encodeURIComponent(number)}/revision`, { method: 'POST', body: { note } });
  },

  /** Confirms the corrections and sends the finished work back to the agent. */
  adminRework(number: string, note: string): Promise<Order> {
    return request<Order>(`/admin/orders/${encodeURIComponent(number)}/rework`, { method: 'POST', body: { note } });
  },

  adminSaveNote(number: string, operatorNote: string): Promise<Order> {
    return request<Order>(`/admin/orders/${encodeURIComponent(number)}`, {
      method: 'PATCH',
      body: { operatorNote },
    });
  },

  adminConsole(number: string): Promise<OrderConsole> {
    return request<OrderConsole>(`/admin/orders/${encodeURIComponent(number)}/console`);
  },

  adminReviews(status = ''): Promise<ReviewListPage> {
    const query = status ? `?status=${encodeURIComponent(status)}` : '';
    return request<ReviewListPage>(`/admin/reviews${query}`);
  },

  adminModerateReview(id: string, status: 'published' | 'rejected' | 'pending'): Promise<Review> {
    return request<Review>(`/admin/reviews/${encodeURIComponent(id)}`, { method: 'PATCH', body: { status } });
  },

  adminPromos(): Promise<PromoCode[]> {
    return request<PromoCode[]>('/admin/promos');
  },

  adminCreatePromo(input: PromoInput): Promise<PromoCode> {
    return request<PromoCode>('/admin/promos', { method: 'POST', body: input });
  },

  adminUpdatePromo(id: string, patch: Partial<PromoInput>): Promise<PromoCode> {
    return request<PromoCode>(`/admin/promos/${encodeURIComponent(id)}`, { method: 'PATCH', body: patch });
  },

  adminDeletePromo(id: string): Promise<void> {
    return request<void>(`/admin/promos/${encodeURIComponent(id)}`, { method: 'DELETE' });
  },

  adminPricing(): Promise<PricingConfig> {
    return request<PricingConfig>('/admin/pricing');
  },

  adminSavePricing(pricing: PricingConfig): Promise<PricingConfig> {
    return request<PricingConfig>('/admin/pricing', { method: 'PUT', body: pricing });
  },

  adminSettings(): Promise<Settings> {
    return request<Settings>('/admin/settings');
  },

  adminSaveSettings(settings: Settings): Promise<Settings> {
    return request<Settings>('/admin/settings', { method: 'PUT', body: settings });
  },

  async adminUploadQr(file: File): Promise<Settings> {
    const form = new FormData();
    form.append('file', file);
    const headers: Record<string, string> = { Accept: 'application/json' };
    const data = initData();
    if (data) headers['X-Telegram-Init-Data'] = data;

    const response = await fetch(`${BASE}/admin/settings/payment-qr`, { method: 'POST', headers, body: form });
    const payload = (await response.json()) as { error?: APIErrorPayload; settings?: Settings };
    if (!response.ok) {
      throw new ApiError(response.status, payload.error ?? { code: 'internal', message: 'Не удалось загрузить QR' });
    }
    return payload.settings as Settings;
  },

  adminStats(): Promise<AdminStats> {
    return request<AdminStats>('/admin/stats');
  },

  adminConsoleLogUrl(number: string): string {
    return `${BASE}/admin/orders/${encodeURIComponent(number)}/console/log`;
  },
};

/** Absolute download URL of one result document (signed link from the backend).
 * Multi-work orders carry one document per work: pass the file to target it. */
export function resultFileUrl(order: Order, file?: ResultFile): string | undefined {
  const url = file?.url ?? order.resultFile?.url;
  if (!url) return undefined;
  // The backend builds the URL from PUBLIC_BASE_URL; keep it absolute when it
  // already is, otherwise resolve against the app origin (same-origin deploy).
  if (/^https?:\/\//.test(url)) return url;
  return new URL(url, window.location.origin).toString();
}

/** Absolute download URL of the primary document of a finished order. */
export function resultUrl(order: Order): string | undefined {
  return resultFileUrl(order);
}
