import { useEffect, useMemo, useRef } from 'react';
import { Navigate, Route, Routes, useLocation, useNavigate } from 'react-router-dom';
import { bootstrapTelegram, initData, startParam, tg } from '@/telegram/sdk';
import { initTheme } from '@/app/theme';
import { useOrderStart, useScrollTop } from '@/lib/hooks';
import { SessionProvider } from '@/app/session';
import { HomeScreen } from '@/screens/HomeScreen';
import { WizardScreen } from '@/screens/Wizard/WizardScreen';
import { OrderScreen } from '@/screens/OrderScreen';
import { MyOrdersScreen } from '@/screens/MyOrdersScreen';
import { TemplatesScreen } from '@/screens/TemplatesScreen';
import { ReviewsScreen } from '@/screens/ReviewsScreen';
import { SupportScreen } from '@/screens/SupportScreen';
import { LegalScreen } from '@/screens/LegalScreen';
import { ProfileScreen } from '@/screens/ProfileScreen';
import { GateScreen } from '@/screens/GateScreen';
import { OperatorHubScreen } from '@/screens/operator/OperatorHubScreen';
import { OperatorOrdersScreen } from '@/screens/operator/OperatorOrdersScreen';
import { OperatorOrderScreen } from '@/screens/operator/OperatorOrderScreen';
import { OperatorReviewsScreen } from '@/screens/operator/OperatorReviewsScreen';
import { OperatorPromosScreen } from '@/screens/operator/OperatorPromosScreen';
import { OperatorPricingScreen } from '@/screens/operator/OperatorPricingScreen';
import { OperatorStatsScreen } from '@/screens/operator/OperatorStatsScreen';
import { OperatorSettingsScreen } from '@/screens/operator/OperatorSettingsScreen';

/** Deep link routing: startapp=order_DG-260391, new, orders, support, review_DG-… */
function useStartParamRouting(): void {
  const navigate = useNavigate();
  const param = useMemo(() => startParam(), []);
  const startOrder = useOrderStart();
  const handled = useRef(false);

  useEffect(() => {
    if (!param || handled.current) return;
    // Read once, then replace so a reload does not bounce the user back.
    handled.current = true;
    if (param === 'new') {
      void startOrder();
    } else if (param === 'orders') {
      navigate('/orders', { replace: true });
    } else if (param === 'support') {
      navigate('/support', { replace: true });
    } else if (param.startsWith('order_')) {
      navigate(`/order/${param.slice('order_'.length)}`, { replace: true });
    } else if (param.startsWith('review_')) {
      navigate(`/reviews?order=${param.slice('review_'.length)}`, { replace: true });
    }
  }, [param, navigate, startOrder]);
}

export function App() {
  useEffect(() => {
    tg?.ready();
    tg?.expand();
    const stopTheme = initTheme();
    const stopTelegram = bootstrapTelegram();
    return () => {
      stopTheme();
      stopTelegram();
    };
  }, []);

  // Outside Telegram the app explains itself instead of failing on 401s.
  if (!initData()) {
    return <GateScreen />;
  }

  return <Shell />;
}

/** Every screen opens at the top: without this a Mini App keeps the previous
 * scroll position when the route changes (e.g. Home → «Заказать»). */
function ScrollToTop() {
  const { pathname } = useLocation();
  useScrollTop(pathname);
  return null;
}

function Shell() {
  useStartParamRouting();
  return (
    <SessionProvider>
      <ScrollToTop />
      <div className="flex min-h-screen flex-col bg-page">
        <Routes>
          <Route path="/" element={<HomeScreen />} />
          <Route path="/new" element={<WizardScreen />} />
          <Route path="/order/:number" element={<OrderScreen />} />
          <Route path="/orders" element={<MyOrdersScreen />} />
          <Route path="/templates" element={<TemplatesScreen />} />
          <Route path="/reviews" element={<ReviewsScreen />} />
          <Route path="/support" element={<SupportScreen />} />
          <Route path="/legal/:doc" element={<LegalScreen />} />
          <Route path="/legal" element={<Navigate to="/legal/offer" replace />} />
          <Route path="/profile" element={<ProfileScreen />} />

          {/* Operator panel (available when the chat is bound to an operator). */}
          <Route path="/operator" element={<OperatorHubScreen />} />
          <Route path="/operator/orders" element={<OperatorOrdersScreen />} />
          <Route path="/operator/orders/:number" element={<OperatorOrderScreen />} />
          <Route path="/operator/reviews" element={<OperatorReviewsScreen />} />
          <Route path="/operator/promos" element={<OperatorPromosScreen />} />
          <Route path="/operator/pricing" element={<OperatorPricingScreen />} />
          <Route path="/operator/stats" element={<OperatorStatsScreen />} />
          <Route path="/operator/settings" element={<OperatorSettingsScreen />} />

          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </div>
    </SessionProvider>
  );
}
