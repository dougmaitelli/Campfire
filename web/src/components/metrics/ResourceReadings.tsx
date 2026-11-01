import { useTranslation } from "react-i18next";
import { ResourceReading } from "./ResourceReading";
import {
  useResourceReadingProps,
  type ResourceReadingOptions,
} from "./useResourceReadingProps";
import "./resource-readings.css";

export function ResourceReadings({
  layout = "compact",
  ...options
}: ResourceReadingOptions & { layout?: "compact" | "columns" }) {
  const { t } = useTranslation();
  const readings = useResourceReadingProps(options);
  const compact = (
    <div className="resource-readings resource-readings-compact">
      {readings.map(({ key, ...reading }) => (
        <ResourceReading key={key} {...reading} />
      ))}
    </div>
  );
  if (layout === "columns")
    return (
      <>
        {readings.map(({ key, ...reading }) => (
          <td
            className="resource-reading-column"
            key={key}
            data-label={reading.label}
          >
            <ResourceReading {...reading} showLabel={false} />
          </td>
        ))}
        <td
          className="resource-readings-group"
          data-label={t("common.resources")}
        >
          {compact}
        </td>
      </>
    );
  return compact;
}
