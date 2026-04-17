// BarChart is a dependency-free SVG bar chart. Keeping charting in-house
// respects the offline-first constraint of the portal (no CDN-hosted
// libraries) and keeps the bundle tiny. It is adequate for operations
// dashboards; complex analytics exports go through CSV.

interface Datum {
  label: string;
  value: number;
}

interface Props {
  data: Datum[];
  width?: number;
  height?: number;
  color?: string;
  ariaLabel: string;
}

export function BarChart({ data, width = 480, height = 200, color = "#2563eb", ariaLabel }: Props) {
  if (data.length === 0) return <div className="muted">No data yet.</div>;
  const max = Math.max(...data.map((d) => d.value), 1);
  const barWidth = (width - 60) / data.length - 8;
  return (
    <svg width={width} height={height} role="img" aria-label={ariaLabel} style={{ background: "var(--card)", borderRadius: 6 }}>
      {data.map((d, i) => {
        const h = ((height - 40) * d.value) / max;
        const x = 50 + i * ((width - 60) / data.length);
        const y = height - 20 - h;
        return (
          <g key={d.label}>
            <rect x={x} y={y} width={barWidth} height={h} fill={color} />
            <text x={x + barWidth / 2} y={y - 4} fontSize={11} textAnchor="middle" fill="#374151">
              {d.value}
            </text>
            <text x={x + barWidth / 2} y={height - 6} fontSize={10} textAnchor="middle" fill="#6b7280">
              {d.label}
            </text>
          </g>
        );
      })}
    </svg>
  );
}

interface LineProps {
  data: Array<{ label: string; value: number }>;
  width?: number;
  height?: number;
  ariaLabel: string;
}

export function LineChart({ data, width = 560, height = 200, ariaLabel }: LineProps) {
  if (data.length === 0) return <div className="muted">No data yet.</div>;
  const max = Math.max(...data.map((d) => d.value), 1);
  const stepX = (width - 60) / Math.max(1, data.length - 1);
  const points = data.map((d, i) => {
    const x = 50 + i * stepX;
    const y = height - 20 - ((height - 40) * d.value) / max;
    return `${x},${y}`;
  });
  return (
    <svg width={width} height={height} role="img" aria-label={ariaLabel} style={{ background: "var(--card)", borderRadius: 6 }}>
      <polyline points={points.join(" ")} fill="none" stroke="#16a34a" strokeWidth={2} />
      {data.map((d, i) => {
        const x = 50 + i * stepX;
        const y = height - 20 - ((height - 40) * d.value) / max;
        return (
          <g key={i}>
            <circle cx={x} cy={y} r={3} fill="#16a34a" />
            <text x={x} y={height - 6} fontSize={10} textAnchor="middle" fill="#6b7280">
              {d.label.slice(5)}
            </text>
          </g>
        );
      })}
    </svg>
  );
}
