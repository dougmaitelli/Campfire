import { useEffect, useRef, useState } from "react";
import type { FormEvent, ReactNode } from "react";
import "./wizard.css";

export interface WizardStep {
  id: string;
  title: string;
  content: ReactNode;
  validate?: () => string | null;
}

interface WizardProps {
  allowAllSteps?: boolean;
  steps: readonly WizardStep[];
  busy: boolean;
  error: string | null;
  finishLabel: string;
  onFinish: () => Promise<void>;
  onCancel: () => void;
}

export function Wizard({
  allowAllSteps = false,
  steps,
  busy,
  error,
  finishLabel,
  onFinish,
  onCancel,
}: WizardProps) {
  const [index, setIndex] = useState(0);
  const [completedThrough, setCompletedThrough] = useState(() =>
    allowAllSteps ? steps.length - 1 : -1,
  );
  const [validationError, setValidationError] = useState<string | null>(null);
  const headingRef = useRef<HTMLHeadingElement>(null);
  const formRef = useRef<HTMLFormElement>(null);
  const step = steps[index];
  const finalStep = index === steps.length - 1;

  useEffect(() => {
    headingRef.current?.focus();
  }, [index]);

  if (!step) throw new Error("A wizard must contain at least one step.");

  function validateCurrentStep() {
    if (busy || !formRef.current?.reportValidity()) return false;
    const message = step?.validate?.() ?? null;
    setValidationError(message);
    if (message) return false;
    setCompletedThrough((current) => Math.max(current, index));
    return true;
  }

  function goToStep(position: number) {
    if (
      busy ||
      position === index ||
      (!allowAllSteps && position > completedThrough + 1)
    )
      return;
    if (position > index && !validateCurrentStep()) return;
    setValidationError(null);
    setIndex(position);
  }

  function invalidateCurrentStep() {
    setCompletedThrough((current) => Math.min(current, index - 1));
  }

  function advance(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!validateCurrentStep()) return;
    if (finalStep) {
      void onFinish().catch((cause: unknown) => {
        setValidationError(
          cause instanceof Error ? cause.message : "Unable to save. Try again.",
        );
      });
    } else {
      setIndex((current) => current + 1);
    }
  }

  return (
    <form ref={formRef} className="wizard" onSubmit={advance} aria-busy={busy}>
      <ol className="wizard-progress" aria-label="Progress">
        {steps.map((item, position) => (
          <li
            key={item.id}
            aria-current={index === position ? "step" : undefined}
            className={position <= completedThrough ? "is-complete" : ""}
          >
            <button
              type="button"
              className="wizard-step-link"
              disabled={
                busy || (!allowAllSteps && position > completedThrough + 1)
              }
              aria-current={index === position ? "step" : undefined}
              onClick={() => goToStep(position)}
            >
              <span className="wizard-step-number" aria-hidden="true">
                {position <= completedThrough ? "✓" : position + 1}
              </span>
              <span>{item.title}</span>
            </button>
          </li>
        ))}
      </ol>
      <div className="wizard-body">
        <h3 className="wizard-heading" ref={headingRef} tabIndex={-1}>
          {step.title}
        </h3>
        {(validationError ?? error) && (
          <p className="error-message" role="alert">
            {validationError ?? error}
          </p>
        )}
        <fieldset
          className="wizard-fields"
          disabled={busy}
          onChangeCapture={invalidateCurrentStep}
          onClickCapture={(event) => {
            if (
              event.target instanceof Element &&
              event.target.closest("button")
            ) {
              invalidateCurrentStep();
            }
          }}
        >
          {step.content}
        </fieldset>
      </div>
      <footer className="wizard-footer">
        <button
          type="button"
          className="button wizard-cancel"
          disabled={busy}
          onClick={onCancel}
        >
          Cancel
        </button>
        {index > 0 && (
          <button
            type="button"
            className="button"
            disabled={busy}
            onClick={() => {
              goToStep(index - 1);
            }}
          >
            Back
          </button>
        )}
        <button type="submit" className="button button-primary" disabled={busy}>
          {busy
            ? "Saving…"
            : finalStep
              ? finishLabel
              : `Next: ${steps[index + 1]?.title ?? ""}`}
        </button>
      </footer>
    </form>
  );
}
