import { useEffect, useRef } from 'react';
import { tg, haptic, hasNativeButtons } from './sdk';

/**
 * Telegram MainButton bound to a React component. Inside Telegram it drives
 * the native button; in a plain browser it renders an inline sticky button
 * with the same look so the flow stays testable outside the messenger.
 *
 * Always offClick before onClick: Telegram keeps handlers across screens
 * (skill: "MainButton not responding").
 */
export function MainButton({
  text,
  onClick,
  disabled = false,
  progress = false,
  visible = true,
}: {
  text: string;
  onClick: () => void;
  disabled?: boolean;
  progress?: boolean;
  visible?: boolean;
}) {
  const handler = useRef(onClick);
  handler.current = onClick;
  const native = hasNativeButtons();

  useEffect(() => {
    if (!native) return;
    const button = tg?.MainButton;
    if (!button) return;
    const listener = () => handler.current();
    button.setText(text);
    button.offClick(listener);
    button.onClick(listener);
    return () => {
      button.offClick(listener);
    };
  }, [text, native]);

  useEffect(() => {
    if (!native) return;
    const button = tg?.MainButton;
    if (!button) return;
    if (!visible) {
      button.hide();
      return;
    }
    button.show();
    if (disabled || progress) {
      button.disable();
    } else {
      button.enable();
    }
    if (progress) {
      button.showProgress(false);
    } else {
      button.hideProgress();
    }
  }, [disabled, progress, visible, native]);

  useEffect(() => {
    if (!native) return;
    // Never leave a stale button behind when the screen unmounts.
    const button = tg?.MainButton;
    return () => {
      button?.hideProgress();
      button?.hide();
    };
  }, [native]);

  if (native || !visible) return null;

  // Browser fallback: the same sticky CTA the design uses.
  return (
    <div className="fixed inset-x-0 bottom-0 z-40 bg-page/90 backdrop-blur-xl" style={{ paddingBottom: 'var(--safe-bottom)' }}>
      <div className="mx-auto w-full max-w-[480px] px-4 py-2">
        <button
          type="button"
          disabled={disabled || progress}
          onClick={() => {
            haptic.impact();
            onClick();
          }}
          className="flex h-12 w-full items-center justify-center gap-2 rounded-xl bg-accent text-button text-btn-text shadow-[0_4px_12px_rgba(36,129,204,0.25)] transition active:scale-[0.98] disabled:opacity-50"
        >
          {progress ? <span className="size-4 animate-spin rounded-full border-2 border-white/40 border-t-white" /> : null}
          {text}
        </button>
      </div>
    </div>
  );
}

/** Native back button: visible on every screen except Home. */
export function BackButton({ visible = true, onClick }: { visible?: boolean; onClick: () => void }) {
  const handler = useRef(onClick);
  handler.current = onClick;
  const native = hasNativeButtons();

  useEffect(() => {
    if (!native) return;
    const button = tg?.BackButton;
    if (!button) return;
    const listener = () => handler.current();
    button.offClick(listener);
    button.onClick(listener);
    return () => {
      button.offClick(listener);
    };
  }, [native]);

  useEffect(() => {
    if (!native) return;
    const button = tg?.BackButton;
    if (!button) return;
    if (visible) {
      button.show();
    } else {
      button.hide();
    }
    return () => {
      button.hide();
    };
  }, [visible, native]);

  return null;
}
