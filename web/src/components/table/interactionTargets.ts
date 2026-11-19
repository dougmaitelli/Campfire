/** Native controls keep their own clicks, keys, and drag gestures. */
export const interactiveSelector =
  "a, button, input, label, select, textarea, summary, details, [role=button], [contenteditable]:not([contenteditable=false])";

/** Check descendants only: an enclosing disclosure is not a control in this surface. */
export function isInteractionTarget(
  target: EventTarget | null,
  boundary: Element,
  selector = interactiveSelector,
): boolean {
  if (!(target instanceof Element) || !boundary.contains(target)) return false;
  for (
    let element: Element | null = target;
    element && element !== boundary;
    element = element.parentElement
  ) {
    if (element.matches(selector)) return true;
  }
  return false;
}
