import { FormField } from "../forms/FormField";
import { TextInput } from "../forms/TextInput";
import { Button } from "../forms/Button";
import { Select } from "../forms/Select";
import type { ServerConfiguration, ServerPort } from "../../api/servers";
import type { Spark } from "../../api/sparks";

export function ServerResourcesFields({
  disabled = false,
  value,
  spark,
  onChange,
}: {
  disabled?: boolean;
  value: ServerConfiguration;
  spark: Spark | undefined;
  onChange: (config: ServerConfiguration) => void;
}) {
  function changePort(index: number, changes: Partial<ServerPort>) {
    onChange({
      ...value,
      ports: value.ports.map((port, i) =>
        i === index ? { ...port, ...changes } : port,
      ),
    });
  }
  const allocations = spark?.port_allocations ?? [];
  return (
    <fieldset disabled={disabled}>
      <p className="field-help">
        Reserve capacity on {spark?.name ?? "the selected Spark"}. Set the
        resources allocated to this server.
      </p>
      <FormField label="CPU cores">
        <TextInput
          id="server-cpu"
          type="number"
          min={0.1}
          max={1024}
          step="any"
          required
          value={Number.isNaN(value.cpu_limit) ? "" : value.cpu_limit}
          onChange={(event) =>
            onChange({ ...value, cpu_limit: event.target.valueAsNumber })
          }
        />
      </FormField>
      <div className="form-grid">
        <div>
          <FormField label="Memory (MiB)">
            <TextInput
              id="server-memory"
              type="number"
              min={64}
              max={1048576}
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
              id="server-disk"
              type="number"
              min={1}
              max={1073741824}
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
      <h4>Port mappings</h4>
      <p className="field-help">
        Host ports must belong to the Spark’s allocation ranges and be
        available. Container ports must match the Ember requirements.
      </p>
      {value.ports.map((port, index) => {
        const selected = allocations.findIndex(
          (allocation) =>
            allocation.bind_ip === port.bind_ip &&
            allocation.protocol === port.protocol &&
            port.host_port >= allocation.start &&
            port.host_port <= allocation.end,
        );
        const allocation = allocations[selected];
        return (
          <fieldset className="port-mapping" key={index}>
            <legend>Mapping {index + 1}</legend>
            <FormField label="Spark allocation">
              <Select
                id={`mapping-allocation-${index}`}
                required
                value={selected >= 0 ? String(selected) : ""}
                onChange={(event) => {
                  const next = allocations[Number(event.target.value)];
                  if (next)
                    changePort(index, {
                      bind_ip: next.bind_ip,
                      protocol: next.protocol,
                      host_port: next.start,
                    });
                }}
              >
                <option value="" disabled>
                  Select allocation
                </option>
                {allocations.map((entry, i) => (
                  <option key={i} value={i}>
                    {entry.bind_ip} · {entry.start}–{entry.end} /{" "}
                    {entry.protocol.toUpperCase()}
                  </option>
                ))}
              </Select>
            </FormField>
            <div className="form-grid">
              <div>
                <FormField label="Host port">
                  <TextInput
                    id={`mapping-host-${index}`}
                    type="number"
                    min={allocation?.start ?? 1}
                    max={allocation?.end ?? 65535}
                    step={1}
                    required
                    value={Number.isNaN(port.host_port) ? "" : port.host_port}
                    onChange={(event) =>
                      changePort(index, {
                        host_port: event.target.valueAsNumber,
                      })
                    }
                  />
                </FormField>
              </div>
              <div>
                <FormField label="Container port">
                  <TextInput
                    id={`mapping-container-${index}`}
                    type="number"
                    min={1}
                    max={65535}
                    step={1}
                    required
                    value={
                      Number.isNaN(port.container_port)
                        ? ""
                        : port.container_port
                    }
                    onChange={(event) =>
                      changePort(index, {
                        container_port: event.target.valueAsNumber,
                      })
                    }
                  />
                </FormField>
              </div>
            </div>
            <Button
              type="button"
              variant="danger"
              onClick={() =>
                onChange({
                  ...value,
                  ports: value.ports.filter((_, i) => i !== index),
                })
              }
              aria-label={`Remove mapping ${index + 1}`}
            >
              Remove mapping
            </Button>
          </fieldset>
        );
      })}
      <Button
        type="button"

        disabled={allocations.length === 0 || value.ports.length >= 32}
        onClick={() => {
          const first = allocations[0];
          if (first)
            onChange({
              ...value,
              ports: [
                ...value.ports,
                {
                  bind_ip: first.bind_ip,
                  host_port: first.start,
                  container_port: first.start,
                  protocol: first.protocol,
                },
              ],
            });
        }}
      >
        + Add mapping
      </Button>
    </fieldset>
  );
}
