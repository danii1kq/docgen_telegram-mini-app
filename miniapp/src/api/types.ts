/** API contract = the Go backend's JSON shapes (internal/domain). */

export type ProductCode = 'essay' | 'lab' | 'presentation' | 'other';

export type OrderStatus =
  | 'awaiting_payment'
  | 'payment_claimed'
  | 'payment_rejected'
  | 'queued'
  | 'generating'
  | 'completed'
  | 'failed_final'
  | 'expired'
  | 'cancelled';

export interface Product {
  code: ProductCode;
  name: string;
  short: string;
  unitOne: string;
  unitFew: string;
  unitMany: string;
  unitTitle: string;
  /** Short unit for price hints: "стр." / "слайд" / "раб.". */
  unitLabel?: string;
  min: number;
  max: number;
  fixed: boolean;
  /** Price per unit / flat price, from the public pricing config. */
  price?: number;
}

export interface QuoteLine {
  label: string;
  value: string;
  hint?: string;
  discount?: boolean;
}

export interface Quote {
  product: string;
  unitPrice: number;
  units: number;
  works: number;
  subtotal: number;
  discountPct: number;
  discount: number;
  promoCode: string | null;
  promoPct: number;
  promoDiscount: number;
  total: number;
  lines: QuoteLine[];
}

export interface FileMeta {
  name: string;
  size: number;
  type: string;
}

export interface ResultFile {
  /** Row id in order_files (multi-work orders expose one file per work). */
  id?: number;
  name: string;
  size: number;
  type: string;
  url: string;
  expiresAt?: string;
}

export interface OrderEvent {
  at: string;
  actor: 'client' | 'operator' | 'system';
  text: string;
}

export interface Order {
  number: string;
  token?: string;
  status: OrderStatus;
  createdAt: string;
  updatedAt: string;
  senderName: string;
  product: ProductCode;
  works: number;
  units: number;
  topics: string;
  notes: string;
  workKind: string;
  language: string;
  institute: string;
  department: string;
  studentName: string;
  disciplineCode: string;
  teacherName: string;
  attachments: FileMeta[];
  contactName: string;
  contactType: string;
  contact: string;
  amount: number;
  quoteLines: QuoteLine[];
  promoCode?: string;
  promoDiscountPct?: number;
  clientNote: string;
  paymentRejectedReason?: string;
  operatorNotes?: string;
  claimedAt?: string;
  confirmedAt?: string;
  completedAt?: string;
  events: OrderEvent[];
  resultFile?: ResultFile;
  /** Every document of the newest generation run (one per work). */
  resultFiles?: ResultFile[];
  /** Corrections the client asked for on a finished order (operator rework). */
  revisionNote?: string;
  revisionRequestedAt?: string;
  /** What the generation runs of this order cost in USD (operator panel). */
  agentCostUsd?: number;
  /** The same amount in tenge at the current rate. */
  agentCostKzt?: number;
}

export interface MyOrders {
  orders: Order[];
  counts: Record<string, number>;
}

export interface OrderForm {
  product: ProductCode;
  works: number;
  units: number;
  workKind: string;
  topics: string;
  notes: string;
  language: string;
  institute: string;
  department: string;
  studentName: string;
  disciplineCode: string;
  teacherName: string;
  attachments: FileMeta[];
  contactName: string;
  consent: boolean;
  promoCode: string | null;
}

export interface CreateOrderResult extends Order {
  promoDropReason?: string;
}

export interface PromoCheck {
  ok: boolean;
  promo?: { code: string; discountPct: number };
  code?: string;
  reason?: string;
}

export interface Review {
  id: string;
  orderNumber: string;
  author: string;
  institute: string;
  department: string;
  discipline: string;
  product: string;
  rating: number;
  text: string;
  createdAt: string;
  status: string;
}

export interface ReviewInput {
  orderNumber: string;
  author: string;
  institute: string;
  department: string;
  discipline: string;
  product: string;
  rating: number;
  text: string;
}

/** Public settings subset returned by GET /api/settings. */
export interface PublicSettings {
  requisites?: Requisites;
  paymentQr?: string | null;
  /** Kaspi QR deep link behind the «Оплатить через Kaspi» button. */
  paymentLink?: string;
  paymentRecipient: string;
  supportTelegram: string;
  maintenance: boolean;
  queuePaused: boolean;
}

export interface Requisites {
  recipient: string;
  bank: string;
  card: string;
  iban: string;
  note: string;
}

/** Admin settings (GET/PUT /api/admin/settings). */
export interface Settings extends PublicSettings {
  requisites: Requisites;
  paymentQr: string | null;
}

export interface PricingProduct {
  code: ProductCode;
  min: number;
  max: number;
  unitPrice: number;
  active: boolean;
}

export interface VolumeDiscount {
  min: number;
  pct: number;
}

export interface PricingConfig {
  products: PricingProduct[];
  discounts: VolumeDiscount[];
}

export interface PromoCode {
  id: string;
  code: string;
  discountPct: number;
  appliesTo: 'first' | 'any';
  products: ProductCode[];
  minAmount: number | null;
  minUnits: number | null;
  usageLimit: number | null;
  usedCount: number;
  expiresAt: string | null;
  active: boolean;
  createdAt: string;
}

export interface PromoInput {
  code: string;
  discountPct: number;
  appliesTo: 'first' | 'any';
  products: ProductCode[];
  minAmount: number | null;
  minUnits: number | null;
  usageLimit: number | null;
  expiresAt: string | null;
  active: boolean;
}

export interface AdminStats {
  ordersByStatus: Record<string, number>;
  ordersTotal: number;
  confirmedRevenue: number;
  revenueToday: number;
  completedToday: number;
  queueDepth: number;
  oldestQueuedMinutes: number;
  queueWarning: boolean;
  averageGenerationSeconds: number;
  failureRatePct: number;
  diskUsedBytes: number;
  pendingReviews: number;
  activePromos: number;
  /** Generation spend (billed in USD) with the tenge equivalent at the current rate. */
  agentCostUsd: number;
  agentCostKzt: number;
  usdRate: number;
  netRevenue: number;
  generatedAt: string;
}

export interface ConsoleLine {
  at?: string;
  kind: 'iteration' | 'text' | 'reasoning' | 'tool' | 'usage' | 'error' | 'result' | 'event';
  text: string;
}

export interface OrderConsole {
  number: string;
  status: OrderStatus;
  attempt: number;
  jobStatus: string;
  startedAt?: string;
  finishedAt?: string;
  updatedAt?: string;
  lines: ConsoleLine[];
  truncated: boolean;
  /** Relative signed link to the standalone log viewer (opened in a browser). */
  logUrl?: string;
  /** What the runs of this order have cost so far, USD and tenge. */
  costUsd?: number;
  costKzt?: number;
}

export interface OrderListPage {
  orders: Order[];
  counts: Record<string, number>;
  total: number;
  page: number;
  limit: number;
}

export interface ReviewListPage {
  reviews: Review[];
  counts: Record<string, number>;
}

export interface ClientSession {
  chatId: number;
  username: string;
  since: string;
  /** Present when the chat is bound to an operator (the panel is available). */
  operatorId?: string;
  operatorName?: string;
}

export interface APIErrorPayload {
  code: string;
  message: string;
  fields?: Record<string, string>;
}
