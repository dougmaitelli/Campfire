import i18n from "../../i18n";
import { Check } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Fieldset } from "../forms/Fieldset";
import { InlineMessage } from "../status/InlineMessage";
import { Button } from "../forms/Button";
import {
  useCallback,
  useEffect,
  useId,
  useRef,
  useState,
  useSyncExternalStore,
} from "react";
import type { CSSProperties, FormEvent, ReactNode } from "react";
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
  children: (content: ReactNode, actions: ReactNode) => ReactNode;
}

export function Wizard({
  allowAllSteps = false,
  steps,
  busy,
  error,
  finishLabel,
  onFinish,
  children,
}: WizardProps) {
  const { t } = useTranslation();
  const [index, setIndex] = useState(0);
  const [completedThrough, setCompletedThrough] = useState(() =>
    allowAllSteps ? steps.length - 1 : -1,
  );
  const [validationError, setValidationError] = useState<string | null>(null);
  const headingRef = useRef<HTMLHeadingElement>(null);
  const [form, setForm] = useState<HTMLFormElement | null>(null);
  const subscribeValidity = useCallback(
    (changed: () => void) => {
      if (!form) return () => {};
      form.addEventListener("input", changed);
      form.addEventListener("change", changed);
      const observer = new MutationObserver(changed);
      observer.observe(form, {
        childList: true,
        subtree: true,
        attributes: true,
      });
      return () => {
        form.removeEventListener("input", changed);
        form.removeEventListener("change", changed);
        observer.disconnect();
      };
    },
    [form],
  );
  const fieldsValid = useSyncExternalStore(
    subscribeValidity,
    () =>
      form !== null &&
      Array.from(form.elements).every((control) => {
        if (
          control instanceof HTMLInputElement ||
          control instanceof HTMLSelectElement ||
          control instanceof HTMLTextAreaElement
        ) {
          return !control.willValidate || control.validity.valid;
        }
        return true;
      }),
    () => false,
  );
  const formID = useId();
  const step = steps[index];
  const finalStep = index === steps.length - 1;

  useEffect(() => {
    headingRef.current?.focus();
  }, [index]);

  if (!step) throw new Error(t("wizard.aWizardMustContainAtLeastOneStep"));
  const stepError = step.validate?.() ?? null;
  const message = validationError ?? error ?? (fieldsValid ? stepError : null);

  function validateCurrentStep() {
    if (busy || !form?.reportValidity()) return false;
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
          cause instanceof Error
            ? cause.message
            : i18n.t("wizard.unableToSaveTryAgain"),
        );
      });
    } else {
      setIndex((current) => current + 1);
    }
  }

  const content = (
    <form
      id={formID}
      ref={setForm}
      className="wizard"
      onSubmit={advance}
      aria-busy={busy}
    >
      <ol
        className={`wizard-progress${steps.length === 1 ? " wizard-progress-single" : ""}`}
        aria-label={t("wizard.progress")}
        style={
          {
            gridTemplateColumns:
              steps.length > 2
                ? `minmax(0, 0.5fr) repeat(${steps.length - 2}, minmax(0, 1fr)) minmax(0, 0.5fr)`
                : `repeat(${steps.length}, minmax(0, 1fr))`,
            "--wizard-progress": `${steps.length > 1 ? (Math.min(steps.length - 1, completedThrough + 1) / (steps.length - 1)) * 100 : 0}%`,
          } as CSSProperties
        }
      >
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
                {position <= completedThrough ? (
                  <Check size={16} aria-hidden="true" />
                ) : (
                  position + 1
                )}
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
        {message && <InlineMessage role="alert">{message}</InlineMessage>}
        <Fieldset
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
        </Fieldset>
      </div>
    </form>
  );

  const actions = (
    <>
      {index > 0 && (
        <Button
          type="button"

          disabled={busy}
          onClick={() => {
            goToStep(index - 1);
          }}
        >
          {t("common.back")}
        </Button>
      )}
      <Button
        type="submit"
        form={formID}
        variant="primary"
        disabled={busy || !fieldsValid || stepError !== null}
        loading={busy}
        loadingLabel={t("wizard.saving")}
      >
        {finalStep
          ? finishLabel
          : t("wizard.next", { stepsIndex1: steps[index + 1]?.title ?? "" })}
      </Button>
    </>
  );

  return children(content, actions);
}
