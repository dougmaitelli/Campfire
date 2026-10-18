import { Button } from "../components/forms/Button";
import { ContentState } from "../components/layout/ContentState";
import { InlineMessage } from "../components/status/InlineMessage";
import { useEffect, useState } from "react";
import type { ReactNode } from "react";
import { Drawer } from "../components/layout/Drawer";
import { DrawerDeleteButton } from "../components/layout/DrawerDeleteButton";

export function ResourceDetailsDialog<T>({
  title,
  description,
  editLabel = "Edit",
  loadDetails,
  children,
  onClose,
  onEdit,
  editDisabled = false,
  onDelete,
  deleteDisabled = false,
  deleting = false,
  actionError,
  actions,
}: ResourceDetailsDialogProps<T>) {
  const [details, setDetails] = useState<T | null>(null);
  const [error, setError] = useState<string | null>(null);
  useEffect(() => {
    const controller = new AbortController();
    void loadDetails(
      AbortSignal.any([controller.signal, AbortSignal.timeout(15000)]),
    )
      .then((value) => {
        if (!controller.signal.aborted) setDetails(value);
      })
      .catch((cause: unknown) => {
        if (!controller.signal.aborted)
          setError(
            cause instanceof Error ? cause.message : "Unable to load details.",
          );
      });
    return () => controller.abort();
  }, [loadDetails]);
  return (
    <Drawer
      title={title}
      description={description}
      busy={deleting}
      onClose={onClose}
      actions={actions}
      footerLeft={
        <Button type="button" disabled={deleting} onClick={onClose}>
          Close
        </Button>
      }
      footerRight={
        <>
          <DrawerDeleteButton
            onDelete={onDelete}
            disabled={!details || deleteDisabled}
            deleting={deleting}
          />
          {onEdit && (
            <Button
              type="button"
              variant="primary"
              disabled={!details || editDisabled || deleting}
              onClick={onEdit}
            >
              {editLabel}
            </Button>
          )}
        </>
      }
    >
      <div className="drawer-content">
        {actionError && (
          <InlineMessage role="alert">{actionError}</InlineMessage>
        )}
        {error && <InlineMessage role="alert">{error}</InlineMessage>}
        {!details && !error && (
          <ContentState loading resource="details" compact />
        )}
        {details && children(details)}
      </div>
    </Drawer>
  );
}

export interface ResourceDetailsDialogProps<T> {
  title: ReactNode;
  description: string;
  editLabel?: string;
  loadDetails: (signal: AbortSignal) => Promise<T>;
  children: (details: T) => ReactNode;
  onClose: () => void;
  onEdit?: (() => void) | undefined;
  editDisabled?: boolean;
  onDelete?: (() => void) | undefined;
  deleteDisabled?: boolean;
  deleting?: boolean;
  actionError?: string | null;
  actions?: ReactNode;
}
