import i18n from "../i18n";
import { useTranslation } from "react-i18next";
import { useCallback } from "react";
import { apiFetch, readJSON } from "../api/client";
import { isSpark, type Spark } from "../api/sparks";
import { SparkReview } from "../components/sparks/SparkReview";
import { SparkFeatures } from "../components/sparks/SparkFeatures";
import {
  ResourceDetailsDrawer,
  type ResourceDetailsDrawerProps,
} from "./ResourceDetailsDrawer";

type Props = Omit<
  ResourceDetailsDrawerProps<Spark>,
  "loadDetails" | "children" | "description" | "editLabel"
> & { id: string; spark?: Spark };

export function SparkDetailsDrawer({
  id,
  spark: currentSpark,
  ...props
}: Props) {
  const { t } = useTranslation();
  const loadDetails = useCallback(
    async (signal: AbortSignal) => {
      const value = await readJSON(
        await apiFetch(`/api/v1/sparks/${encodeURIComponent(id)}`, { signal }),
      );
      if (!isSpark(value))
        throw new Error(
          i18n.t("sparkDetailsDrawer.unexpectedSparkDetailsResponse"),
        );
      return value;
    },
    [id],
  );
  return (
    <ResourceDetailsDrawer
      {...props}
      description={t("sparkDetailsDrawer.sparkDetailsAndConfiguredCapacity")}
      editLabel={t("sparkDetailsDrawer.editSpark")}
      loadDetails={loadDetails}
    >
      {(loadedSpark) => {
        const spark = currentSpark ?? loadedSpark;
        return (
          <>
            <SparkReview
              name={spark.name}
              description={spark.description}
              disabled={spark.disabled}
              settings={spark}
              notice=""
            />
            <SparkFeatures spark={spark} />
          </>
        );
      }}
    </ResourceDetailsDrawer>
  );
}
