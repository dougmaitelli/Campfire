import i18n from "../i18n";
import { useTranslation, Trans } from "react-i18next";
import { Fieldset } from "../components/forms/Fieldset";
import { FormField } from "../components/forms/FormField";
import { TextInput } from "../components/forms/TextInput";
import { Textarea } from "../components/forms/Textarea";
import { Button } from "../components/forms/Button";
import { Select } from "../components/forms/Select";
import { useEffect, useRef, useState } from "react";
import {
  sparkAccessOptions,
  readSparkAccess,
  saveAccessGrants,
  type AccessGrant,
  type Role,
  type AccessUser,
} from "../api/access";
import { InlineMessage } from "../components/status/InlineMessage";
import { createSpark, updateSpark } from "../api/sparks";
import type { PortAllocation, Spark, SparkSettings } from "../api/sparks";
import { Wizard } from "../components/wizard/Wizard";
import { SparkReview } from "../components/sparks/SparkReview";
import { Drawer } from "../components/layout/Drawer";
import { CheckboxField } from "../components/forms/CheckboxField";
import "./spark-dialog.css";
import { AccessGrantFields } from "../components/forms/AccessGrantFields";
import { AccessGrantLabel } from "../components/forms/AccessGrantLabel";

interface SparkDialogProps {
  spark: Spark | null;
  onClose: () => void;
  onSaved: (spark: Spark) => void;
}

export function SparkDialog({ spark, onClose, onSaved }: SparkDialogProps) {
  const { t } = useTranslation();
  const submittingRef = useRef(false);
  const [created, setCreated] = useState<Spark | null>(null);
  const [roles, setRoles] = useState<Role[]>([]);
  const [users, setUsers] = useState<AccessUser[]>([]);
  const [grants, setGrants] = useState<AccessGrant[]>([]);
  const [removedIDs, setRemovedIDs] = useState<number[]>([]);
  const [rolesLoading, setRolesLoading] = useState(true);
  const [rolesError, setRolesError] = useState<string | null>(null);
  const [rolesAttempt, setRolesAttempt] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    void (
      spark
        ? readSparkAccess(spark.id, controller.signal)
        : sparkAccessOptions(controller.signal)
    )
      .then((access) => {
        if (controller.signal.aborted) return;
        setRoles(access.subject_roles);
        setUsers(access.subject_users);
        setGrants(access.grants);
      })
      .catch((cause: unknown) => {
        if (!controller.signal.aborted)
          setRolesError(
            cause instanceof Error
              ? cause.message
              : i18n.t("sparkDialog.unableToLoadServerCreationAccess"),
          );
      })
      .finally(() => {
        if (!controller.signal.aborted) setRolesLoading(false);
      });
    return () => controller.abort();
  }, [spark, rolesAttempt]);
  const [name, setName] = useState(spark?.name ?? "");
  const [description, setDescription] = useState(spark?.description ?? "");
  const [disabled, setDisabled] = useState(spark?.disabled ?? false);
  const [settings, setSettings] = useState<SparkSettings>(() => ({
    game_address: spark?.game_address ?? "",
    cpu_limit: spark?.cpu_limit ?? 0,
    memory_mb: spark?.memory_mb ?? 0,
    disk_mb: spark?.disk_mb ?? 0,
    port_allocations: spark?.port_allocations ?? [],
  }));
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  function changeAllocation(index: number, fields: Partial<PortAllocation>) {
    setSettings({
      ...settings,
      port_allocations: settings.port_allocations.map((entry, i) =>
        i === index ? { ...entry, ...fields } : entry,
      ),
    });
  }

  async function submit() {
    if (submittingRef.current) return;
    if (rolesLoading || rolesError) {
      setError(
        rolesError ?? i18n.t("sparkDialog.waitForServerCreationAccessToLoad"),
      );
      return;
    }
    submittingRef.current = true;
    setSubmitting(true);
    setError(null);
    try {
      const existing = spark ?? created;
      const result = existing
        ? await updateSpark(existing.id, { description, disabled, ...settings })
        : await createSpark({
            name: name.trim(),
            description,
            disabled,
            ...settings,
          });
      if (!spark) setCreated(result);
      try {
        await saveAccessGrants(
          `/api/v1/sparks/${encodeURIComponent(result.id)}/access`,
          grants,
          removedIDs,
        );
      } catch (cause) {
        throw new Error(
          i18n.t(
            "sparkDialog.sparkSavedButServerCreationAccessCouldNotBeSaved",
            {
              causeInstanceofError: cause instanceof Error ? cause.message : "",
            },
          ),
          { cause },
        );
      }
      onSaved(result);
      onClose();
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : i18n.t("sparkDialog.unableToAddTheSpark"),
      );
    } finally {
      submittingRef.current = false;
      setSubmitting(false);
    }
  }

  return (
    <Wizard
      allowAllSteps={spark !== null}
      busy={submitting}
      error={error}
      finishLabel={spark ? t("common.saveChanges") : t("sparkDialog.addSpark")}
      onFinish={submit}
      steps={[
        {
          id: "basics",
          title: t("sparkDialog.general"),
          validate: () =>
            name.trim() === "" ? i18n.t("sparkDialog.enterASparkName") : null,
          content: (
            <>
              <FormField
                label={t("common.name")}
                description={t(
                  "sparkDialog.runTheSparkAgentOnThisHostOrAnotherMachine",
                )}
              >
                <TextInput
                  id="spark-name"
                  name="name"
                  value={name}
                  onChange={(event) => setName(event.target.value)}
                  required
                  maxLength={100}
                  placeholder={t("sparkDialog.eGHomeServer")}
                  autoComplete="off"
                  readOnly={spark !== null || created !== null}
                />
              </FormField>
              <FormField
                label={
                  <>
                    <Trans
                      i18nKey="sparkDialog.description"
                      t={t}
                      components={{
                        span: (
                          <span className="field-help">
                            {t("sparkDialog.optional")}
                          </span>
                        ),
                      }}
                    />
                  </>
                }
              >
                <Textarea
                  id="spark-description"
                  name="description"
                  value={description}
                  onChange={(event) => setDescription(event.target.value)}
                  maxLength={1000}
                  placeholder={t("common.descriptionPlaceholder")}
                />
              </FormField>
              <CheckboxField
                id="spark-disabled"
                checked={disabled}
                onChange={(event) => setDisabled(event.target.checked)}
              >
                {t("common.disabled")}
              </CheckboxField>
              <p className="field-help">
                {t("sparkDialog.keepThisSparkUnavailableForHostingGameServers")}
              </p>
            </>
          ),
        },
        {
          id: "resources",
          title: t("sparkDialog.resourcesNetworking"),
          content: (
            <div className="spark-settings">
              <h3>{t("sparkDialog.resourceLimits")}</h3>
              <p className="field-help">
                {t("sparkDialog.capacityAvailableForGameServersSetALimitTo0")}
              </p>
              <div className="resource-fields">
                <div>
                  <FormField label={t("sparkDialog.cpuCores")}>
                    <TextInput
                      id="spark-cpu"
                      type="number"
                      min={0}
                      max={1024}
                      step="any"
                      required
                      value={
                        Number.isNaN(settings.cpu_limit)
                          ? ""
                          : settings.cpu_limit
                      }
                      onChange={(event) =>
                        setSettings({
                          ...settings,
                          cpu_limit: event.target.valueAsNumber,
                        })
                      }
                    />
                  </FormField>
                </div>
                <div>
                  <FormField label={t("sparkDialog.memoryMiB")}>
                    <TextInput
                      id="spark-memory"
                      type="number"
                      min={0}
                      max={Number.MAX_SAFE_INTEGER}
                      step={1}
                      required
                      value={
                        Number.isNaN(settings.memory_mb)
                          ? ""
                          : settings.memory_mb
                      }
                      onChange={(event) =>
                        setSettings({
                          ...settings,
                          memory_mb: event.target.valueAsNumber,
                        })
                      }
                    />
                  </FormField>
                </div>
                <div>
                  <FormField label={t("sparkDialog.diskMiB")}>
                    <TextInput
                      id="spark-disk"
                      type="number"
                      min={0}
                      max={Number.MAX_SAFE_INTEGER}
                      step={1}
                      required
                      value={
                        Number.isNaN(settings.disk_mb) ? "" : settings.disk_mb
                      }
                      onChange={(event) =>
                        setSettings({
                          ...settings,
                          disk_mb: event.target.valueAsNumber,
                        })
                      }
                    />
                  </FormField>
                </div>
              </div>
              <h3>{t("sparkDialog.network")}</h3>
              <FormField
                label={
                  <>
                    <Trans
                      i18nKey="sparkDialog.gameAddress"
                      t={t}
                      components={{
                        span: (
                          <span className="field-help">
                            {t("sparkDialog.optional")}
                          </span>
                        ),
                      }}
                    />
                  </>
                }
              >
                <TextInput
                  id="spark-address"
                  value={settings.game_address}
                  maxLength={253}
                  placeholder={t("sparkDialog.gamesExampleComOrAnIP")}
                  onChange={(event) =>
                    setSettings({
                      ...settings,
                      game_address: event.target.value,
                    })
                  }
                  aria-describedby="game-address-help"
                />
              </FormField>
              <p id="game-address-help" className="field-help">
                {t("sparkDialog.theAddressPlayersUseToReachGameServersEnterAn")}
              </p>
              <h3>{t("sparkDialog.portAllocations")}</h3>
              <p className="field-help">
                {t("sparkDialog.gamePortsAvailableOnThisSparkUseTheSameStart")}
              </p>
              {settings.port_allocations.map((allocation, index) => (
                <Fieldset className="allocation-range" key={index}>
                  <legend className="visually-hidden">
                    {i18n.t("sparkDialog.range")}
                    {index + 1}
                  </legend>
                  <FormField
                    label={i18n.t("sparkDialog.bindIP")}
                    className="allocation-ip"
                  >
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
                      <FormField label={i18n.t("sparkDialog.startPort")}>
                        <TextInput
                          id={`allocation-start-${index}`}
                          type="number"
                          min={1}
                          max={65535}
                          step={1}
                          required
                          value={
                            Number.isNaN(allocation.start)
                              ? ""
                              : allocation.start
                          }
                          onChange={(event) =>
                            changeAllocation(index, {
                              start: event.target.valueAsNumber,
                            })
                          }
                        />
                      </FormField>
                    </div>
                    <div>
                      <FormField label={i18n.t("sparkDialog.endPort")}>
                        <TextInput
                          id={`allocation-end-${index}`}
                          type="number"
                          min={allocation.start}
                          max={65535}
                          step={1}
                          required
                          value={
                            Number.isNaN(allocation.end) ? "" : allocation.end
                          }
                          onChange={(event) =>
                            changeAllocation(index, {
                              end: event.target.valueAsNumber,
                            })
                          }
                        />
                      </FormField>
                    </div>
                  </div>
                  <FormField
                    label={i18n.t("sparkDialog.protocol")}
                    className="allocation-protocol"
                  >
                    <Select
                      id={`allocation-protocol-${index}`}
                      value={allocation.protocol}
                      onValueChange={(selectedValue) =>
                        changeAllocation(index, {
                          protocol: selectedValue === "udp" ? "udp" : "tcp",
                        })
                      }
                    >
                      <option value="tcp">{i18n.t("sparkDialog.tcp")}</option>
                      <option value="udp">{i18n.t("sparkDialog.udp")}</option>
                    </Select>
                  </FormField>
                  <Button
                    type="button"
                    variant="danger"
                    iconOnly
                    className="allocation-remove"
                    aria-label={i18n.t("sparkDialog.removeRange", {
                      index1: index + 1,
                    })}
                    title={i18n.t("sparkDialog.removeRange", {
                      index1: index + 1,
                    })}
                    onClick={() =>
                      setSettings({
                        ...settings,
                        port_allocations: settings.port_allocations.filter(
                          (_, i) => i !== index,
                        ),
                      })
                    }
                  >
                    <svg
                      width="16"
                      height="16"
                      viewBox="0 0 24 24"
                      fill="none"
                      stroke="currentColor"
                      strokeWidth="1.8"
                      strokeLinecap="round"
                      aria-hidden="true"
                    >
                      <path d="M6 6l12 12M18 6 6 18" />
                    </svg>
                  </Button>
                </Fieldset>
              ))}
              <Button
                type="button"
                disabled={settings.port_allocations.length >= 128}
                onClick={() =>
                  setSettings({
                    ...settings,
                    port_allocations: [
                      ...settings.port_allocations,
                      {
                        bind_ip: "0.0.0.0",
                        start: 25565,
                        end: 25565,
                        protocol: "tcp",
                      },
                    ],
                  })
                }
              >
                {t("sparkDialog.addPortRange")}
              </Button>
            </div>
          ),
        },
        {
          id: "access",
          title: t("sparkDialog.access"),
          validate: () =>
            rolesLoading
              ? i18n.t("sparkDialog.waitForServerCreationAccessToLoad")
              : rolesError,
          content: (
            <Fieldset>
              <legend>{t("sparkDialog.serverCreationAccess")}</legend>
              <p className="field-help">
                {t("sparkDialog.chooseWhichUsersOrRolesCanCreateServersOnThis")}
              </p>
              {rolesLoading ? (
                <p role="status">{t("sparkDialog.loadingRoles")}</p>
              ) : rolesError ? (
                <>
                  <InlineMessage>{rolesError}</InlineMessage>
                  <Button
                    type="button"
                    onClick={() => {
                      setRolesLoading(true);
                      setRolesError(null);
                      setRolesAttempt((attempt) => attempt + 1);
                    }}
                  >
                    {t("sparkDialog.retry")}
                  </Button>
                </>
              ) : (
                <AccessGrantFields
                  users={users}
                  grants={grants}
                  subjectRoles={roles}
                  accessRoles={[
                    { id: 2, name: t("sparkDialog.createServers") },
                  ]}
                  onChange={(next, removedID) => {
                    setGrants(next);
                    if (removedID) setRemovedIDs([...removedIDs, removedID]);
                  }}
                />
              )}
              <p className="field-help">
                {t(
                  "sparkDialog.thisDoesNotGrantSparkManagementAccessRemovingAccessDoes",
                )}
              </p>
            </Fieldset>
          ),
        },
        {
          id: "review",
          title: t("common.review"),
          content: (
            <SparkReview
              name={name}
              description={description}
              disabled={disabled}
              settings={settings}
              serverCreationRoles={
                <>
                  <Trans
                    i18nKey="sparkDialog.administrators"
                    t={t}
                    components={{
                      grantsMapGrant: (
                        <>
                          {grants.map((grant) => (
                            <span
                              key={
                                grant.user_id !== null
                                  ? `user:${grant.user_id}`
                                  : `role:${grant.role_id}`
                              }
                            >
                              ,{" "}
                              <AccessGrantLabel
                                grant={grant}
                                showAccessRole={false}
                              />
                            </span>
                          ))}
                        </>
                      ),
                    }}
                  />
                </>
              }
            />
          ),
        },
      ]}
    >
      {(content, actions) => (
        <Drawer
          title={
            spark
              ? i18n.t("sparkDialog.editSpark")
              : i18n.t("sparkDialog.addSpark")
          }
          description={
            spark
              ? i18n.t(
                  "sparkDialog.manageThisNodeSCapacityNetworkAndAvailability",
                )
              : i18n.t("sparkDialog.registerANodeToHostYourGameServers")
          }
          busy={submitting}
          onClose={onClose}
          footerLeft={
            <Button
              type="button"

              disabled={submitting}
              onClick={onClose}
            >
              {i18n.t("common.cancel")}
            </Button>
          }
          footerRight={actions}
        >
          {content}
        </Drawer>
      )}
    </Wizard>
  );
}
