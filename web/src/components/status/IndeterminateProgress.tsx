import "./indeterminate-progress.css";

export function IndeterminateProgress({ label }: { label: string }) {
  return (
    <div className="indeterminate-progress">
      <span role="status">{label}</span>
      <div
        className="indeterminate-progress-track"
        role="progressbar"
        aria-label={label}
      >
        <span />
      </div>
    </div>
  );
}
