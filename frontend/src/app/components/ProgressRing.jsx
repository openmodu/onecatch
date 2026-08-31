export default function ProgressRing({ ratio = 0, size = 20, radius = 7.5, stroke = 2.25, className = "text-primary", trackClassName = "text-muted" }) {
  const circumference = 2 * Math.PI * radius;
  const swept = circumference * Math.min(1, Math.max(0, Number(ratio) || 0));
  const center = size / 2;

  return <svg className="shrink-0 -rotate-90" width={size} height={size} viewBox={`0 0 ${size} ${size}`} aria-hidden="true" focusable="false">
    <circle cx={center} cy={center} r={radius} fill="none" strokeWidth={stroke} className={trackClassName} stroke="currentColor" />
    <circle cx={center} cy={center} r={radius} fill="none" strokeWidth={stroke} strokeLinecap="round" strokeDasharray={`${swept} ${circumference}`} className={`transition-[stroke-dasharray] duration-150 ${className}`} stroke="currentColor" />
  </svg>;
}
