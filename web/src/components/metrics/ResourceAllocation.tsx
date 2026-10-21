import type { ReactNode } from "react";
import "./resource-allocation.css";

export function ResourceAllocation({ children }: { children: ReactNode }) {
  return <span className="resource-allocation">/ {children}</span>;
}
