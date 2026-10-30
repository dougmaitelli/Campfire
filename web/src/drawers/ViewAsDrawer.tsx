import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { listUsers } from "../api/access";
import { startViewAs } from "../api/auth";
import { Button } from "../components/forms/Button";
import { FormField } from "../components/forms/FormField";
import { UserSelect } from "../components/forms/UserSelect";
import { Drawer } from "../components/layout/Drawer";
import { InlineMessage } from "../components/status/InlineMessage";
import { useAuthentication } from "../hooks/useAuthentication";
import { useCollection } from "../hooks/useCollection";

export function ViewAsDrawer({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation();
  const auth = useAuthentication();
  const users = useCollection(listUsers);
  const [userID, setUserID] = useState(0);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const pending = useRef(false);
  const available = users.items.filter(
    (user) => !user.disabled && user.id !== auth.session?.user_id,
  );
  async function preview() {
    if (pending.current || !userID) return;
    pending.current = true;
    setBusy(true);
    setError(null);
    try {
      await startViewAs(userID);
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : t("viewAs.unableToStart"),
      );
      pending.current = false;
      setBusy(false);
    }
  }
  return (
    <Drawer
      title={t("viewAs.title")}
      description={t("viewAs.description")}
      busy={busy}
      onClose={onClose}
      footerLeft={
        <Button disabled={busy} onClick={onClose}>
          {t("common.cancel")}
        </Button>
      }
      footerRight={
        <Button
          variant="primary"
          disabled={busy || users.loading || !userID}
          onClick={() => {
            void preview();
          }}
        >
          {t("viewAs.start")}
        </Button>
      }
    >
      <div className="drawer-content">
        {(error ?? users.error) && (
          <InlineMessage role="alert">{error ?? users.error}</InlineMessage>
        )}
        <FormField label={t("common.user")}>
          <UserSelect
            users={available}
            value={userID}
            onValueChange={setUserID}
            disabled={busy || users.loading}
          />
        </FormField>
        {!users.loading && !users.error && available.length === 0 && (
          <p>{t("viewAs.noUsers")}</p>
        )}
      </div>
    </Drawer>
  );
}
