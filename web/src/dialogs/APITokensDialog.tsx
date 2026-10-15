import { useEffect, useRef, useState } from "react";
import {
  createToken,
  listTokens,
  revokeToken,
  type APIToken,
} from "../api/tokens";
import { Drawer } from "../components/layout/Drawer";

export function APITokensDialog({ onClose }: { onClose: () => void }) {
  const [tokens, setTokens] = useState<APIToken[]>([]);
  const [choices, setChoices] = useState<string[]>([]);
  const [scopes, setScopes] = useState<string[]>(["*"]);
  const [name, setName] = useState("");
  const [days, setDays] = useState(90);
  const [secret, setSecret] = useState("");
  const [copied, setCopied] = useState(false);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const pending = useRef(false);
  useEffect(() => {
    const controller = new AbortController();
    void listTokens(controller.signal)
      .then((result) => {
        if (!controller.signal.aborted) {
          setTokens(result.tokens);
          setChoices(result.scopes);
        }
      })
      .catch((cause: unknown) => {
        if (!controller.signal.aborted)
          setError(
            cause instanceof Error
              ? cause.message
              : "Unable to load API tokens.",
          );
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, []);

  async function perform(action: () => Promise<void>) {
    if (pending.current) return;
    pending.current = true;
    setBusy(true);
    setError(null);
    try {
      await action();
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : "Unable to update API tokens.",
      );
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }
  function toggle(scope: string, checked: boolean) {
    setScopes((current) => {
      let next = checked
        ? [...current, scope]
        : current.filter((item) => item !== scope);
      if (
        checked &&
        scope.startsWith("server.") &&
        !next.includes("server.view")
      )
        next.push("server.view");
      if (!checked && scope === "server.view")
        next = next.filter((item) => !item.startsWith("server."));
      return next;
    });
  }
  return (
    <Drawer
      title="API tokens"
      description="Tokens act as your user. Scopes can restrict your permissions but never grant additional access."
      busy={busy}
      onClose={onClose}
      footerLeft={
        <button className="button" disabled={busy} onClick={onClose}>
          Close
        </button>
      }
      footerRight={null}
    >
      <div className="drawer-content">
        {error && (
          <p className="error-message" role="alert">
            {error}
          </p>
        )}
        {secret && (
          <section aria-label="New token secret">
            <h3>Save your token</h3>
            <p>
              This secret is shown only once. Store it somewhere safe before
              closing this window.
            </p>
            <input
              aria-label="Token secret"
              value={secret}
              readOnly
              spellCheck={false}
              autoComplete="off"
            />
            <button
              className="button"
              disabled={busy}
              onClick={() => {
                void perform(async () => {
                  await navigator.clipboard.writeText(secret);
                  setCopied(true);
                });
              }}
            >
              {copied ? "Copied" : "Copy token"}
            </button>
          </section>
        )}
        <h3>Create a token</h3>
        <form
          onSubmit={(event) => {
            event.preventDefault();
            void perform(async () => {
              const result = await createToken(name, days, scopes);
              setTokens((current) => [result.token, ...current]);
              setSecret(result.secret);
              setCopied(false);
              setName("");
            });
          }}
        >
          <label htmlFor="token-name">Name</label>
          <input
            id="token-name"
            required
            maxLength={100}
            value={name}
            disabled={busy}
            onChange={(event) => setName(event.target.value)}
          />
          <label htmlFor="token-expiry">Expires in days</label>
          <input
            id="token-expiry"
            type="number"
            min={1}
            max={365}
            required
            value={days}
            disabled={busy}
            onChange={(event) => setDays(Number(event.target.value))}
          />
          <label className="checkbox-field">
            <input
              type="checkbox"
              checked={scopes.includes("*")}
              disabled={busy}
              onChange={(event) =>
                setScopes(event.target.checked ? ["*"] : ["server.view"])
              }
            />
            All my current permissions
          </label>
          {!scopes.includes("*") &&
            choices
              .filter((scope) => scope !== "*")
              .map((scope) => (
                <label key={scope} className="checkbox-field">
                  <input
                    type="checkbox"
                    checked={scopes.includes(scope)}
                    disabled={busy}
                    onChange={(event) => toggle(scope, event.target.checked)}
                  />
                  {scope}
                </label>
              ))}
          <p className="field-help">
            Server access follows your ownership and assignments. Account
            password, role, or disabled-state changes invalidate existing
            tokens.
          </p>
          <button
            className="button button-primary"
            disabled={
              busy ||
              loading ||
              !choices.length ||
              !name.trim() ||
              !scopes.length ||
              !!secret
            }
          >
            Create token
          </button>
          {secret && (
            <p>Close and reopen this window to create another token.</p>
          )}
        </form>
        <h3>Active tokens</h3>
        {loading ? (
          <p>Loading…</p>
        ) : (
          !tokens.length && <p>No active API tokens.</p>
        )}
        {tokens.map((token) => (
          <section key={token.id} aria-label={token.name}>
            <h4>{token.name}</h4>
            <p>
              <code>{token.prefix}…</code> · Expires{" "}
              {new Date(token.expires_at * 1000).toLocaleDateString()}
            </p>
            <p>
              Last used:{" "}
              {token.last_used_at
                ? new Date(token.last_used_at * 1000).toLocaleString()
                : "Never"}
            </p>
            <p>
              Scopes:{" "}
              {token.scopes.includes("*")
                ? "All my current permissions"
                : token.scopes.join(", ")}
            </p>
            <button
              className="button"
              disabled={busy}
              onClick={() => {
                if (
                  !window.confirm(
                    `Revoke “${token.name}”? Applications using it will lose access immediately.`,
                  )
                )
                  return;
                void perform(async () => {
                  await revokeToken(token.id);
                  setTokens((current) =>
                    current.filter((item) => item.id !== token.id),
                  );
                });
              }}
            >
              Revoke
            </button>
          </section>
        ))}
      </div>
    </Drawer>
  );
}
