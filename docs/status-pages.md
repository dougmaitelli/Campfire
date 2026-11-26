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

## Publish a live Discord card

In the status-page drawer, turn on **Publish a live Discord card** and choose a
channel from the panel's [Discord connection](/Campfire/discord/). The preview
uses the draft title and public server names in their selected order. Saving
publishes one Discord message showing each server as online, offline, or unknown.
Campfire edits that message as availability or page settings change.

Reopen the drawer to check publishing status or retry a failed delivery. Changing
channels moves the card. Turning publishing off or deleting the page requests
removal of the Discord message. The public web page remains available when only
Discord publishing is turned off.

Deleting the page revokes its public URL immediately. If Discord cannot remove
the message, open **Integrations** and find the page under **Discord cards awaiting
removal**. Restore bot access to the channel and use the retry action. The entry
disappears once Campfire confirms removal.

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

## Availability history

Faint uptime bars behind the bottom of each server row show the last 24 hours,
from oldest on the left to newest on the right. Green means observed online and ready, red
means observed offline or not ready, and yellow means unknown. Mixed intervals
show the proportion of each state. The accessible graph description includes
coverage percentages and the history's end time.

History is retained in completed five-minute intervals and displayed as wider
15-minute bars. Its newest point can be up to five minutes behind the current
status. Campfire records elapsed availability with its three-second lifecycle
collector, independently of resource sampling and public-page visits. Coverage
is split across five-minute boundaries without dropping seconds, and stored in
the resource-history buckets under the same retention setting (30 days by
default). Resource compaction preserves availability coverage. Only public
status pages currently display it.

Data predating availability collection, missed observations, disconnected
Sparks, and panel downtime remain unknown; older CPU readings are not treated
as proof of uptime. These are panel-observed availability estimates, not an
external connectivity check or an SLA calculation. History stays anchored to
its recorded time if a page refresh fails.

## What visitors can see

Visitors can see the page title, the public names of the selected servers, their
current availability, 24-hour availability history, and when they were last
checked. Each server also shows its game
artwork, each server's configured or inherited public connection address, and all
available integration state fields,
such as player counts, player lists, and the current map. Missing or unavailable
artwork uses a generic server icon. The copy icon beside a game address copies it
exactly as configured. Port allocations are not appended to it.

Visitors cannot use the page to manage servers or view their configuration,
port allocations, resource usage, or logs.

Review the title, public names, artwork, game addresses, and integration fields before saving. Anyone can share the URL, and
removing a page cannot recall information visitors have already seen or saved.
