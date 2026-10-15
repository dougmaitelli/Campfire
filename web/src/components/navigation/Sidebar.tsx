import { navigation } from "../../routing/navigation";
import type { NavigationItem } from "../../routing/navigation";
import { NavigationIcon } from "./NavigationIcon";
import { useState } from "react";
import { useAuthentication } from "../../hooks/useAuthentication";
import { linkOIDC } from "../../api/auth";

export function Sidebar({ page }: { page: NavigationItem }) {
  const auth = useAuthentication();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const oidcError = new URLSearchParams(window.location.search).has(
    "oidc_error",
  );
  async function signOut() {
    if (busy) return;
    setBusy(true);
    setError(null);
    try {
      await auth.signOut();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Unable to sign out.");
    } finally {
      setBusy(false);
    }
  }
  return (
    <aside className="sidebar">
      <a className="brand" href="#dashboard" aria-label="Campfire dashboard">
        <span className="brand-mark">
          <NavigationIcon name="campfire" />
        </span>
        <span>Campfire</span>
      </a>
      <nav className="navigation" aria-label="Main navigation">
        {navigation
          .filter(
            (item) =>
              auth.session?.admin === true ||
              item.id === "servers" ||
              item.id === "dashboard" ||
              item.id === "schedules",
          )
          .map((item) => (
            <a
              key={item.id}
              href={`#${item.id}`}
              className="navigation-link"
              aria-current={page.id === item.id ? "page" : undefined}
            >
              <NavigationIcon name={item.icon} />
              <span>{item.label}</span>
            </a>
          ))}
      </nav>
      <div className="sidebar-footer sidebar-account">
        <span>{auth.session?.username}</span>
        <span>{auth.session?.role}</span>
        {error && <span role="alert">{error}</span>}
        {oidcError && (
          <span role="alert">
            OpenID Connect failed. The identity may already belong to another
            account. Try again or contact your administrator.
          </span>
        )}
        {auth.session?.oidc_enabled && (
          <button
            className="button"
            disabled={busy}
            onClick={() => {
              setBusy(true);
              void linkOIDC().catch((cause: unknown) => {
                setError(
                  cause instanceof Error
                    ? cause.message
                    : "Unable to link OpenID Connect.",
                );
                setBusy(false);
              });
            }}
          >
            Link OpenID Connect
          </button>
        )}
        <button
          className="button"
          disabled={busy}
          onClick={() => {
            void signOut();
          }}
        >
          {busy ? "Signing out…" : "Sign out"}
        </button>
      </div>
    </aside>
  );
}
