import { useCallback } from "react";
import {
  serverDiscordSettings,
  retryIntegrationCard,
} from "../../api/integrations";
import { CardPublishingStatus } from "../integrations/CardPublishingStatus";

export function IntegrationCardStatus({
  serverID,
  bindingID,
  revision,
  disabled,
  showStatus,
}: {
  serverID: string;
  bindingID: string;
  revision: string;
  disabled: boolean;
  showStatus: boolean;
}) {
  const readStatus = useCallback(
    async (signal: AbortSignal) =>
      (await serverDiscordSettings(serverID, bindingID, signal)).card_status,
    [serverID, bindingID],
  );
  return (
    <CardPublishingStatus
      readStatus={readStatus}
      retry={() => retryIntegrationCard(serverID, bindingID)}
      revision={revision}
      disabled={disabled}
      showStatus={showStatus}
    />
  );
}
