import type { ComponentProps } from "react";
import "./progress-bar.css";

type Props =
  | ({ as?: "progress" } & ComponentProps<"progress">)
  | ({ as: "meter" } & ComponentProps<"meter">);

export function ProgressBar(props: Props) {
  const className = ["progress-bar", props.className].filter(Boolean).join(" ");
  if (props.as === "meter") {
    const { as: Element, ...attributes } = props;
    return <Element {...attributes} className={className} />;
  }
  const { as: Element = "progress", ...attributes } = props;
  return <Element {...attributes} className={className} />;
}
