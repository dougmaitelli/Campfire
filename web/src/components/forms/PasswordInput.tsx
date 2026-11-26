import { useId, type ComponentProps } from "react";
import { Check, Circle } from "lucide-react";
import { useTranslation } from "react-i18next";
import { AccountLimits } from "../../config/policies";
import { TextInput } from "./TextInput";
import "./password-input.css";

export function passwordPolicy(value: string) {
  return {
    minimum: [...value].length >= AccountLimits.PasswordMin,
    uppercase: !AccountLimits.PasswordRequireUppercase || /\p{Lu}/u.test(value),
    lowercase: !AccountLimits.PasswordRequireLowercase || /\p{Ll}/u.test(value),
    number: !AccountLimits.PasswordRequireNumber || /\p{Nd}/u.test(value),
    symbol: !AccountLimits.PasswordRequireSymbol || /[\p{P}\p{S}]/u.test(value),
  };
}

export function PasswordInput({
  value,
  "aria-describedby": describedBy,
  ...props
}: Omit<ComponentProps<typeof TextInput>, "type" | "value"> & {
  value: string;
}) {
  const { t } = useTranslation();
  const rulesID = useId();
  const policy = passwordPolicy(value);
  const rules = [
    {
      met: policy.minimum,
      label: t("passwordInput.minimum", { count: AccountLimits.PasswordMin }),
    },
    ...(
      [
        ["uppercase", AccountLimits.PasswordRequireUppercase],
        ["lowercase", AccountLimits.PasswordRequireLowercase],
        ["number", AccountLimits.PasswordRequireNumber],
        ["symbol", AccountLimits.PasswordRequireSymbol],
      ] as const
    )
      .filter(([, required]) => required)
      .map(([rule]) => ({
        met: policy[rule],
        label: t(`passwordInput.${rule}`),
      })),
  ];
  return (
    <div className="password-input">
      <TextInput
        autoComplete="new-password"
        {...props}
        type="password"
        value={value}
        aria-describedby={
          [describedBy, value && rulesID].filter(Boolean).join(" ") || undefined
        }
      />
      {value && (
        <ul
          id={rulesID}
          className="password-input-rules"
          aria-live="polite"
          aria-label={t("passwordInput.rules")}
        >
          {rules.map((rule) => (
            <li
              key={rule.label}
              className={rule.met ? "password-rule-met" : undefined}
            >
              {rule.met ? (
                <Check size={14} aria-hidden="true" />
              ) : (
                <Circle size={14} aria-hidden="true" />
              )}
              <span className="visually-hidden">
                {t(rule.met ? "passwordInput.met" : "passwordInput.unmet")}
                :{" "}
              </span>
              {rule.label}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
