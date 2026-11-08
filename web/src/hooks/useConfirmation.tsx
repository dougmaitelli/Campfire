import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type ComponentProps,
} from "react";
import { ConfirmDialog } from "../components/layout/ConfirmDialog";

type Confirmation = Pick<
  ComponentProps<typeof ConfirmDialog>,
  "title" | "description" | "confirmLabel" | "danger" | "warning"
>;

export function useConfirmation() {
  const [options, setOptions] = useState<Confirmation | null>(null);
  const pending = useRef<((confirmed: boolean) => void) | null>(null);

  const confirm = useCallback((next: Confirmation): Promise<boolean> => {
    if (pending.current) return Promise.resolve(false);
    return new Promise((resolve) => {
      pending.current = resolve;
      setOptions(next);
    });
  }, []);

  const finish = useCallback((confirmed: boolean) => {
    const resolve = pending.current;
    pending.current = null;
    setOptions(null);
    resolve?.(confirmed);
  }, []);

  useEffect(
    () => () => {
      pending.current?.(false);
      pending.current = null;
    },
    [],
  );

  return {
    confirm,
    dialog: options && (
      <ConfirmDialog
        {...options}
        busy={false}
        error={null}
        onClose={() => finish(false)}
        onConfirm={() => finish(true)}
      />
    ),
  };
}
