import { OperationProgress } from "./OperationProgress";

export function IndeterminateProgress({ label }: { label: string }) {
  return (
    <OperationProgress
      label={label}
      detail={<span role="status">{label}</span>}
    />
  );
}
