import { FormField } from "../components/forms/FormField";
import { Button } from "../components/forms/Button";
import { InlineMessage } from "../components/status/InlineMessage";
import { Textarea } from "../components/forms/Textarea";
import { TextInput } from "../components/forms/TextInput";
import { useEffect, useRef, useState } from "react";
import { createSparkEnrollment } from "../api/sparks";
import type { Spark, SparkEnrollment } from "../api/sparks";
import { Drawer } from "../components/layout/Drawer";
import {
  sparkInstallCommand,
  validAgentAddress,
} from "../components/sparks/installCommand";

export function EnrollSparkDialog({
  spark,
  onClose,
  reEnrollment = false,
}: {
  spark: Spark;
  onClose: () => void;
  reEnrollment?: boolean;
}) {
  const [enrollment, setEnrollment] = useState<SparkEnrollment | null>(null);
  const [busy, setBusy] = useState(!spark.disabled);
  const [error, setError] = useState<string | null>(null);
  const [panel, setPanel] = useState(
    `https://${window.location.hostname}:8443`,
  );
  const [caFile, setCAFile] = useState("");
  const [copied, setCopied] = useState(false);
  const started = useRef(false);
  const pending = useRef(false);
  const panelEdited = useRef(false);

  function acceptEnrollment(value: SparkEnrollment) {
    setEnrollment(value);
    if (!panelEdited.current && value.agent_url) setPanel(value.agent_url);
  }
  useEffect(() => {
    if (started.current || spark.disabled) return;
    started.current = true;
    pending.current = true;
    void createSparkEnrollment(spark.id)
      .then(acceptEnrollment)
      .catch((cause: unknown) =>
        setError(
          cause instanceof Error
            ? cause.message
            : "Unable to create enrollment token.",
        ),
      )
      .finally(() => {
        pending.current = false;
        setBusy(false);
      });
  }, [spark.id, spark.disabled]);
  const validPanel = validAgentAddress(panel);
  const command = enrollment
    ? sparkInstallCommand(
        window.location.origin,
        panel,
        enrollment.token,
        caFile,
        enrollment.server_ca,
      )
    : "";
  async function generate() {
    if (pending.current) return;
    pending.current = true;
    setBusy(true);
    setError(null);
    setCopied(false);
    setEnrollment(null);
    try {
      acceptEnrollment(await createSparkEnrollment(spark.id));
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : "Unable to create enrollment token.",
      );
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }
  return (
    <Drawer
      title={`${reEnrollment ? "Re-enroll Spark" : "Install Spark"} · ${spark.name}`}
      description="Run one command on the game host to install and enroll Spark."
      busy={busy}
      onClose={onClose}
      footerLeft={
        <Button
          type="button"

          disabled={busy}
          onClick={onClose}
        >
          Close
        </Button>
      }
      footerRight={
        <Button
          type="button"
          variant="primary"
          disabled={busy || spark.disabled}
          onClick={() => {
            void generate();
          }}
          loading={busy}
          loadingLabel={"Generating…"}
        >
          {enrollment ? "Generate new command" : "Generate command"}
        </Button>
      }
    >
      <div className="drawer-content">
        {reEnrollment && (
          <InlineMessage tone="warning">
            Running this command replaces the existing Spark credentials and
            briefly restarts its service. Agents using the old credentials will
            no longer be able to reconnect. Server files and configuration are
            preserved. Credentials change only after successful enrollment.
          </InlineMessage>
        )}
        {error && <InlineMessage role="alert">{error}</InlineMessage>}
        {spark.disabled && (
          <InlineMessage tone="info">
            Enable this Spark before generating an installation command.
          </InlineMessage>
        )}
        <p>
          Run this on a Linux amd64 or arm64 host with Docker running, curl,
          systemd, and sudo access. It downloads the latest stable GitHub
          release, verifies its checksum, installs Spark, enrolls it, and starts
          the service.
        </p>
        <FormField
          label="Agent HTTPS address"
          description="Use Campfire’s dedicated agent listener, which may have a different address or port from this page."
        >
          <TextInput
            type="url"
            value={panel}
            onChange={(event) => {
              panelEdited.current = true;
              setPanel(event.target.value);
              setCopied(false);
            }}
            aria-invalid={!validPanel}
          />
        </FormField>
        {!busy && !enrollment?.server_ca && (
          <FormField
            label="Private CA file (optional)"
            description="For a private HTTPS certificate, enter the absolute path to its CA certificate on the Spark host."
          >
            <TextInput
              value={caFile}
              onChange={(event) => {
                setCAFile(event.target.value);
                setCopied(false);
              }}
              placeholder="/etc/campfire/panel-ca.crt"
            />
          </FormField>
        )}
        {busy && <p role="status">Generating installation command…</p>}
        {enrollment && (
          <>
            <FormField label="Installation command">
              <Textarea
                id="spark-install-command"
                readOnly
                value={command}
                spellCheck={false}
              />
            </FormField>
            <Button
              type="button"
              disabled={!command}
              onClick={() => {
                void navigator.clipboard
                  .writeText(command)
                  .then(() => setCopied(true))
                  .catch(() =>
                    setError(
                      "Unable to copy. Select and copy the command above.",
                    ),
                  );
              }}
            >
              {copied ? "Copied" : "Copy command"}
            </Button>
            {!validPanel && (
              <InlineMessage>
                Enter a valid HTTPS agent address without a path.
              </InlineMessage>
            )}
            <p>
              This command contains a one-time enrollment token and expires{" "}
              {new Date(enrollment.expires_at).toLocaleString()}. Generating a
              new command replaces the previous unused token.
            </p>
            <p>Refresh the Sparks page to see the connection status.</p>
          </>
        )}
      </div>
    </Drawer>
  );
}
