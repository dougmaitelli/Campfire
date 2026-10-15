import { useRef, useState } from "react";
import { NavigationIcon } from "../components/navigation/NavigationIcon";
import { useAuthentication } from "../hooks/useAuthentication";
import "./authentication.css";

export function AuthenticationPage() {
  const auth = useAuthentication();
  const [username, setUsername] = useState("admin");
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [token, setToken] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const pending = useRef(false);
  const setup = auth.session?.setup_required === true;
  const localLogin = auth.session?.local_login !== false;
  const oidcError = new URLSearchParams(window.location.search).has(
    "oidc_error",
  );
  async function submit() {
    if (pending.current) return;
    if (setup && password !== confirmation) {
      setError("The passwords do not match.");
      return;
    }
    pending.current = true;
    setBusy(true);
    setError(null);
    try {
      await auth.signIn(
        { username, password, ...(setup ? { setup_token: token.trim() } : {}) },
        setup,
      );
      setPassword("");
      setConfirmation("");
      setToken("");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Unable to sign in.");
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }
  return (
    <main className="authentication-page">
      <section className="authentication-card" aria-labelledby="auth-title">
        <div className="authentication-brand">
          <NavigationIcon name="campfire" />
          <span>Campfire</span>
        </div>
        <h1 id="auth-title">
          {!auth.session
            ? "Welcome to Campfire"
            : setup
              ? "Set up your administrator"
              : "Sign in"}
        </h1>
        {auth.error ? (
          <>
            <p className="error-message" role="alert">
              {auth.error}
            </p>
            <button
              className="button"
              onClick={() => {
                void auth.reload();
              }}
            >
              Retry
            </button>
          </>
        ) : !auth.session ? (
          <p role="status">Connecting…</p>
        ) : (
          <form
            className="authentication-form"
            onSubmit={(event) => {
              event.preventDefault();
              void submit();
            }}
          >
            <p className="authentication-description">
              {setup
                ? "Create the administrator account to manage your game servers."
                : "Sign in to access your game servers."}
            </p>
            {(error !== null || oidcError) && (
              <p className="error-message" role="alert">
                {error ??
                  "OpenID Connect sign-in failed. Please try again or contact your administrator."}
              </p>
            )}
            {!setup && auth.session.oidc_enabled && (
              <a
                className="button button-primary"
                href="/api/v1/auth/oidc/start"
              >
                Sign in with OpenID Connect
              </a>
            )}
            {localLogin && (
              <>
                {setup && (
                  <label>
                    Setup token
                    <input
                      type="password"
                      name="setup_token"
                      autoComplete="off"
                      value={token}
                      maxLength={128}
                      required
                      disabled={busy}
                      onChange={(event) => setToken(event.target.value)}
                      aria-describedby="setup-token-help"
                    />
                    <span id="setup-token-help" className="authentication-hint">
                      Read the private .setup-token file beside the panel
                      database on your Campfire host.
                    </span>
                  </label>
                )}
                <label>
                  Username
                  <input
                    autoComplete="username"
                    name="username"
                    autoCapitalize="none"
                    spellCheck={false}
                    value={username}
                    minLength={setup ? 3 : 1}
                    maxLength={64}
                    required
                    disabled={busy}
                    onChange={(event) => setUsername(event.target.value)}
                  />
                </label>
                <label>
                  Password
                  <input
                    type="password"
                    autoComplete={setup ? "new-password" : "current-password"}
                    name="password"
                    value={password}
                    minLength={setup ? 15 : 1}
                    maxLength={1024}
                    required
                    disabled={busy}
                    onChange={(event) => setPassword(event.target.value)}
                  />
                </label>
                {setup && (
                  <label>
                    Confirm password
                    <input
                      type="password"
                      autoComplete="new-password"
                      name="confirmation"
                      value={confirmation}
                      minLength={15}
                      maxLength={1024}
                      required
                      disabled={busy}
                      onChange={(event) => setConfirmation(event.target.value)}
                    />
                    <span className="authentication-hint">
                      Use at least 15 characters. Spaces and passphrases are
                      welcome.
                    </span>
                  </label>
                )}
                <button className="button button-primary" disabled={busy}>
                  {busy
                    ? "Please wait…"
                    : setup
                      ? "Create administrator"
                      : "Sign in"}
                </button>
                {!setup && (
                  <p className="authentication-hint">
                    Forgot your password? Use the reset-password command on your
                    Campfire host.
                  </p>
                )}
              </>
            )}
          </form>
        )}
      </section>
    </main>
  );
}
