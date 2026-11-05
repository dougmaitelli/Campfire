import { useState } from "react";
import { useTranslation } from "react-i18next";
import { JobStatus, JobType } from "../contracts/domains";
import { PageHeader } from "../components/layout/PageHeader";
import { ContentPanel } from "../components/layout/ContentPanel";
import { ListToolbar } from "../components/layout/ListToolbar";
import { FormField } from "../components/forms/FormField";
import { Select } from "../components/forms/Select";
import { JobList } from "../components/jobs/JobList";
import { jobStatusLabels } from "../components/jobs/jobLabels";

export function JobsPage() {
  const { t } = useTranslation();
  const [status, setStatus] = useState<JobStatus | "">("");
  const [type, setType] = useState<JobType | "">("");
  return (
    <>
      <PageHeader
        title={t("common.jobs")}
        description={t("jobsPage.description")}
      />
      <ContentPanel>
        <ListToolbar>
          <FormField label={t("jobsPage.type")}>
            <Select value={type} onValueChange={setType}>
              <option value="">{t("jobsPage.allTypes")}</option>
              <option value={JobType.Copy}>{t("serverCopy.copy")}</option>
              <option value={JobType.Transfer}>
                {t("serversPage.transfer")}
              </option>
            </Select>
          </FormField>
          <FormField label={t("common.status")}>
            <Select value={status} onValueChange={setStatus}>
              <option value="">{t("jobsPage.allStatuses")}</option>
              {Object.values(JobStatus).map((value) => (
                <option key={value} value={value}>
                  {t(jobStatusLabels[value])}
                </option>
              ))}
            </Select>
          </FormField>
        </ListToolbar>
        <JobList
          key={`${type}:${status}`}
          filter={{ type: type || undefined, status: status || undefined }}
        />
      </ContentPanel>
    </>
  );
}
