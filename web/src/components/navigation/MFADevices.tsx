import { useState } from "react";
import { useTranslation } from "react-i18next";
import { removeMFADevice, type MFADevice, type MFAStatus } from "../../api/mfa";
import { Button } from "../forms/Button";
import { Table } from "../table/Table";
import { InputDialog } from "../layout/InputDialog";
import { MFAForm } from "./MFAForm";

export function MFADevices({
  status,
  disabled,
  onChanged,
  onBusy,
}: {
  status: MFAStatus;
  disabled: boolean;
  onChanged: () => void;
  onBusy: (busy: boolean) => void;
}) {
  const { t } = useTranslation();
  const [adding, setAdding] = useState(false);
  const [removing, setRemoving] = useState<MFADevice | null>(null);
  return (
    <section className="form-stack">
      <h3>{t("mfa.title")}</h3>
      {!status.available ? (
        <p>{t("mfa.oidcManaged")}</p>
      ) : (
        <>
          <Table
            aria-label={t("mfa.devices")}
            empty={status.devices.length === 0}
            hideEmptyState
          >
            <thead>
              <tr>
                <th>{t("mfa.deviceName")}</th>
                <th>{t("mfa.added")}</th>
                <th className="table-actions">{t("common.actions")}</th>
              </tr>
            </thead>
            <tbody>
              {status.devices.map((device) => (
                <tr key={device.id}>
                  <td>{device.name}</td>
                  <td>
                    {device.created_at
                      ? new Date(device.created_at * 1000).toLocaleDateString()
                      : "—"}
                  </td>
                  <td className="table-actions">
                    <Button
                      type="button"
                      variant="danger"
                      disabled={disabled}
                      aria-label={t("mfa.removeDevice", { name: device.name })}
                      onClick={() => setRemoving(device)}
                    >
                      {t("common.remove")}
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </Table>
          <Button
            type="button"
            disabled={disabled}
            onClick={() => setAdding(true)}
          >
            {t("mfa.addDevice")}
          </Button>
          {status.enabled && (
            <p className="field-help">
              {t("mfa.recoveryRemaining", { count: status.recovery_remaining })}
            </p>
          )}
        </>
      )}
      {adding && (
        <MFAForm
          onBusy={onBusy}
          onClose={() => setAdding(false)}
          onComplete={() => {
            setAdding(false);
            onBusy(false);
            onChanged();
          }}
        />
      )}
      {removing && (
        <InputDialog
          title={t("mfa.removeDevice", { name: removing.name })}
          label={t("mfa.code")}
          description={t(
            status.devices.length === 1
              ? "mfa.removeLastWarning"
              : "mfa.removeWarning",
          )}
          submitLabel={t("common.remove")}
          onClose={() => setRemoving(null)}
          onSubmit={async (code) => {
            onBusy(true);
            try {
              await removeMFADevice(removing.id, code);
              onChanged();
            } finally {
              onBusy(false);
            }
          }}
        />
      )}
    </section>
  );
}
