import { useTranslation } from "react-i18next";
import { resolveGameAddress } from "../../utils/gameAddress";

export function GameAddressReading({
  override,
  host,
  port = 0,
  spark,
}: {
  override?: string | undefined;
  host?: string | undefined;
  port?: number | undefined;
  spark?: string | undefined;
}) {
  const { t } = useTranslation();
  const address = resolveGameAddress(override, host, port);
  return (
    <div style={{ overflowWrap: "anywhere" }}>
      <span>{address || t("serverDrawer.noGameAddress")}</span>
      {!override && address && (
        <p className="field-help">
          {t("serverDrawer.inheritedGameAddress", { spark: spark ?? "", port })}
        </p>
      )}
    </div>
  );
}
