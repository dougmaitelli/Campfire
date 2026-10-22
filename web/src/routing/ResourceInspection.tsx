import {
  createContext,
  Suspense,
  useContext,
  useState,
  type ReactNode,
} from "react";
import { getRoute, parseResourceRoute } from "./routeRegistry";

const InspectionContext = createContext<((href: string) => void) | null>(null);

export function useResourceInspection() {
  return useContext(InspectionContext);
}

export function ResourceInspectionProvider({
  children,
}: {
  children: ReactNode;
}) {
  const [targets, setTargets] = useState<string[]>([]);
  return (
    <InspectionContext.Provider
      value={(href) => setTargets((current) => [...current, href])}
    >
      {children}
      {targets.map((href, index) => (
        <Suspense key={`${index}:${href}`} fallback={null}>
          <ResourceInspection
            href={href}
            onClose={() => setTargets((current) => current.slice(0, index))}
          />
        </Suspense>
      ))}
    </InspectionContext.Provider>
  );
}

function ResourceInspection({
  href,
  onClose,
}: {
  href: string;
  onClose: () => void;
}) {
  const { page, id } = parseResourceRoute(href);
  const Drawer = getRoute(page)?.resource?.Drawer;
  return Drawer && id ? <Drawer id={id} onClose={onClose} /> : null;
}
