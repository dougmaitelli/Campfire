// Docker prefixes each log record with its original RFC3339Nano timestamp.
// Buffer only a possible prefix so partial prompts and ANSI output stay live.
export class ConsoleTimestamps {
  private prefix = "";
  private lineStart = true;

  reset() {
    this.prefix = "";
    this.lineStart = true;
  }

  flush(): string {
    const pending = this.prefix;
    this.prefix = "";
    this.lineStart = false;
    return pending;
  }

  format(content: string): string {
    let output = "";
    for (const character of content) {
      if (!this.lineStart) {
        output += character;
        this.lineStart = character === "\n";
        continue;
      }
      this.prefix += character;
      if (character === " ") {
        const match =
          /^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z) $/.exec(
            this.prefix,
          );
        const timestamp = match ? new Date(match[1]!) : null;
        if (timestamp && Number.isFinite(timestamp.getTime())) {
          const time = [
            timestamp.getHours(),
            timestamp.getMinutes(),
            timestamp.getSeconds(),
          ]
            .map((part) => String(part).padStart(2, "0"))
            .join(":");
          output += `[${time}] `;
          this.prefix = "";
          this.lineStart = false;
          continue;
        }
      }
      if (!/^[\dT:.Z-]+$/.test(this.prefix) || this.prefix.length > 35) {
        output += this.flush();
        this.lineStart = character === "\n";
      }
    }
    return output;
  }
}
