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
          <td className="resource-reading-column" key={key}>
            <ResourceReading {...reading} showLabel={false} />
          </td>
        ))}
        <td className="resource-readings-group">{compact}</td>
      </>
    );
  return compact;
}
