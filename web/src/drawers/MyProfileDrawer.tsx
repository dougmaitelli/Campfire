import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { readProfile } from "../api/profile";
import { useMFAStatus } from "../hooks/useMFAStatus";
import { UserDrawer } from "./UserDrawer";
import { Drawer } from "../components/layout/Drawer";
import { Button } from "../components/forms/Button";
import { InlineMessage } from "../components/status/InlineMessage";
import { useAuthentication } from "../hooks/useAuthentication";

export function MyProfileDrawer({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation();
  const auth = useAuthentication();
  const [profile, setProfile] = useState<Awaited<
    ReturnType<typeof readProfile>
  > | null>(null);
  const [error, setError] = useState<string | null>(null);
  const mfa = useMFAStatus();
  const [revision, setRevision] = useState(0);
  function refreshProfile() {
    setError(null);
    setRevision((value) => value + 1);
  }
  useEffect(() => {
    const controller = new AbortController();
    void readProfile(controller.signal)
      .then((value) => {
        if (!controller.signal.aborted) {
          setProfile(value);
          setError(null);
        }
      })
      .catch((cause: unknown) => {
        if (!controller.signal.aborted) {
          setError(cause instanceof Error ? cause.message : t("mfa.failed"));
        }
      });
    return () => controller.abort();
  }, [t, revision]);
  if (profile)
    return (
      <UserDrawer
        user={profile.user}
        roles={[]}
        profile={{
          passwordAllowed: profile.localPassword,
          mfa,
        }}
        onClose={onClose}
        onSaved={() => void auth.reload()}
        onDeleted={() => {}}
      />
    );
  return (
    <Drawer
      title={t("profile.title")}
      busy={false}
      onClose={onClose}
      footerLeft={<Button onClick={onClose}>{t("common.close")}</Button>}
      footerRight={null}
    >
      <div className="panel-content">
        {error ? (
          <>
            <InlineMessage role="alert">{error}</InlineMessage>
            <Button onClick={refreshProfile}>{t("common.retry")}</Button>
          </>
        ) : (
          <p role="status">{t("common.loading")}</p>
        )}
      </div>
    </Drawer>
  );
}
