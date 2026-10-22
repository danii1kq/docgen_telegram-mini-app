import type { FileMeta, OrderForm, ProductCode } from '@/api/types';
import { deviceStorage } from '@/telegram/sdk';

/**
 * Wizard draft persistence (design.md §12.6): the draft survives app restarts,
 * is cleared on a successful order, and drives the closing confirmation.
 */

const DRAFT_KEY = 'srunal.mini.draft';

export interface Draft {
  product: ProductCode;
  units: number;
  works: number;
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
  promoCode: string;
  step: number;
  savedAt: number;
}

export const EMPTY_DRAFT: Draft = {
  product: 'essay',
  units: 10,
  works: 1,
  workKind: 'СРС',
  topics: '',
  notes: '',
  language: 'ru',
  institute: 'ИЦИТ',
  department: '',
  studentName: '',
  disciplineCode: '',
  teacherName: '',
  attachments: [],
  contactName: '',
  consent: false,
  promoCode: '',
  step: 1,
  savedAt: 0,
};

export function loadDraft(): Draft {
  const raw = deviceStorage.get(DRAFT_KEY);
  if (!raw) return { ...EMPTY_DRAFT };
  try {
    const parsed = JSON.parse(raw) as Partial<Draft>;
    return { ...EMPTY_DRAFT, ...parsed, attachments: parsed.attachments ?? [] };
  } catch {
    return { ...EMPTY_DRAFT };
  }
}

export function saveDraft(draft: Draft): void {
  deviceStorage.set(DRAFT_KEY, JSON.stringify({ ...draft, savedAt: Date.now() }));
}

export function clearDraft(): void {
  deviceStorage.remove(DRAFT_KEY);
}

/** A draft is "dirty" once the client typed something worth keeping. */
export function isDirty(draft: Draft): boolean {
  return Boolean(
    draft.topics.trim() ||
      draft.notes.trim() ||
      draft.department.trim() ||
      draft.studentName.trim() ||
      draft.disciplineCode.trim() ||
      draft.teacherName.trim() ||
      draft.contactName.trim() ||
      draft.attachments.length > 0 ||
      draft.promoCode.trim() ||
      draft.consent ||
      draft.works > 1 ||
      draft.product !== EMPTY_DRAFT.product ||
      draft.units !== EMPTY_DRAFT.units,
  );
}

/** Draft → the API payload (frozen at creation server-side). */
export function toOrderForm(draft: Draft, promoCode: string | null): OrderForm {
  return {
    product: draft.product,
    works: draft.works,
    units: draft.units,
    workKind: draft.workKind,
    topics: draft.topics.trim(),
    notes: draft.notes.trim(),
    language: draft.language,
    institute: draft.institute.trim(),
    department: draft.department.trim(),
    studentName: draft.studentName.trim(),
    disciplineCode: draft.disciplineCode.trim(),
    teacherName: draft.teacherName.trim(),
    attachments: draft.attachments,
    contactName: draft.contactName.trim(),
    consent: draft.consent,
    promoCode,
  };
}

/**
 * Preset academic profiles ("Шаблоны данных" on wizard step 3 and the "Мои
 * шаблоны" screen): the saved sets are kept on the device so a returning client
 * fills the title page in one tap.
 */
export interface AcademicPreset {
  name: string;
  institute: string;
  department: string;
  studentName: string;
  disciplineCode: string;
  teacherName: string;
}

const PRESETS_KEY = 'srunal.mini.presets';

/** Hard cap: the templates screen lists them all, more becomes unusable. */
export const MAX_PRESETS = 10;

export function loadPresets(): AcademicPreset[] {
  const raw = deviceStorage.get(PRESETS_KEY);
  if (!raw) return [];
  try {
    const parsed = JSON.parse(raw) as AcademicPreset[];
    return Array.isArray(parsed) ? parsed.slice(0, MAX_PRESETS) : [];
  } catch {
    return [];
  }
}

export function savePreset(preset: AcademicPreset): AcademicPreset[] {
  const presets = loadPresets().filter((item) => item.name !== preset.name);
  presets.unshift(preset);
  const trimmed = presets.slice(0, MAX_PRESETS);
  deviceStorage.set(PRESETS_KEY, JSON.stringify(trimmed));
  return trimmed;
}

export function removePreset(name: string): AcademicPreset[] {
  const presets = loadPresets().filter((item) => item.name !== name);
  deviceStorage.set(PRESETS_KEY, JSON.stringify(presets));
  return presets;
}

/** Copies the academic data of a preset into the stored draft (optionally
 * pointing it at step 3) - used by the "Мои шаблоны" screen. */
export function applyPresetToDraft(preset: AcademicPreset, step = 3): Draft {
  const draft: Draft = {
    ...loadDraft(),
    institute: preset.institute,
    department: preset.department,
    studentName: preset.studentName,
    disciplineCode: preset.disciplineCode,
    teacherName: preset.teacherName,
    step,
  };
  saveDraft(draft);
  return draft;
}
