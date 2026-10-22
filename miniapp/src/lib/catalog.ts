import type { OrderStatus, Product } from '@/api/types';

/** Static UI copy mirroring the backend catalog (labels, icons, hints). */

export interface StatusMeta {
  label: string;
  icon: string;
  tone: 'accent' | 'success' | 'warning' | 'danger' | 'neutral';
  filled?: boolean;
}

export const STATUS_META: Record<OrderStatus, StatusMeta> = {
  awaiting_payment: { label: 'Ожидает оплату', icon: 'hourglass_top', tone: 'warning', filled: true },
  payment_claimed: { label: 'Оплата проверяется', icon: 'hourglass_top', tone: 'warning', filled: true },
  payment_rejected: { label: 'Оплата отклонена', icon: 'error', tone: 'danger', filled: true },
  queued: { label: 'В очереди', icon: 'schedule', tone: 'accent' },
  generating: { label: 'Готовим работу', icon: 'autorenew', tone: 'accent' },
  completed: { label: 'Готово', icon: 'check_circle', tone: 'success', filled: true },
  failed_final: { label: 'Проблема', icon: 'report', tone: 'danger', filled: true },
  expired: { label: 'Истёк', icon: 'schedule', tone: 'neutral' },
  cancelled: { label: 'Отменён', icon: 'cancel', tone: 'neutral' },
};

export const ACTIVE_STATUSES: OrderStatus[] = [
  'awaiting_payment',
  'payment_claimed',
  'payment_rejected',
  'queued',
  'generating',
];

export const DONE_STATUSES: OrderStatus[] = ['completed'];

export const ARCHIVE_STATUSES: OrderStatus[] = ['failed_final', 'expired', 'cancelled'];

export function isActive(status: OrderStatus): boolean {
  return ACTIVE_STATUSES.includes(status);
}

/** The 4-step progress tracker of the order screen. */
export const PROGRESS_STEPS = ['Оплата', 'Проверка', 'В работе', 'Готово'] as const;

export function progressIndex(status: OrderStatus): number {
  switch (status) {
    case 'awaiting_payment':
    case 'payment_rejected':
    case 'expired':
    case 'cancelled':
      return 0;
    case 'payment_claimed':
      return 1;
    case 'queued':
    case 'generating':
      return 2;
    case 'completed':
      return 4;
    case 'failed_final':
      return 3;
    default:
      return 0;
  }
}

export const FILE_ACCEPT = '.pdf,.docx,.txt,.png,.jpg,.jpeg';
export const FILE_MAX_COUNT = 3;
export const FILE_MAX_SIZE = 5 * 1024 * 1024;
export const FILE_FORMATS = ['PDF', 'DOCX', 'TXT', 'PNG', 'JPG'];

export function productIcon(code: string): string {
  switch (code) {
    case 'essay':
      return 'article';
    case 'lab':
      return 'science';
    case 'presentation':
      return 'slideshow';
    default:
      return 'assignment';
  }
}

/** Fallback catalogue when /api/products has not answered yet. */
export const PRODUCTS_FALLBACK: Product[] = [
  {
    code: 'essay',
    name: 'Реферат',
    short: 'Реферат',
    unitOne: 'страница',
    unitFew: 'страницы',
    unitMany: 'страниц',
    unitTitle: 'страницы',
    min: 4,
    max: 40,
    fixed: false,
    price: 80,
  },
  {
    code: 'lab',
    name: 'Лабораторная',
    short: 'Лабораторная',
    unitOne: 'работа',
    unitFew: 'работы',
    unitMany: 'работ',
    unitTitle: 'работы',
    min: 1,
    max: 1,
    fixed: true,
    price: 1000,
  },
  {
    code: 'presentation',
    name: 'Презентация',
    short: 'Презентация',
    unitOne: 'слайд',
    unitFew: 'слайда',
    unitMany: 'слайдов',
    unitTitle: 'слайды',
    min: 5,
    max: 30,
    fixed: false,
    price: 240,
  },
  {
    code: 'other',
    name: 'Другой док.',
    short: 'СРС / СРСП / другое',
    unitOne: 'работа',
    unitFew: 'работы',
    unitMany: 'работ',
    unitTitle: 'работы',
    min: 1,
    max: 1,
    fixed: true,
    price: 1000,
  },
];

export const SUPPORT_HANDLE = '@meownuarka';
