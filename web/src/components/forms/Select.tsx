import {
  Children,
  Fragment,
  isValidElement,
  useId,
  useState,
  type ComponentProps,
  type ReactNode,
} from "react";
import * as SelectPrimitive from "@radix-ui/react-select";
import { Check, ChevronDown, ChevronUp } from "lucide-react";
import "./form-controls.css";
import "./select.css";

type Props = Omit<
  ComponentProps<typeof SelectPrimitive.Trigger>,
  "value" | "defaultValue" | "onChange" | "children" | "ref"
> & {
  value?: string | number;
  defaultValue?: string | number;
  onValueChange?: (value: string) => void;
  children: ReactNode;
  name?: string;
  required?: boolean;
  autoComplete?: string;
};

function optionsFrom(children: ReactNode): ComponentProps<"option">[] {
  return Children.toArray(children).flatMap((child) => {
    if (!isValidElement<ComponentProps<"option">>(child)) return [];
    if (child.type === Fragment) return optionsFrom(child.props.children);
    return child.type === "option" ? [child.props] : [];
  });
}

function optionValue(option: ComponentProps<"option">): string {
  if (typeof option.value === "string" || typeof option.value === "number")
    return String(option.value);
  return Children.toArray(option.children)
    .map((child) =>
      typeof child === "string" || typeof child === "number"
        ? String(child)
        : "",
    )
    .join("");
}

export function Select({
  className,
  children,
  value,
  defaultValue,
  onValueChange,
  name,
  required,
  disabled,
  autoComplete,
  form,
  dir,
  ...props
}: Props) {
  const [trigger, setTrigger] = useState<HTMLButtonElement | null>(null);
  const emptyValue = `${useId()}-empty`;
  const options = optionsFrom(children);
  const encode = (value: string | number) => String(value) || emptyValue;
  const rootValue = (value: string | number) =>
    String(value) || (required || name ? "" : emptyValue);
  return (
    <SelectPrimitive.Root
      value={value === undefined ? undefined : rootValue(value)}
      defaultValue={rootValue(
        defaultValue ??
          options.find((option) => !option.disabled)?.value?.toString() ??
          "",
      )}
      onValueChange={(value) =>
        onValueChange?.(value === emptyValue ? "" : value)
      }
      name={name}
      required={required}
      disabled={disabled}
      autoComplete={autoComplete}
      form={form}
      dir={dir === "rtl" || dir === "ltr" ? dir : undefined}
    >
      <SelectPrimitive.Trigger
        {...props}
        ref={setTrigger}
        className={["select", className].filter(Boolean).join(" ")}
      >
        <SelectPrimitive.Value
          placeholder={options.find((option) => option.value === "")?.children}
        />
        <SelectPrimitive.Icon className="select-chevron">
          <ChevronDown size={16} aria-hidden="true" />
        </SelectPrimitive.Icon>
      </SelectPrimitive.Trigger>
      <SelectPrimitive.Portal
        container={trigger?.closest("dialog") ?? undefined}
      >
        <SelectPrimitive.Content
          className="select-menu"
          position="popper"
          sideOffset={6}
          collisionPadding={12}
          onEscapeKeyDown={(event) => event.stopPropagation()}
        >
          <SelectPrimitive.ScrollUpButton
            className="select-scroll"
            aria-hidden="true"
          >
            <ChevronUp size={16} aria-hidden="true" />
          </SelectPrimitive.ScrollUpButton>
          <SelectPrimitive.Viewport className="select-options">
            {options.map((option, index) => (
              <SelectPrimitive.Item
                key={`${optionValue(option)}:${index}`}
                value={encode(optionValue(option))}
                disabled={option.disabled}
                className="select-option"
              >
                <SelectPrimitive.ItemText>
                  {option.children}
                </SelectPrimitive.ItemText>
                <SelectPrimitive.ItemIndicator
                  className="select-check"
                  aria-hidden="true"
                >
                  <Check size={16} aria-hidden="true" />
                </SelectPrimitive.ItemIndicator>
              </SelectPrimitive.Item>
            ))}
          </SelectPrimitive.Viewport>
          <SelectPrimitive.ScrollDownButton
            className="select-scroll"
            aria-hidden="true"
          >
            <ChevronDown size={16} aria-hidden="true" />
          </SelectPrimitive.ScrollDownButton>
        </SelectPrimitive.Content>
      </SelectPrimitive.Portal>
    </SelectPrimitive.Root>
  );
}
