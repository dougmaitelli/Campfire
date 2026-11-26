import { useTranslation } from "react-i18next";
import type { MFAState } from "../../hooks/useMFAStatus";
import { useAuthentication } from "../../hooks/useAuthentication";
import { Button } from "../forms/Button";
import { InlineMessage } from "../status/InlineMessage";
import { MFADevices } from "./MFADevices";

export function ProfileMFA({
  state,
  disabled,
  onBusy,
}: {
  state: MFAState;
  disabled: boolean;
  onBusy: (busy: boolean) => void;
}) {
  const { t } = useTranslation();
  const auth = useAuthentication();
  return (
    <>
      {state.error && (
        <>
          <InlineMessage role="alert">{state.error}</InlineMessage>
          <Button disabled={disabled} onClick={state.refresh}>
            {t("common.retry")}
          </Button>
        </>
      )}
      {state.loading && <p role="status">{t("common.loading")}</p>}
      {state.status && (
        <MFADevices
          status={state.status}
          disabled={disabled}
          onBusy={onBusy}
          onChanged={() => {
            state.refresh();
            void auth.reload();
          }}
        />
      )}
    </>
  );
}
