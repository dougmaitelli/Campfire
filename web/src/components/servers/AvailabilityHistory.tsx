import { useEffect, useId, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import type { AvailabilityHistory as History } from "../../api/statusPages";

// Historical coverage remains historical when polling fails; it must never be
// stretched to "now" or recolored to match the latest lifecycle status.
export function AvailabilityHistory({ history }: { history: History }) {
  const { t, i18n } = useTranslation();
  const clipID = useId();
  const svgRef = useRef<SVGSVGElement>(null);
  const [aspectRatio, setAspectRatio] = useState(20);
  useEffect(() => {
    const svg = svgRef.current;
    if (!svg || typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(([entry]) => {
      if (entry && entry.contentRect.height > 0) {
        setAspectRatio(entry.contentRect.width / entry.contentRect.height);
      }
    });
    observer.observe(svg);
    return () => observer.disconnect();
  }, []);
  const duration = history.to - history.from;
  const online = history.points.reduce(
    (sum, point) => sum + point.online_seconds,
    0,
  );
  const offline = history.points.reduce(
    (sum, point) => sum + point.offline_seconds,
    0,
  );
  const percent = new Intl.NumberFormat(i18n.resolvedLanguage ?? "en", {
    style: "percent",
    maximumFractionDigits: 1,
  });
  const label = t("statusPages.historySummary", {
    online: percent.format(online / duration),
    offline: percent.format(offline / duration),
    unknown: percent.format((duration - online - offline) / duration),
    time: new Date(history.to * 1000).toLocaleString(),
  });
  // Combine three retained samples into wider, fifteen-minute display bars.
  const samplesPerBar = 3;
  const interval = history.interval * samplesPerBar;
  const points = Array.from(
    { length: history.points.length / samplesPerBar },
    (_, index) =>
      history.points
        .slice(index * samplesPerBar, (index + 1) * samplesPerBar)
        .reduce(
          (sum, point) => ({
            online_seconds: sum.online_seconds + point.online_seconds,
            offline_seconds: sum.offline_seconds + point.offline_seconds,
          }),
          { online_seconds: 0, offline_seconds: 0 },
        ),
  );
  const width = 1.0;
  const gap = 0.55;
  const plotWidth = points.length * width + (points.length - 1) * gap;
  const paths = { online: "", offline: "", unknown: "" };
  let outlines = "";
  for (const [index, point] of points.entries()) {
    const unknown = interval - point.online_seconds - point.offline_seconds;
    const x = index * (width + gap);
    const rx = width / 2;
    // Match horizontal and vertical radii in screen pixels despite SVG scaling.
    const ry = Math.min(interval, (rx * aspectRatio * interval) / plotWidth);
    // Clip the complete bar, not its individual colors: transitions stay square.
    outlines += `M${x},${interval}V${ry}a${rx},${ry} 0 0 1 ${width},0V${interval}Z`;
    const bar = (y: number, height: number) =>
      height > 0 ? `M${x},${y}h${width}v${height}h-${width}Z` : "";
    paths.unknown += bar(0, unknown);
    paths.offline += bar(unknown, point.offline_seconds);
    paths.online += bar(unknown + point.offline_seconds, point.online_seconds);
  }
  return (
    <svg
      ref={svgRef}
      className="public-status-history"
      viewBox={`0 0 ${plotWidth} ${interval}`}
      preserveAspectRatio="none"
      role="img"
      aria-label={label}
      focusable="false"
    >
      <title>{label}</title>
      <defs>
        <clipPath id={clipID}>
          <path d={outlines} />
        </clipPath>
      </defs>
      <g clipPath={`url(#${clipID})`}>
        <path className="history-unknown" d={paths.unknown} />
        <path className="history-offline" d={paths.offline} />
        <path className="history-online" d={paths.online} />
      </g>
    </svg>
  );
}
