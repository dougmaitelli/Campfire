import { useEffect, useId, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import "./resource-graph.css";

export interface GraphHistory {
  from: number;
  to: number;
  interval: number;
  points: { time: number; value: number | null }[];
}

export function ResourceGraph({
  label,
  history,
  format,
  allocation,
  loading,
}: {
  label: string;
  history: GraphHistory | undefined;
  format: (value: number) => string;
  allocation: number;
  loading: boolean;
}) {
  const { t } = useTranslation();
  const titleID = useId();
  const descriptionID = useId();
  const plot = useRef<SVGSVGElement>(null);
  const [width, setWidth] = useState(600);
  const hasPoints =
    history?.points.some((point) => point.value !== null) ?? false;
  useEffect(() => {
    const element = plot.current;
    if (!element) return;
    const observer = new ResizeObserver(([entry]) => {
      if (entry) setWidth(Math.max(1, entry.contentRect.width));
    });
    observer.observe(element);
    return () => observer.disconnect();
  }, [hasPoints]);
  const points = history?.points.filter((point) => point.value !== null) ?? [];
  const latest = points.at(-1)?.value;
  const maximum = Math.max(
    1,
    allocation,
    ...points.map((point) => point.value ?? 0),
  );
  const x = (time: number) =>
    history
      ? Math.max(
          0,
          Math.min(
            width,
            ((time - history.from) / Math.max(1, history.to - history.from)) *
              width,
          ),
        )
      : 0;
  const y = (value: number) => 160 - (value / maximum) * 160;
  const path =
    history?.points
      .map((point, index) => {
        if (point.value === null) return "";
        const previous = history.points[index - 1];
        const connected =
          previous &&
          previous.value !== null &&
          point.time - previous.time <= history.interval * 1.5;
        return `${connected ? "L" : "M"}${x(point.time)},${y(point.value)}`;
      })
      .join(" ") ?? "";
  const timeLabel = (time: number) =>
    new Date(time * 1000).toLocaleTimeString(undefined, {
      hour: "2-digit",
      minute: "2-digit",
    });
  return (
    <section className="resource-graph" aria-labelledby={titleID}>
      <header className="resource-graph-header">
        <div>
          <h2 id={titleID}>{label}</h2>
          <span>{t("resourceHistory.lastHour")}</span>
        </div>
        {latest !== undefined && latest !== null && (
          <strong>{format(latest)}</strong>
        )}
      </header>
      {history && points.length > 0 ? (
        <>
          <div className="resource-graph-plot">
            <div className="resource-graph-scale" aria-hidden="true">
              {[1, 0.5, 0].map((ratio) => (
                <span key={ratio}>{format(maximum * ratio)}</span>
              ))}
            </div>
            <svg
              ref={plot}
              viewBox={`0 -4 ${width} 168`}
              role="img"
              aria-label={t("resourceHistory.chartLabel", { resource: label })}
              aria-describedby={descriptionID}
            >
              <desc id={descriptionID}>
                {t("resourceHistory.summary", {
                  count: points.length,
                  value: format(latest!),
                })}
              </desc>
              {[0, 80, 160].map((height) => (
                <line key={height} x1={0} x2={width} y1={height} y2={height} />
              ))}
              <path d={path} />
              {points.map((point) => (
                <circle
                  key={point.time}
                  cx={x(point.time)}
                  cy={y(point.value!)}
                  r={3}
                >
                  <title>{`${new Date(point.time * 1000).toLocaleString()}: ${format(point.value!)}`}</title>
                </circle>
              ))}
            </svg>
          </div>
          <div className="resource-graph-times" aria-hidden="true">
            {[history.from, (history.from + history.to) / 2, history.to].map(
              (time) => (
                <span key={time}>{timeLabel(time)}</span>
              ),
            )}
          </div>
        </>
      ) : (
        <p className="resource-graph-empty" role="status">
          {t(loading ? "resourceHistory.loading" : "resourceHistory.empty")}
        </p>
      )}
    </section>
  );
}
