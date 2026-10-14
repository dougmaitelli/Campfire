import { navigation } from "../../routing/navigation";
import type { NavigationItem } from "../../routing/navigation";
import { NavigationIcon } from "./NavigationIcon";

export function Sidebar({ page }: { page: NavigationItem }) {
  return (
    <aside className="sidebar">
      <a className="brand" href="#dashboard" aria-label="Campfire dashboard">
        <span className="brand-mark">
          <NavigationIcon name="campfire" />
        </span>
        <span>Campfire</span>
      </a>
      <nav className="navigation" aria-label="Main navigation">
        {navigation.map((item) => (
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
      <div className="sidebar-footer">Game server management</div>
    </aside>
  );
}
