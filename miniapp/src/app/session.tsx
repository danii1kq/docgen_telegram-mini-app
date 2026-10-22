import { createContext, useContext, useMemo, type ReactNode } from 'react';
import { api } from '@/api/client';
import type { ClientSession } from '@/api/types';
import { useAsync } from '@/lib/hooks';

/**
 * The Telegram identity of the current session (/api/auth/me). When the chat is
 * bound to an operator the answer also carries the operator identity, which is
 * what unlocks the operator panel inside the Mini App.
 */
interface SessionState {
  session?: ClientSession;
  loading: boolean;
  reload: () => void;
}

const SessionContext = createContext<SessionState>({ loading: true, reload: () => {} });

export function SessionProvider({ children }: { children: ReactNode }) {
  const { data, loading, reload } = useAsync(() => api.me(), []);
  const value = useMemo(() => ({ session: data, loading, reload }), [data, loading, reload]);
  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}

export function useSession(): SessionState {
  return useContext(SessionContext);
}

/** The operator identity of the chat, when there is one. */
export function useOperator(): { id: string; name: string } | undefined {
  const { session } = useSession();
  if (!session?.operatorId) return undefined;
  return { id: session.operatorId, name: session.operatorName ?? session.operatorId };
}
