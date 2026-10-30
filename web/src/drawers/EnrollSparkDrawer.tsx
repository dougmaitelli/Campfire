import i18n from "../i18n";
import { useTranslation, Trans } from "react-i18next";
import { FormField } from "../components/forms/FormField";
import { Button } from "../components/forms/Button";
import { InlineMessage } from "../components/status/InlineMessage";
import { Textarea } from "../components/forms/Textarea";
import { TextInput } from "../components/forms/TextInput";
import { useEffect, useRef, useState } from "react";
import { createSparkEnrollment } from "../api/sparks";
import type { Spark, SparkEnrollment } from "../api/sparks";
import { Drawer } from "../components/layout/Drawer";
import { SparkConnectionState } from "../components/sparks/SparkConnectionState";
import {
  sparkInstallCommand,
  validAgentAddress,
} from "../components/sparks/installCommand";

export function EnrollSparkDrawer({
  spark,
  onClose,
  reEnrollment = false,
}: {
  spark: Spark;
  onClose: () => void;
  reEnrollment?: boolean;
}) {
  const { t } = useTranslation();
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
            : i18n.t("enrollSparkDrawer.unableToCreateEnrollmentToken"),
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
          : i18n.t("enrollSparkDrawer.unableToCreateEnrollmentToken"),
      );
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }
  return (
    <Drawer
      title={
        <>
          <span>
            {t(
              reEnrollment
                ? "enrollSparkDrawer.reEnrollTitle"
                : "enrollSparkDrawer.installTitle",
              { name: spark.name },
            )}
          </span>
          <SparkConnectionState
            disabled={spark.disabled}
            connected={spark.connected}
            version={spark.agent_version}
          />
        </>
      }
      description={t(
        "enrollSparkDrawer.runOneCommandOnTheGameHostToInstallAnd",
      )}
      busy={busy}
      onClose={onClose}
      footerLeft={
        <Button
          type="button"

          disabled={busy}
          onClick={onClose}
        >
          {t("common.close")}
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
          loadingLabel={t("enrollSparkDrawer.generating")}
        >
          {enrollment
            ? t("enrollSparkDrawer.generateNewCommand")
            : t("enrollSparkDrawer.generateCommand")}
        </Button>
      }
    >
      <div className="drawer-content">
        {reEnrollment && (
          <InlineMessage tone="warning">
            {t(
              "enrollSparkDrawer.runningThisCommandReplacesTheExistingSparkCredentialsAndBriefly",
            )}
          </InlineMessage>
        )}
        {error && <InlineMessage role="alert">{error}</InlineMessage>}
        {spark.disabled && (
          <InlineMessage tone="info">
            {t(
              "enrollSparkDrawer.enableThisSparkBeforeGeneratingAnInstallationCommand",
            )}
          </InlineMessage>
        )}
        <p>{t("enrollSparkDrawer.runThisOnALinuxAmd64OrArm64HostWith")}</p>
        <FormField
          label={t("enrollSparkDrawer.agentHTTPSAddress")}
          description={t(
            "enrollSparkDrawer.useCampfireSDedicatedAgentListenerWhichMayHaveA",
          )}
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
            label={t("enrollSparkDrawer.privateCAFileOptional")}
            description={t(
              "enrollSparkDrawer.forAPrivateHTTPSCertificateEnterTheAbsolutePathTo",
            )}
          >
            <TextInput
              value={caFile}
              onChange={(event) => {
                setCAFile(event.target.value);
                setCopied(false);
              }}
              placeholder={t("enrollSparkDrawer.etcCampfirePanelCaCrt")}
            />
          </FormField>
        )}
        {busy && (
          <p role="status">
            {t("enrollSparkDrawer.generatingInstallationCommand")}
          </p>
        )}
        {enrollment && (
          <>
            <FormField label={t("enrollSparkDrawer.installationCommand")}>
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
                      i18n.t(
                        "enrollSparkDrawer.unableToCopySelectAndCopyTheCommandAbove",
                      ),
                    ),
                  );
              }}
            >
              {copied
                ? t("enrollSparkDrawer.copied")
                : t("enrollSparkDrawer.copyCommand")}
            </Button>
            {!validPanel && (
              <InlineMessage>
                {t(
                  "enrollSparkDrawer.enterAValidHTTPSAgentAddressWithoutAPath",
                )}
              </InlineMessage>
            )}
            <p>
              <Trans
                i18nKey="enrollSparkDrawer.thisCommandContainsAOneTimeEnrollmentTokenAndExpires"
                t={t}
                components={{
                  newDateEnrollment: (
                    <>{new Date(enrollment.expires_at).toLocaleString()}</>
                  ),
                }}
              />
            </p>
            <p>
              {t(
                "enrollSparkDrawer.refreshTheSparksPageToSeeTheConnectionStatus",
              )}
            </p>
          </>
        )}
      </div>
    </Drawer>
  );
}
