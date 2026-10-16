import { useEffect, useId, useRef, useState } from "react";
import { useAuthentication } from "../../hooks/useAuthentication";
import { linkOIDC } from "../../api/auth";
import { APITokensDialog } from "../../dialogs/APITokensDialog";
import { NavigationIcon } from "./NavigationIcon";
import "./account-menu.css";

export function AccountMenu() {
  const auth = useAuthentication();
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [tokensOpen, setTokensOpen] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const container = useRef<HTMLDivElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const panelID = useId();
  const username = auth.session?.username ?? "Account";
  const displayName = auth.session?.name?.trim() ?? "";
  const name = displayName === "" ? username : displayName;
  const initials = name
    .split(/\s+/)
    .map((part) => Array.from(part)[0])
    .slice(0, 2)
    .join("")
    .toUpperCase();
  const [failedPicture, setFailedPicture] = useState<string | null>(null);
  const picture = auth.session?.picture;
  const oidcError = new URLSearchParams(window.location.search).has(
    "oidc_error",
  );

  useEffect(() => {
    if (!open) return;
    container.current
      ?.querySelector<HTMLButtonElement>(".account-dropdown button")
      ?.focus();
    function outside(event: PointerEvent) {
      if (
        event.target instanceof Node &&
        !container.current?.contains(event.target)
      ) {
        setOpen(false);
      }
    }
    function escape(event: KeyboardEvent) {
      if (event.key === "Escape") {
        setOpen(false);
        trigger.current?.focus();
      }
    }
    document.addEventListener("pointerdown", outside);
    document.addEventListener("keydown", escape);
    return () => {
      document.removeEventListener("pointerdown", outside);
      document.removeEventListener("keydown", escape);
    };
  }, [open]);

  async function perform(action: () => Promise<void>) {
    if (busy) return;
    setOpen(false);
    setBusy(true);
    setError(null);
    try {
      await action();
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : "Unable to update account.",
      );
    } finally {
      setBusy(false);
    }
  }

  return (
    <div
      className="sidebar-footer account-menu"
      ref={container}
      onBlur={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget)) setOpen(false);
      }}
    >
      {open && (
        <div
          id={panelID}
          className="account-dropdown"
          aria-label="Account options"
        >
          <button
            type="button"
            onClick={() => {
              setOpen(false);
              setTokensOpen(true);
            }}
          >
            <NavigationIcon name="key" />
            API tokens
          </button>
          {auth.session?.oidc_enabled && (
            <button
              type="button"
              onClick={() => {
                void perform(linkOIDC);
              }}
            >
              <NavigationIcon name="link" />
              Link OpenID Connect
            </button>
          )}
        </div>
      )}
      <div className="account-summary">
        <button
          type="button"
          className="account-trigger"
          ref={trigger}
          aria-label={`Account options for ${name}`}
          aria-expanded={open}
          aria-controls={open ? panelID : undefined}
          disabled={busy}
          onClick={() => setOpen((value) => !value)}
        >
          {picture && picture !== failedPicture ? (
            <img
              className="account-avatar"
              src={picture}
              alt=""
              referrerPolicy="no-referrer"
              onError={() => setFailedPicture(picture)}
            />
          ) : (
            <span className="account-avatar" aria-hidden="true">
              {initials}
            </span>
          )}
          <span className="account-identity">
            <strong>{name}</strong>
            <small>{auth.session?.role}</small>
          </span>
          <NavigationIcon name="chevron" />
        </button>
        <button
          type="button"
          className="account-logout"
          aria-label="Sign out"
          title="Sign out"
          disabled={busy}
          onClick={() => {
            void perform(auth.signOut);
          }}
        >
          <NavigationIcon name="logout" />
        </button>
      </div>
      {error && (
        <p className="account-error" role="alert">
          {error}
        </p>
      )}
      {oidcError && (
        <p className="account-error" role="alert">
          OpenID Connect failed. The identity may already belong to another
          account. Try again or contact your administrator.
        </p>
      )}
      {tokensOpen && <APITokensDialog onClose={() => setTokensOpen(false)} />}
    </div>
  );
}
