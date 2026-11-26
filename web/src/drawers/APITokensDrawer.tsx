import i18n from "../i18n";
import { useTranslation } from "react-i18next";
import { FormField } from "../components/forms/FormField";
import { Button } from "../components/forms/Button";
import "./api-tokens-drawer.css";
import { InlineMessage } from "../components/status/InlineMessage";
import { TextInput } from "../components/forms/TextInput";
import { useEffect, useId, useRef, useState } from "react";
import { Table } from "../components/table/Table";
import {
  createToken,
  listTokens,
  revokeToken,
  type APIToken,
} from "../api/tokens";
import { Drawer } from "../components/layout/Drawer";
import { useConfirmation } from "../hooks/useConfirmation";
import { CopyableText } from "../components/forms/CopyableText";
import { CheckboxField } from "../components/forms/CheckboxField";
import { Fieldset } from "../components/forms/Fieldset";
import { ServerPermissionTree } from "../components/permissions/ServerPermissionTree";
import { PlatformPermissionFields } from "../components/permissions/PlatformPermissionFields";
import {
  Permission,
  type PermissionDefinition,
  type PermissionID,
} from "../api/permissions";

export function APITokensDrawer({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation();
  const confirmation = useConfirmation();
  const [tokens, setTokens] = useState<APIToken[]>([]);
  const [choices, setChoices] = useState<PermissionID[]>([]);
  const [catalog, setCatalog] = useState<PermissionDefinition[]>([]);
  const [allAccess, setAllAccess] = useState(true);
  const [scopes, setScopes] = useState<PermissionID[]>([]);
  const [name, setName] = useState("");
  const [days, setDays] = useState(90);
  const [creating, setCreating] = useState(false);
  const [created, setCreated] = useState<{
    token: APIToken;
    secret: string;
  } | null>(null);
  const formID = useId();
  const secretBanner = useRef<HTMLElement>(null);
  useEffect(() => {
    // Focusing the newly mounted banner also brings its one-time secret into view,
    // even when the previous form and the returned token list are both scrollable.
    if (!creating) secretBanner.current?.focus();
  }, [creating, created]);
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
              : i18n.t("apiTokensDrawer.unableToLoadAPITokens"),
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
          : i18n.t("apiTokensDrawer.unableToUpdateAPITokens"),
      );
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }
  async function revoke(token: APIToken) {
    if (
      pending.current ||
      !(await confirmation.confirm({
        title: t("confirmDialog.revokeToken"),
        description: t(
          "apiTokensDrawer.revokeApplicationsUsingItWillLoseAccessImmediately",
          { tokenName: token.name },
        ),
        confirmLabel: t("apiTokensDrawer.revoke"),
        danger: true,
      }))
    )
      return;
    await perform(async () => {
      await revokeToken(token.id);
      setTokens((current) => current.filter((item) => item.id !== token.id));
      setCreated((current) =>
        current?.token.id === token.id ? null : current,
      );
    });
  }

  const platformChoices = choices.filter(
    (scope) => scope !== Permission.AllAccess && !scope.startsWith("server."),
  );
  const serverCatalog = catalog.filter(
    (item) => item.id.startsWith("server.") && choices.includes(item.id),
  );
  return (
    <Drawer
      title={t("apiTokensDrawer.apiTokens")}
      description={t(
        "apiTokensDrawer.tokensActAsYourUserScopesCanRestrictYourPermissions",
      )}
      busy={busy}
      onClose={onClose}
      footerLeft={
        <Button
          disabled={busy}
          onClick={
            creating
              ? () => {
                  setCreating(false);
                  setError(null);
                }
              : onClose
          }
        >
          {t(creating ? "common.back" : "common.close")}
        </Button>
      }
      footerRight={
        creating ? (
          <Button
            variant="primary"
            type="submit"
            form={formID}
            loading={busy}
            disabled={
              busy ||
              loading ||
              !choices.length ||
              !name.trim() ||
              (!allAccess && !scopes.length)
            }
          >
            {t("apiTokensDrawer.createToken")}
          </Button>
        ) : (
          <Button
            variant="primary"
            disabled={busy || loading || !choices.length}
            onClick={() => {
              setName("");
              setDays(90);
              setAllAccess(true);
              setScopes([]);
              setError(null);
              setCreating(true);
            }}
          >
            {t("apiTokensDrawer.createToken")}
          </Button>
        )
      }
    >
      {confirmation.dialog}
      <div className="panel-content form-stack">
        {error && <InlineMessage role="alert">{error}</InlineMessage>}
        {!creating && created && (
          <section
            ref={secretBanner}
            tabIndex={-1}
            aria-label={t("apiTokensDrawer.newTokenSecret")}
          >
            <InlineMessage tone="warning" className="api-token-banner">
              <strong>{t("apiTokensDrawer.saveYourToken")}</strong>
              <span>
                {t(
                  "apiTokensDrawer.thisSecretIsShownOnlyOnceStoreItSomewhereSafe",
                )}
              </span>
              <CopyableText
                value={created.secret}
                copyLabel={t("apiTokensDrawer.copyToken")}
                copiedLabel={t("apiTokensDrawer.copied")}
                failedLabel={t("apiTokensDrawer.copyFailed")}
              >
                <code>{created.secret}</code>
              </CopyableText>
            </InlineMessage>
          </section>
        )}
        {creating ? (
          <>
            <h3>{t("apiTokensDrawer.createAToken")}</h3>
            <form
              id={formID}
              className="form-stack"
              onSubmit={(event) => {
                event.preventDefault();
                void perform(async () => {
                  const result = await createToken(
                    name,
                    days,
                    allAccess ? [Permission.AllAccess] : scopes,
                  );
                  setTokens((current) => [result.token, ...current]);
                  setCreated(result);
                  setCreating(false);
                });
              }}
            >
              <FormField label={t("common.name")}>
                <TextInput
                  id="token-name"
                  autoFocus
                  required
                  maxLength={100}
                  value={name}
                  disabled={busy}
                  onChange={(event) => setName(event.target.value)}
                />
              </FormField>
              <FormField label={t("apiTokensDrawer.expiresInDays")}>
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
              <Fieldset
                className="permission-fields form-stack form-stack-compact"
                disabled={busy || loading}
              >
                <legend>{t("roleDrawer.permissions")}</legend>
                <CheckboxField
                  checked={allAccess}
                  onChange={(event) => setAllAccess(event.target.checked)}
                >
                  {t("apiTokensDrawer.allMyCurrentPermissions")}
                </CheckboxField>
              </Fieldset>
              {!allAccess && (
                <>
                  <Fieldset
                    className="permission-fields form-stack form-stack-compact"
                    disabled={busy || loading}
                  >
                    <legend>{t("apiTokensDrawer.platformPermissions")}</legend>
                    <PlatformPermissionFields
                      choices={platformChoices}
                      selected={scopes}
                      onChange={setScopes}
                    />
                  </Fieldset>
                  <Fieldset
                    className="permission-fields form-stack form-stack-compact"
                    disabled={busy || loading}
                  >
                    <legend>{t("apiTokensDrawer.serverPermissions")}</legend>
                    <p className="field-help permission-tree-guidance">
                      {t("roleDrawer.treeGuidance")}
                    </p>
                    <ServerPermissionTree
                      catalog={serverCatalog}
                      selected={scopes}
                      onChange={setScopes}
                    />
                  </Fieldset>
                </>
              )}
              <p className="field-help">
                {t(
                  "apiTokensDrawer.serverAccessFollowsYourOwnershipAndAssignmentsAccountPasswordRole",
                )}
              </p>
            </form>
          </>
        ) : (
          <Table
            aria-label={t("apiTokensDrawer.activeTokens")}
            empty={!tokens.length}
            loading={loading}
            resource={t("apiTokensDrawer.apiTokens")}
            emptyMessage={t("apiTokensDrawer.noActiveAPITokens")}
          >
            <thead>
              <tr>
                <th scope="col">{t("common.name")}</th>
                <th scope="col" className="table-date">
                  {t("apiTokensDrawer.expiresOn")}
                </th>
                <th scope="col" className="table-date">
                  {t("apiTokensDrawer.lastUsedAt")}
                </th>
                <th scope="col" className="table-fit">
                  <span className="visually-hidden">{t("common.actions")}</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {tokens.map((token) => (
                <tr key={token.id}>
                  <td>
                    {token.name}
                    <span className="row-description">
                      <code>{token.prefix}…</code>
                    </span>
                    <span className="row-description">
                      {token.scopes.includes(Permission.AllAccess)
                        ? t("apiTokensDrawer.allMyCurrentPermissions")
                        : token.scopes
                            .map((scope) =>
                              t(
                                `permissionNames.${scope.replaceAll(".", "_")}`,
                                { defaultValue: scope },
                              ),
                            )
                            .join(", ")}
                    </span>
                  </td>
                  <td className="table-date">
                    <time
                      dateTime={new Date(token.expires_at * 1000).toISOString()}
                    >
                      {new Date(token.expires_at * 1000).toLocaleDateString()}
                    </time>
                  </td>
                  <td className="table-date">
                    {token.last_used_at ? (
                      <time
                        dateTime={new Date(
                          token.last_used_at * 1000,
                        ).toISOString()}
                      >
                        {new Date(token.last_used_at * 1000).toLocaleString()}
                      </time>
                    ) : (
                      t("apiTokensDrawer.never")
                    )}
                  </td>
                  <td className="table-fit">
                    <Button
                      variant="danger"
                      disabled={busy}
                      onClick={() => void revoke(token)}
                    >
                      {t("apiTokensDrawer.revoke")}
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </Table>
        )}
      </div>
    </Drawer>
  );
}
