const channels = document.querySelector("#channels");
const messages = document.querySelector("#messages");
const failure = document.querySelector("#failure");
const error = document.querySelector("#error");
let selected = "400";
let state;
let rendered = "";
let refreshing = false;

function element(tag, text, className) {
  const node = document.createElement(tag);
  if (text !== undefined) node.textContent = text;
  if (className) node.className = className;
  return node;
}

function showError(cause) {
  error.hidden = !cause;
  error.textContent = cause ? cause.message : "";
}

async function request(path, options = {}) {
  const response = await fetch(`/mock/discord/${path}`, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      "X-Campfire-Mock": "discord",
    },
  });
  if (!response.ok)
    throw new Error(`Mock Discord request failed (${response.status}).`);
  return response.status === 204 ? undefined : response.json();
}

function render() {
  const channel = state.channels.find((item) => item.id === selected);
  document.querySelector("#token").textContent = state.token;
  document.querySelector("#channel-title").textContent = `# ${channel.name}`;
  failure.disabled = channel.id === "401";
  if (document.activeElement !== failure) failure.value = channel.failure;
  document.querySelector("#channel-note").textContent =
    channel.id === "401"
      ? "This fixture channel always lacks Send Messages permission."
      : "Failures apply only to this channel. Return to Normal, then retry failed work in the panel. Rate-limit cooldowns still need to expire.";

  if (!channels.childElementCount) {
    for (const item of state.channels) {
      const button = element("button", `# ${item.name}`);
      button.type = "button";
      button.dataset.channel = item.id;
      button.addEventListener("click", () => {
        selected = item.id;
        render();
      });
      channels.append(button);
    }
  }
  for (const button of channels.children) {
    button.setAttribute(
      "aria-pressed",
      String(button.dataset.channel === selected),
    );
  }

  const items = state.messages.filter(
    (message) => message.channel_id === selected,
  );
  const key = JSON.stringify([selected, items]);
  if (key === rendered) return;
  rendered = key;
  messages.replaceChildren();
  if (!items.length) {
    messages.append(
      element(
        "p",
        "No messages yet. Send a test message or enable a server integration in the panel.",
        "empty",
      ),
    );
  }
  for (const message of items) {
    const article = element("article", undefined, "message");
    article.dataset.messageId = message.id;
    const header = element("div", undefined, "message-header");
    header.append(
      element("strong", "Campfire Bot"),
      element(
        "span",
        `#${message.id}${message.edited_timestamp ? " · edited" : ""}`,
      ),
    );
    const remove = element("button", "Delete message");
    remove.type = "button";
    remove.addEventListener("click", async () => {
      remove.disabled = true;
      try {
        await request(`messages/${message.id}`, { method: "DELETE" });
        await refresh();
      } catch (cause) {
        showError(cause);
        remove.disabled = false;
      }
    });
    header.append(remove);
    article.append(header);
    if (message.content) article.append(element("p", message.content));
    for (const embed of message.embeds ?? []) {
      const card = element("div", undefined, "embed");
      if (
        Number.isInteger(embed.color) &&
        embed.color >= 0 &&
        embed.color <= 0xffffff
      ) {
        card.style.borderLeftColor = `#${embed.color.toString(16).padStart(6, "0")}`;
      }
      if (embed.title) card.append(element("h3", embed.title));
      if (embed.description) card.append(element("p", embed.description));
      if (embed.fields?.length) {
        const fields = element("dl", undefined, "fields");
        for (const field of embed.fields) {
          const group = element("div", undefined, "field");
          group.append(element("dt", field.name), element("dd", field.value));
          fields.append(group);
        }
        card.append(fields);
      }
      if (embed.footer?.text) card.append(element("footer", embed.footer.text));
      article.append(card);
    }
    for (const attachment of message.attachments ?? []) {
      const group = element("figure", undefined, "attachment");
      if (attachment.content_type.startsWith("image/")) {
        const image = element("img");
        image.src = attachment.url;
        image.alt = attachment.filename;
        image.loading = "lazy";
        group.append(image);
      } else if (attachment.content_type.startsWith("video/")) {
        const video = element("video");
        video.controls = true;
        video.preload = "metadata";
        video.setAttribute("aria-label", attachment.filename);
        const source = element("source");
        source.src = attachment.url;
        source.type = attachment.content_type;
        video.append(source);
        group.append(video);
      }
      const caption = element("figcaption");
      const download = element("a", attachment.filename);
      download.href = attachment.url;
      download.download = attachment.filename;
      caption.append(download);
      group.append(caption);
      article.append(group);
    }
    messages.append(article);
  }
}

async function refresh() {
  if (refreshing) return;
  refreshing = true;
  try {
    state = await request("state");
    showError(null);
    render();
  } catch (cause) {
    showError(cause);
  } finally {
    refreshing = false;
  }
}

failure.addEventListener("change", async () => {
  const channel = selected;
  failure.disabled = true;
  try {
    await request(`channels/${channel}`, {
      method: "PUT",
      body: JSON.stringify({ failure: failure.value }),
    });
    await refresh();
  } catch (cause) {
    showError(cause);
  } finally {
    failure.disabled = selected === "401";
  }
});

document.querySelector("#copy-token").addEventListener("click", async () => {
  try {
    await navigator.clipboard.writeText(state.token);
    showError(null);
  } catch {
    showError(
      new Error("Clipboard unavailable. Select and copy the fake token above."),
    );
  }
});

void refresh();
setInterval(refresh, 1000);
