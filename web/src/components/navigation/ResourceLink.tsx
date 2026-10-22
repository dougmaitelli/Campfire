import type { ReactNode } from "react";
import { useResourceInspection } from "../../routing/ResourceInspection";

export function ResourceLink({
  href,
  children,
}: {
  href: string | undefined;
  children: ReactNode;
}) {
  const inspect = useResourceInspection();
  return href ? (
    <a
      href={href}
      onClick={(event) => {
        if (
          !inspect ||
          event.button !== 0 ||
          event.metaKey ||
          event.ctrlKey ||
          event.shiftKey ||
          event.altKey
        )
          return;
        event.preventDefault();
        event.stopPropagation();
        inspect(href);
      }}
    >
      {children}
    </a>
  ) : (
    <>{children}</>
  );
}
