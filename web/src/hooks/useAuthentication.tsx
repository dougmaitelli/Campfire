import i18n from "../i18n";
import { useTranslation } from "react-i18next";
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
} from "react";
import type { ReactNode } from "react";
import {
  authenticate,
  logout,
  readSession,
  type AuthState,
  type Credentials,
} from "../api/auth";
import { setCSRFToken } from "../api/client";

interface Authentication {
  session: AuthState | null;
  error: string | null;
  signIn: (credentials: Credentials, setup: boolean) => Promise<void>;
  signOut: () => Promise<void>;
  reload: () => Promise<void>;
}

const AuthenticationContext = createContext<Authentication | null>(null);

export function AuthenticationProvider({ children }: { children: ReactNode }) {
  useTranslation();
  const [session, setSession] = useState<AuthState | null>(null);
  const [error, setError] = useState<string | null>(null);
  const generation = useRef(0);
  const accept = useCallback((next: AuthState) => {
    generation.current += 1;
    setCSRFToken(next.csrf_token ?? "");
    setSession((previous) => ({
      ...next,
      local_login: next.local_login ?? previous?.local_login ?? true,
      oidc_enabled: next.oidc_enabled ?? previous?.oidc_enabled ?? false,
    }));
    setError(null);
  }, []);
  const load = useCallback(
    (signal: AbortSignal) => {
      const current = generation.current;
      return readSession(signal)
        .then((next) => {
          if (!signal.aborted && current === generation.current) accept(next);
        })
        .catch((cause: unknown) => {
          const timedOut =
            signal.reason instanceof DOMException &&
            signal.reason.name === "TimeoutError";
          if ((!signal.aborted || timedOut) && current === generation.current)
            setError(
              timedOut
                ? i18n.t(
                    "useAuthentication.theConnectionTimedOutRetryToConnectToCampfire",
                  )
                : cause instanceof Error
                  ? cause.message
                  : i18n.t("useAuthentication.unableToConnectToCampfire"),
            );
        });
    },
    [accept],
  );
  useEffect(() => {
    const controller = new AbortController();
    const expired = () => {
      accept({ authenticated: false, setup_required: false });
      void load(AbortSignal.timeout(15000));
    };
    window.addEventListener("campfire:unauthenticated", expired);
    void load(AbortSignal.any([controller.signal, AbortSignal.timeout(15000)]));
    return () => {
      controller.abort();
      window.removeEventListener("campfire:unauthenticated", expired);
    };
  }, [accept, load]);
  async function signIn(credentials: Credentials, setup: boolean) {
    accept(await authenticate(credentials, setup));
  }
  async function signOut() {
    await logout();
    accept({ authenticated: false, setup_required: false });
    await load(AbortSignal.timeout(15000));
  }
  async function reload() {
    setError(null);
    await load(AbortSignal.timeout(15000));
  }
  return (
    <AuthenticationContext.Provider
      value={{ session, error, signIn, signOut, reload }}
    >
      {children}
    </AuthenticationContext.Provider>
  );
}

export function useAuthentication(): Authentication {
  const value = useContext(AuthenticationContext);
  if (!value)
    throw new Error(
      i18n.t("useAuthentication.authenticationProviderIsMissing"),
    );
  return value;
}
