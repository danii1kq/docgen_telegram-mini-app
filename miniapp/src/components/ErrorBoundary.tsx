import { Component, type ErrorInfo, type ReactNode } from 'react';
import { translateText } from '@/i18n';
import { detectLang } from '@/i18n/core';

interface State {
  error: unknown;
}

/** Keeps a runtime failure from leaving a blank WebView. Sits above the I18n
 * provider, so it translates through the plain helper. */
export class ErrorBoundary extends Component<{ children: ReactNode }, State> {
  state: State = { error: undefined };

  static getDerivedStateFromError(error: unknown): State {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo): void {
    console.error('miniapp crash', error, info.componentStack);
  }

  render(): ReactNode {
    if (this.state.error === undefined) {
      return this.props.children;
    }
    const lang = detectLang();
    const t = (source: string) => translateText(lang, source);
    return (
      <div className="flex min-h-screen flex-col items-center justify-center gap-3 px-8 text-center">
        <span className="text-headline-md text-ink">{t('Что-то пошло не так')}</span>
        <span className="text-body-secondary text-ink-2">
          {t('Закройте приложение и откройте заново - или напишите в поддержку.')}
        </span>
        <button
          type="button"
          onClick={() => this.setState({ error: undefined })}
          className="mt-2 h-11 rounded-xl bg-accent-soft px-5 text-body-secondary-strong text-accent"
        >
          {t('Попробовать снова')}
        </button>
      </div>
    );
  }
}
