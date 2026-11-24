import { cn } from "@/lib/utils";

interface FixedWidthProps {
  /** The longest string this slot will ever hold. */
  reserve: string;
  className?: string;
  contentClassName?: string;
  children: React.ReactNode;
}

/**
 * Holds the width of `reserve` and lays the real content over it, so a control
 * whose label changes -- "Sync now" to "Syncing", one month name to another --
 * never resizes and never nudges its neighbours.
 *
 * The reservation is a `::before` rather than a hidden element: a
 * pseudo-element stays out of both the accessibility tree and `textContent`,
 * so it cannot leak into an accessible name or a test query.
 */
export function FixedWidth({ reserve, className, contentClassName, children }: FixedWidthProps) {
  return (
    <span
      data-reserve={reserve}
      className={cn(
        "before:invisible before:content-[attr(data-reserve)] relative flex items-center",
        className,
      )}
    >
      <span className={cn("absolute inset-0 flex items-center", contentClassName)}>{children}</span>
    </span>
  );
}
