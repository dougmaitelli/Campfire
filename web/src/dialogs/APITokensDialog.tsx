import i18n from "../i18n";
import { useTranslation, Trans } from "react-i18next";
import { FormField } from "../components/forms/FormField";
import { Button } from "../components/forms/Button";
import "./api-tokens-dialog.css";
import { InlineMessage } from "../components/status/InlineMessage";
import { TextInput } from "../components/forms/TextInput";
import { useEffect, useRef, useState } from "react";
import {
  createToken,
  listTokens,
  revokeToken,
  type APIToken,
} from "../api/tokens";
import { Drawer } from "../components/layout/Drawer";
import { CheckboxField } from "../components/forms/CheckboxField";
import {
  Permission,
  togglePermission,
  type PermissionDefinition,
} from "../api/permissions";

export function APITokensDialog({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation();
  const [tokens, setTokens] = useState<APIToken[]>([]);
  const [choices, setChoices] = useState<string[]>([]);
  const [catalog, setCatalog] = useState<PermissionDefinition[]>([]);
  const [scopes, setScopes] = useState<string[]>([Permission.AllAccess]);
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
          setCatalog(result.permissions);
        }
      })
      .catch((cause: unknown) => {
        if (!controller.signal.aborted)
          setError(
            cause instanceof Error
              ? cause.message
              : i18n.t("apiTokensDialog.unableToLoadAPITokens"),
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
        cause instanceof Error
          ? cause.message
          : i18n.t("apiTokensDialog.unableToUpdateAPITokens"),
      );
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }
  function toggle(scope: string, checked: boolean) {
    setScopes((current) => {
      let next = togglePermission(current, scope, checked, catalog);
      if (
        checked &&
        scope.startsWith("server.") &&
        !next.includes(Permission.ServerView)
      )
        next.push(Permission.ServerView);
      if (!checked && scope === Permission.ServerView)
        next = next.filter((item) => !item.startsWith("server."));
      return next;
    });
  }
  return (
    <Drawer
      title={t("apiTokensDialog.apiTokens")}
      description={t(
        "apiTokensDialog.tokensActAsYourUserScopesCanRestrictYourPermissions",
      )}
      busy={busy}
      onClose={onClose}
      footerLeft={
        <Button disabled={busy} onClick={onClose}>
          {t("common.close")}
        </Button>
      }
      footerRight={null}
    >
      <div className="drawer-content">
        {error && <InlineMessage role="alert">{error}</InlineMessage>}
        {secret && (
          <section aria-label={t("apiTokensDialog.newTokenSecret")}>
            <h3>{t("apiTokensDialog.saveYourToken")}</h3>
            <p>
              {t(
                "apiTokensDialog.thisSecretIsShownOnlyOnceStoreItSomewhereSafe",
              )}
            </p>
            <TextInput
              className="api-token-secret"
              aria-label={t("apiTokensDialog.tokenSecret")}
              value={secret}
              readOnly
              spellCheck={false}
              autoComplete="off"
            />
            <Button
              disabled={busy}
              onClick={() => {
                void perform(async () => {
                  await navigator.clipboard.writeText(secret);
                  setCopied(true);
                });
              }}
            >
              {copied
                ? t("apiTokensDialog.copied")
                : t("apiTokensDialog.copyToken")}
            </Button>
          </section>
        )}
        <h3>{t("apiTokensDialog.createAToken")}</h3>
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
          <FormField label={t("common.name")}>
            <TextInput
              id="token-name"
              required
              maxLength={100}
              value={name}
              disabled={busy}
              onChange={(event) => setName(event.target.value)}
            />
          </FormField>
          <FormField label={t("apiTokensDialog.expiresInDays")}>
            <TextInput
              id="token-expiry"
              type="number"
              min={1}
              max={365}
              required
              value={days}
              disabled={busy}
              onChange={(event) => setDays(Number(event.target.value))}
            />
          </FormField>
          <CheckboxField
            checked={scopes.includes(Permission.AllAccess)}
            disabled={busy}
            onChange={(event) =>
              setScopes(
                event.target.checked
                  ? [Permission.AllAccess]
                  : [Permission.ServerView],
              )
            }
          >
            {t("apiTokensDialog.allMyCurrentPermissions")}
          </CheckboxField>
          {!scopes.includes(Permission.AllAccess) &&
            choices
              .filter((scope) => scope !== Permission.AllAccess)
              .map((scope) => (
                <CheckboxField
                  key={scope}
                  checked={scopes.includes(scope)}
                  disabled={busy}
                  onChange={(event) => toggle(scope, event.target.checked)}
                >
                  {scope}
                </CheckboxField>
              ))}
          <p className="field-help">
            {t(
              "apiTokensDialog.serverAccessFollowsYourOwnershipAndAssignmentsAccountPasswordRole",
            )}
          </p>
          <Button
            variant="primary"
            disabled={
              busy ||
              loading ||
              !choices.length ||
              !name.trim() ||
              !scopes.length ||
              !!secret
            }
          >
            {t("apiTokensDialog.createToken")}
          </Button>
          {secret && (
            <p>
              {t(
                "apiTokensDialog.closeAndReopenThisWindowToCreateAnotherToken",
              )}
            </p>
          )}
        </form>
        <h3>{t("apiTokensDialog.activeTokens")}</h3>
        {loading ? (
          <p>{t("common.loading")}</p>
        ) : (
          !tokens.length && <p>{t("apiTokensDialog.noActiveAPITokens")}</p>
        )}
        {tokens.map((token) => (
          <section key={token.id} aria-label={token.name}>
            <h4 className="api-token-heading">{token.name}</h4>
            <p>
              <Trans
                i18nKey="apiTokensDialog.expires"
                t={i18n.t}
                components={{
                  code: <code>{token.prefix}…</code>,
                  newDateToken: (
                    <>
                      {new Date(token.expires_at * 1000).toLocaleDateString()}
                    </>
                  ),
                }}
              />
            </p>
            <p>
              <Trans
                i18nKey="apiTokensDialog.lastUsed"
                t={i18n.t}
                components={{
                  tokenLastUsed: (
                    <>
                      {token.last_used_at
                        ? new Date(token.last_used_at * 1000).toLocaleString()
                        : i18n.t("apiTokensDialog.never")}
                    </>
                  ),
                }}
              />
            </p>
            <p>
              <Trans
                i18nKey="apiTokensDialog.scopes"
                t={i18n.t}
                components={{
                  tokenScopesIncludes: (
                    <>
                      {token.scopes.includes(Permission.AllAccess)
                        ? i18n.t("apiTokensDialog.allMyCurrentPermissions")
                        : token.scopes.join(", ")}
                    </>
                  ),
                }}
              />
            </p>
            <Button
              variant="danger"
              disabled={busy}
              onClick={() => {
                if (
                  !window.confirm(
                    i18n.t(
                      "apiTokensDialog.revokeApplicationsUsingItWillLoseAccessImmediately",
                      { tokenName: token.name },
                    ),
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
              {i18n.t("apiTokensDialog.revoke")}
            </Button>
          </section>
        ))}
      </div>
    </Drawer>
  );
}
