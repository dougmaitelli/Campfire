import { useTranslation } from "react-i18next";
import { PortPill } from "./PortPill";
import { EMPTY_VALUE } from "../../utils/formatDisplayValue";
import "./port-groups.css";

interface PortEntry {
  address?: string | undefined;
  port: number | string;
  protocol: string;
  targetPort?: number;
}

// Callers supply display addresses; this component does not infer resource types.
export function PortGroups({ ports }: { ports: PortEntry[] }) {
  const { t } = useTranslation();
  const groups = new Map<string, PortEntry[]>();
  for (const port of ports) {
    const address = port.address ?? "";
    const entries = groups.get(address) ?? [];
    entries.push(port);
    groups.set(address, entries);
  }
  if (!ports.length) return <>{EMPTY_VALUE}</>;
  return (
    <div className="port-groups">
      {Array.from(groups, ([address, entries]) => (
        <div className="port-group" key={address}>
          {address && <span className="port-group-address">{address}</span>}
          <div className="pill-group">
            {entries.map((entry, index) => (
              <span className="port-group-entry" key={index}>
                <PortPill port={entry.port} protocol={entry.protocol} />
                {entry.targetPort !== undefined &&
                  entry.targetPort !== entry.port && (
                    <span className="port-group-target">
                      {t("portGroups.containerPort", {
                        port: entry.targetPort,
                      })}
                    </span>
                  )}
              </span>
            ))}
          </div>
        </div>
      ))}
    </div>
  );
}
