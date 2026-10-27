import { NavigationIcon } from "../navigation/NavigationIcon";
import { MetricTiles } from "./MetricTiles";
import { ResourceReading } from "./ResourceReading";
import {
  useResourceReadingProps,
  type ResourceReadingOptions,
} from "./useResourceReadingProps";

export function ResourceTiles(options: ResourceReadingOptions) {
  const readings = useResourceReadingProps(options);
  return (
    <MetricTiles>
      {readings.map(({ key, ...reading }) => (
        <ResourceReading
          key={key}
          {...reading}
          showAllocation
          icon={
            <NavigationIcon
              name={
                key === "cpu" ? "cpu" : key === "memory" ? "memory" : "disk"
              }
            />
          }
        />
      ))}
    </MetricTiles>
  );
}
