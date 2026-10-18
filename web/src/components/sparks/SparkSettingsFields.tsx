import { FormField } from "../forms/FormField";
import { TextInput } from "../forms/TextInput";
import { Button } from "../forms/Button";
import { Select } from "../forms/Select";
import type { PortAllocation, SparkSettings } from "../../api/sparks";

interface SparkSettingsFieldsProps {
  value: SparkSettings;
  onChange: (settings: SparkSettings) => void;
}

export function SparkSettingsFields({
  value,
  onChange,
}: SparkSettingsFieldsProps) {
  function changeAllocation(index: number, fields: Partial<PortAllocation>) {
    onChange({
      ...value,
      port_allocations: value.port_allocations.map((entry, i) =>
        i === index ? { ...entry, ...fields } : entry,
      ),
    });
  }

  return (
    <div className="spark-settings">
      <h3>Resource limits</h3>
      <p className="field-help">
        Capacity available for game servers. Set a limit to 0 for no configured
        limit.
      </p>
      <div className="resource-fields">
        <div>
          <FormField label="CPU cores">
            <TextInput
              id="spark-cpu"
              type="number"
              min={0}
              max={1024}
              step="any"
              required
              value={Number.isNaN(value.cpu_limit) ? "" : value.cpu_limit}
              onChange={(event) =>
                onChange({ ...value, cpu_limit: event.target.valueAsNumber })
              }
            />
          </FormField>
        </div>
        <div>
          <FormField label="Memory (MiB)">
            <TextInput
              id="spark-memory"
              type="number"
              min={0}
              max={Number.MAX_SAFE_INTEGER}
              step={1}
              required
              value={Number.isNaN(value.memory_mb) ? "" : value.memory_mb}
              onChange={(event) =>
                onChange({ ...value, memory_mb: event.target.valueAsNumber })
              }
            />
          </FormField>
        </div>
        <div>
          <FormField label="Disk (MiB)">
            <TextInput
              id="spark-disk"
              type="number"
              min={0}
              max={Number.MAX_SAFE_INTEGER}
              step={1}
              required
              value={Number.isNaN(value.disk_mb) ? "" : value.disk_mb}
              onChange={(event) =>
                onChange({ ...value, disk_mb: event.target.valueAsNumber })
              }
            />
          </FormField>
        </div>
      </div>
      <h3>Game networking</h3>
      <FormField
        label={
          <>
            Game address <span className="field-help">(optional)</span>
          </>
        }
      >
        <TextInput
          id="spark-address"
          value={value.game_address}
          maxLength={253}
          placeholder="games.example.com or an IP"
          onChange={(event) =>
            onChange({ ...value, game_address: event.target.value })
          }
          aria-describedby="game-address-help"
        />
      </FormField>
      <p id="game-address-help" className="field-help">
        The address players use to reach game servers. Enter an IP or domain
        without a protocol or port.
      </p>
      <h3>Port allocations</h3>
      <p className="field-help">
        Game ports available on this Spark. Use the same start and end for one
        port; 0.0.0.0 binds all IPv4 interfaces.
      </p>
      {value.port_allocations.map((allocation, index) => (
        <fieldset className="allocation-range" key={index}>
          <legend>Range {index + 1}</legend>
          <FormField label="Bind IP">
            <TextInput
              id={`allocation-ip-${index}`}
              required
              value={allocation.bind_ip}
              onChange={(event) =>
                changeAllocation(index, { bind_ip: event.target.value })
              }
              placeholder="0.0.0.0"
            />
          </FormField>
          <div className="allocation-ports">
            <div>
              <FormField label="Start port">
                <TextInput
                  id={`allocation-start-${index}`}
                  type="number"
                  min={1}
                  max={65535}
                  step={1}
                  required
                  value={Number.isNaN(allocation.start) ? "" : allocation.start}
                  onChange={(event) =>
                    changeAllocation(index, {
                      start: event.target.valueAsNumber,
                    })
                  }
                />
              </FormField>
            </div>
            <div>
              <FormField label="End port">
                <TextInput
                  id={`allocation-end-${index}`}
                  type="number"
                  min={allocation.start}
                  max={65535}
                  step={1}
                  required
                  value={Number.isNaN(allocation.end) ? "" : allocation.end}
                  onChange={(event) =>
                    changeAllocation(index, { end: event.target.valueAsNumber })
                  }
                />
              </FormField>
            </div>
          </div>
          <FormField label="Protocol">
            <Select
              id={`allocation-protocol-${index}`}
              value={allocation.protocol}
              onChange={(event) =>
                changeAllocation(index, {
                  protocol: event.target.value === "udp" ? "udp" : "tcp",
                })
              }
            >
              <option value="tcp">TCP</option>
              <option value="udp">UDP</option>
            </Select>
          </FormField>
          <Button
            type="button"
            variant="danger"
            aria-label={`Remove range ${index + 1}`}
            onClick={() =>
              onChange({
                ...value,
                port_allocations: value.port_allocations.filter(
                  (_, i) => i !== index,
                ),
              })
            }
          >
            Remove range
          </Button>
        </fieldset>
      ))}
      <Button
        type="button"

        disabled={value.port_allocations.length >= 128}
        onClick={() =>
          onChange({
            ...value,
            port_allocations: [
              ...value.port_allocations,
              { bind_ip: "0.0.0.0", start: 25565, end: 25565, protocol: "tcp" },
            ],
          })
        }
      >
        + Add port range
      </Button>
    </div>
  );
}
