import i18n from "../i18n";
import { useTranslation } from "react-i18next";
import { FormField } from "../components/forms/FormField";
import { InlineMessage } from "../components/status/InlineMessage";
import { Button, ButtonLink } from "../components/forms/Button";
import { TextInput } from "../components/forms/TextInput";
import { useRef, useState, type ComponentProps } from "react";
import { ArrowRight, Eye, EyeOff } from "lucide-react";
import { CampfireMark } from "../components/navigation/CampfireMark";
import { useAuthentication } from "../hooks/useAuthentication";
import "./authentication.css";

function AuthenticationPasswordInput(props: ComponentProps<"input">) {
  const { t } = useTranslation();
  const [visible, setVisible] = useState(false);
  return (
    <div className="authentication-password">
      <TextInput {...props} type={visible ? "text" : "password"} />
      <Button
        type="button"
        iconOnly
        className="authentication-password-toggle"
        aria-label={t(
          visible
            ? "authenticationPage.hidePassword"
            : "authenticationPage.showPassword",
        )}
        aria-pressed={visible}
        disabled={props.disabled}
        onClick={() => setVisible(!visible)}
      >
        {visible ? <EyeOff aria-hidden="true" /> : <Eye aria-hidden="true" />}
      </Button>
    </div>
  );
}

export function AuthenticationPage() {
  const { t } = useTranslation();
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
      setError(i18n.t("authenticationPage.thePasswordsDoNotMatch"));
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
      setError(
        cause instanceof Error
          ? cause.message
          : i18n.t("authenticationPage.unableToSignIn"),
      );
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }
  return (
    <main className="authentication-page">
      <div className="authentication-card">
        <div className="authentication-introduction">
          <div className="authentication-brand">
            <CampfireMark />
            <span>{t("authenticationPage.campfire")}</span>
          </div>
          <div className="authentication-welcome">
            <p className="authentication-headline">
              {t("authenticationPage.aHomeForYourGameServers")}
            </p>
            <p className="authentication-tagline">
              {t("authenticationPage.bringYourServersTogether")}
            </p>
          </div>
        </div>
        <section
          className="authentication-content"
          aria-labelledby="auth-title"
        >
          <h1 id="auth-title">
            {!auth.session
              ? t("authenticationPage.welcomeToCampfire")
              : setup
                ? t("authenticationPage.setUpYourAdministrator")
                : t("authenticationPage.signIn")}
          </h1>
          {auth.error ? (
            <>
              <InlineMessage role="alert">{auth.error}</InlineMessage>
              <Button
                onClick={() => {
                  void auth.reload();
                }}
              >
                {t("authenticationPage.retry")}
              </Button>
            </>
          ) : !auth.session ? (
            <p role="status">{t("authenticationPage.connecting")}</p>
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
                  ? t(
                      "authenticationPage.createTheAdministratorAccountToManageYourGameServers",
                    )
                  : !localLogin && auth.session.oidc_enabled
                    ? t("authenticationPage.useYourOrganizationAccount")
                    : t("authenticationPage.signInToAccessYourGameServers")}
              </p>
              {(error !== null || oidcError) && (
                <InlineMessage role="alert">
                  {error ??
                    t(
                      "authenticationPage.openidConnectSignInFailedPleaseTryAgainOrContact",
                    )}
                </InlineMessage>
              )}
              {!setup && auth.session.oidc_enabled && (
                <ButtonLink variant="primary" href="/api/v1/auth/oidc/start">
                  {t("authenticationPage.signInWithOpenIDConnect")}
                  <ArrowRight size={18} aria-hidden="true" />
                </ButtonLink>
              )}
              {!setup &&
                auth.session.oidc_enabled &&
                (localLogin ? (
                  <div className="authentication-divider">
                    {t("authenticationPage.orUseYourUsername")}
                  </div>
                ) : (
                  <p className="authentication-hint">
                    {t("authenticationPage.continueToIdentityProvider")}
                  </p>
                ))}
              {localLogin && (
                <>
                  {setup && (
                    <FormField
                      label={t("authenticationPage.setupToken")}
                      description={
                        <>
                          <span
                            id="setup-token-help"
                            className="authentication-hint"
                          >
                            {t(
                              "authenticationPage.copyTheOneTimeSetupTokenFromCampfireSStartup",
                            )}
                          </span>
                        </>
                      }
                    >
                      <TextInput
                        type="password"
                        name="setup_token"
                        autoComplete="off"
                        value={token}
                        maxLength={128}
                        required
                        disabled={busy}
                        onChange={(event) => setToken(event.target.value)}
                      />
                    </FormField>
                  )}
                  <FormField label={t("authenticationPage.username")}>
                    <TextInput
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
                  </FormField>
                  <FormField label={t("authenticationPage.password")}>
                    <AuthenticationPasswordInput
                      autoComplete={setup ? "new-password" : "current-password"}
                      name="password"
                      value={password}
                      minLength={setup ? 15 : 1}
                      maxLength={1024}
                      required
                      disabled={busy}
                      onChange={(event) => setPassword(event.target.value)}
                    />
                  </FormField>
                  {setup && (
                    <FormField
                      label={t("authenticationPage.confirmPassword")}
                      description={
                        <>
                          <span className="authentication-hint">
                            {t(
                              "authenticationPage.useAtLeast15CharactersSpacesAndPassphrasesAreWelcome",
                            )}
                          </span>
                        </>
                      }
                    >
                      <TextInput
                        type="password"
                        autoComplete="new-password"
                        name="confirmation"
                        value={confirmation}
                        minLength={15}
                        maxLength={1024}
                        required
                        disabled={busy}
                        onChange={(event) =>
                          setConfirmation(event.target.value)
                        }
                      />
                    </FormField>
                  )}
                  <Button
                    variant="primary"
                    disabled={busy}
                    loading={busy}
                    loadingLabel={t("authenticationPage.pleaseWait")}
                  >
                    {setup
                      ? t("authenticationPage.createAdministrator")
                      : t("authenticationPage.signIn")}
                    <ArrowRight size={18} aria-hidden="true" />
                  </Button>
                  {!setup && (
                    <p className="authentication-hint authentication-recovery">
                      {t(
                        "authenticationPage.forgotYourPasswordUseTheResetPasswordCommandOnYour",
                      )}
                    </p>
                  )}
                </>
              )}
            </form>
          )}
        </section>
      </div>
    </main>
  );
}
