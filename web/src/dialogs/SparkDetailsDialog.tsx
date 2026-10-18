import { useCallback } from "react";
import { apiFetch, readJSON } from "../api/client";
import { isSpark, type Spark } from "../api/sparks";
import { SparkReview } from "../components/sparks/SparkReview";
import { SparkFeatures } from "../components/sparks/SparkFeatures";
import {
  ResourceDetailsDialog,
  type ResourceDetailsDialogProps,
} from "./ResourceDetailsDialog";

type Props = Omit<
  ResourceDetailsDialogProps<Spark>,
  "loadDetails" | "children" | "description" | "editLabel"
> & { id: string };

export function SparkDetailsDialog({ id, ...props }: Props) {
  const loadDetails = useCallback(
    async (signal: AbortSignal) => {
      const value = await readJSON(
        await apiFetch(`/api/v1/sparks/${encodeURIComponent(id)}`, { signal }),
      );
      if (!isSpark(value))
        throw new Error("Unexpected Spark details response.");
      return value;
    },
    [id],
  );
  return (
    <ResourceDetailsDialog
      {...props}
      description="Spark details and configured capacity."
      editLabel="Edit Spark"
      loadDetails={loadDetails}
    >
      {(spark) => (
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
      )}
    </ResourceDetailsDialog>
  );
}
