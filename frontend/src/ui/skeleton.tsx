// Loading skeleton rows (unified loading state). The pulse is the only
// motion involved — it signals that work is happening without decoration.
export function Skeleton({ rows = 3 }: { rows?: number }) {
  return (
    <div className="flex flex-col gap-4" role="status" aria-busy="true">
      {Array.from({ length: rows }).map((_, i) => (
        <div key={i} className="skeleton-row h-14 animate-pulse rounded-md bg-white/6" />
      ))}
    </div>
  );
}
