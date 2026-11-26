import type { ComponentProps } from "react";
import { TextInput } from "./TextInput";

// Native email validation rejects internationalized mailbox names.
export function EmailInput(props: ComponentProps<"input">) {
  return (
    <TextInput
      {...props}
      type="text"
      inputMode="email"
      autoComplete="email"
      autoCapitalize="none"
      spellCheck={false}
      pattern=".+@.+"
    />
  );
}
