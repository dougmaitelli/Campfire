import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import type { ServerOptions } from "../api/access";
import { quickCopyServer, type CopyTemplate } from "../api/copies";
import { serverNeedsEmberReview, type Server } from "../api/servers";
import { ServerDrawer } from "./ServerDrawer";
import { Drawer } from "../components/layout/Drawer";
import { Button } from "../components/forms/Button";
import { InlineMessage } from "../components/status/InlineMessage";
import { FormField } from "../components/forms/FormField";
import { Select } from "../components/forms/Select";
import i18n from "../i18n";
import { isEnabledSpark } from "../utils/sparkDestinations";

export type ServerCopyMode = "copy" | "quick";

export function ServerCopyFlow({
  source,
  mode,
  template,
  options,
  servers,
  onClose,
  onSaved,
  onSubmitted,
}: {
  source: Server;
  mode: ServerCopyMode;
  template: CopyTemplate;
  options: ServerOptions;
  servers: Server[];
  onClose: () => void;
  onSaved: (server: Server) => void;
  onSubmitted: () => void;
}) {
  const { t } = useTranslation();
  const choices = options.sparks.filter(isEnabledSpark);
  const [sparkID, setSparkID] = useState(() => {
    return (
      choices.find((item) => item.id === source.spark_id)?.id ??
      choices[0]?.id ??
      ""
    );
  });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const pending = useRef(false);
  const created = useRef(false);
  const spark = choices.find((item) => item.id === sparkID);

  if (mode === "copy")
    return (
      <ServerDrawer
        copy={template}
        server={null}
        s3Configured={options.s3_available}
        sparks={options.sparks}
        embers={[template.ember]}
        servers={servers}
        onClose={() => {
          if (created.current) onSubmitted();
          else onClose();
        }}
        onSaved={(server) => {
          created.current = true;
          onSaved(server);
        }}
      />
    );

  async function submit() {
    if (pending.current || !spark || serverNeedsEmberReview(template.server))
      return;
    pending.current = true;
    setBusy(true);
    setError(null);
    try {
      onSaved(await quickCopyServer(source.id, sparkID));
      onSubmitted();
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : i18n.t("serverCopy.unableToCreate"),
      );
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }

  return (
    <Drawer
      title={t("serverCopy.quickCopy")}
      description={t("serverCopy.quickDescription", { name: source.name })}
      busy={busy}
      onClose={onClose}
      footerLeft={
        <Button disabled={busy} onClick={onClose}>
          {t("common.close")}
        </Button>
      }
      footerRight={
        <Button
          variant="primary"
          disabled={busy || !spark || serverNeedsEmberReview(template.server)}
          onClick={() => void submit()}
        >
          {t("serverCopy.quickCopy")}
        </Button>
      }
    >
      <div className="panel-content">
        {error && <InlineMessage role="alert">{error}</InlineMessage>}
        {serverNeedsEmberReview(template.server) && (
          <InlineMessage tone="info">
            {t("serverCopy.updateRequired")}
          </InlineMessage>
        )}
        <FormField label={t("serverCopy.destinationSpark")}>
          <Select
            value={sparkID}
            onValueChange={setSparkID}
            disabled={busy}
            required
          >
            <option value="" disabled>
              {t("serverCopy.chooseSpark")}
            </option>
            {choices.map((spark) => (
              <option key={spark.id} value={spark.id}>
                {spark.name}
                {spark.connected
                  ? ""
                  : ` (${t("sparkConnectionState.disconnected")})`}
              </option>
            ))}
          </Select>
        </FormField>
      </div>
    </Drawer>
  );
}
