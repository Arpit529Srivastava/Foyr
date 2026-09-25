import Link from "next/link";

/** The Foyr doorway mark: a lintel, two jambs and a half-open door, on a 16px pixel grid. */
export function FoyrMark({ size = 16, className }: { size?: number; className?: string }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 16 16"
      fill="currentColor"
      shapeRendering="crispEdges"
      aria-hidden="true"
      className={className}
    >
      <rect x="2" y="1" width="12" height="2" />
      <rect x="2" y="3" width="2" height="12" />
      <rect x="12" y="3" width="2" height="12" />
      <rect x="5" y="4" width="4" height="11" />
    </svg>
  );
}

export function FoyrLockup({ className = "" }: { className?: string }) {
  return (
    <Link href="/" aria-label="Foyr home" className={`flex items-center gap-2 text-white ${className}`}>
      <FoyrMark />
      <span className="text-[18px] leading-5 font-[560] tracking-[-0.04em]">foyr</span>
    </Link>
  );
}
