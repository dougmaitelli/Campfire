---
title: Public status pages
description: Share selected servers' availability without requiring visitors to sign in.
---

Status pages let you share server availability with your community. Only
administrators can manage pages; anyone with a page's URL can view it without
signing in.

## Create and share a page

1. Open **Status pages** in the administrator sidebar.
2. Choose **Create status page** and enter a title and unique URL slug. The slug
   forms the last part of the public address: `/status/<slug>`.
3. Choose **Add server** for each entry, select a server, and review its public
   display name. These names are separate from the servers' names in the panel.
4. Order the entries using the up and down arrows.
5. Choose **Create public page**. The page becomes public immediately.
6. Use the copy icon beside the public URL to copy it, or open the link to view
   the page. A checkmark confirms that the URL was copied.

You can create up to 100 pages, each listing up to 50 servers.

## Edit or remove a page

Click a page's row to change its title, URL slug, server selection, public names,
or ordering, then save your changes. Changes become public immediately. Changing
the slug breaks the previous URL, so share the new address with your visitors.

Remove a server from the selection to stop listing it on that page. This does not
stop or delete the server. Deleting a server from Campfire also removes it from
status pages.

To remove a page, click its row, choose **Delete** in the drawer, and confirm. Its public URL
will no longer show server availability. Deleting a page does not delete any
servers.

## Understand the statuses

- **Online:** the server reports that it is running and ready.
- **Offline:** the server is not ready to serve players, for example because it
  is stopped, starting, or installing.
- **Unknown:** Campfire cannot confirm the server's current availability.

Pages update automatically. If an update fails or the information becomes too
old, availability changes to **Unknown** rather than continuing to show an old
status. Integration readings also stop displaying when they become too old;
refreshing the page does not make an old reading current again.

**Online** does not guarantee that players can connect from the internet. Status
pages also depend on the Campfire panel being reachable; they cannot report an
outage if the panel itself is unavailable.

## What visitors can see

Visitors can see the page title, the public names of the selected servers, their
availability, and when they were last checked. Each server also shows its game
artwork, each server's configured or inherited public connection address, and all
available integration state fields,
such as player counts, player lists, and the current map. Missing or unavailable
artwork uses a generic server icon. The copy icon beside a game address copies it
exactly as configured. Port allocations are not appended to it.

Visitors cannot use the page to manage servers or view their configuration,
port allocations, resource usage, or logs.

Review the title, public names, artwork, game addresses, and integration fields before saving. Anyone can share the URL, and
removing a page cannot recall information visitors have already seen or saved.
