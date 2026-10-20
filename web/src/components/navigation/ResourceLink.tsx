import type { ReactNode } from "react";

export function ResourceLink({
  href,
  children,
}: {
  href: string | undefined;
  children: ReactNode;
}) {
  return href ? <a href={href}>{children}</a> : <>{children}</>;
}
