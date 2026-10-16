import { navigation } from "../../routing/navigation";
import type { NavigationItem } from "../../routing/navigation";
import { NavigationIcon } from "./NavigationIcon";
import { useAuthentication } from "../../hooks/useAuthentication";
import { AccountMenu } from "./AccountMenu";

export function Sidebar({ page }: { page: NavigationItem }) {
  const auth = useAuthentication();
  return (
    <aside className="sidebar">
      <a className="brand" href="#dashboard" aria-label="Campfire dashboard">
        <span className="brand-mark">
          <NavigationIcon name="campfire" />
        </span>
        <span>Campfire</span>
      </a>
      <nav className="navigation" aria-label="Main navigation">
        {navigation
          .filter(
            (item) =>
              item.id !== "notifications" &&
              (auth.session?.admin === true ||
                item.id === "servers" ||
                item.id === "dashboard" ||
                item.id === "schedules"),
          )
          .map((item) => (
            <a
              key={item.id}
              href={`#${item.id}`}
              className="navigation-link"
              aria-current={page.id === item.id ? "page" : undefined}
            >
              <NavigationIcon name={item.icon} />
              <span>{item.label}</span>
            </a>
          ))}
      </nav>
      <AccountMenu />
    </aside>
  );
}
