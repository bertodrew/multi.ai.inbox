const cols = 4;
const rows = 3;
const lit = 6; // the box that just received mail

/** Wall of numbered P.O. boxes, one per agent; one lights up when mail arrives. */
export function PoBoxWall() {
  const boxes = Array.from({ length: cols * rows }, (_, i) => i);
  return (
    <svg
      viewBox="0 0 420 330"
      role="img"
      aria-labelledby="pobox-title"
      className="h-auto w-full max-w-md"
    >
      <title id="pobox-title">A wall of twelve numbered mail boxes, one per agent. Box 0047 has new mail.</title>
      <rect x="4" y="4" width="412" height="322" rx="18" fill="var(--card)" stroke="var(--border)" strokeWidth="2" />
      {boxes.map((i) => {
        const x = 22 + (i % cols) * 98;
        const y = 22 + Math.floor(i / cols) * 100;
        const isLit = i === lit;
        const number = String(41 + i).padStart(4, "0");
        return (
          <g key={i}>
            <rect x={x} y={y} width="86" height="88" rx="10" fill="var(--bg)" stroke="var(--border)" strokeWidth="2" />
            <rect x={x + 12} y={y + 14} width="62" height="8" rx="4" fill="var(--fg)" opacity="0.85" />
            {isLit ? (
              <rect
                className="slot-glow"
                x={x + 8}
                y={y + 10}
                width="70"
                height="16"
                rx="8"
                fill="var(--accent)"
              />
            ) : null}
            <rect x={x + 12} y={y + 58} width="62" height="20" rx="4" fill={isLit ? "var(--accent)" : "var(--card)"} stroke="var(--border)" />
            <text
              x={x + 43}
              y={y + 72}
              textAnchor="middle"
              fontFamily="var(--font-mono)"
              fontSize="11"
              fill={isLit ? "var(--accent-fg)" : "var(--muted)"}
            >
              {number}
            </text>
            <circle cx={x + 43} cy={y + 42} r="3" fill="var(--muted)" />
          </g>
        );
      })}
    </svg>
  );
}
