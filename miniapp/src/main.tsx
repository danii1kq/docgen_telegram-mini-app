import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { BrowserRouter } from 'react-router-dom';
import { App } from './App';
import { applyThemeMode } from './app/theme';
import { ErrorBoundary } from './components/ErrorBoundary';
import { ToastProvider } from './components/Toast';
import { I18nProvider } from './i18n';
import './index.css';

// Applied before the first paint: a stored light/dark choice must not flash the
// Telegram scheme first (the provider then keeps format/labels in sync).
applyThemeMode();

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ErrorBoundary>
      <I18nProvider>
        <ToastProvider>
          <BrowserRouter>
            <App />
          </BrowserRouter>
        </ToastProvider>
      </I18nProvider>
    </ErrorBoundary>
  </StrictMode>,
);
