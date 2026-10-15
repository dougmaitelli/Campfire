import { navigation } from "../../routing/navigation";
import type { NavigationItem } from "../../routing/navigation";
import { NavigationIcon } from "./NavigationIcon";
import { useState } from "react";
import { useAuthentication } from "../../hooks/useAuthentication";

export function Sidebar({ page }: { page: NavigationItem }) {
  const auth = useAuthentication();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
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
        {navigation.map((item) => (
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
        {error && <span role="alert">{error}</span>}
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
