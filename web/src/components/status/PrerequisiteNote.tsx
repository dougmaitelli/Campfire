import type { ComponentProps } from "react";
import "./prerequisite-note.css";

export function PrerequisiteNote({ className, ...props }: ComponentProps<"p">) {
  return (
    <p
      {...props}
      className={["prerequisite-note", className].filter(Boolean).join(" ")}
    />
  );
}
