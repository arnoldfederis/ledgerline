import { Skeleton } from "@/components/ui/skeleton";

/**
 * Mirrors the loaded report element for element: same wrappers, same padding,
 * and row boxes pinned to the heights the real ones measure (128px stat cell,
 * 62px bar row, 72px section header, 37px table row). Nothing shifts when the
 * data lands.
 */
export function ReportSkeleton() {
  return (
    <>
      <div className="grid grid-cols-2 divide-x divide-y border-y lg:grid-cols-4 lg:divide-y-0">
        {Array.from({ length: 4 }, (_, index) => (
          <div key={index} className="flex flex-col gap-2 px-5 py-6">
            <Skeleton className="h-2.5 w-16" />
            {/* 2.4rem, not the 1.65rem font size: NumberFlow reserves room
                above and below the digits for the slide animation. */}
            <Skeleton className="h-[2.4rem] w-36" />
            <Skeleton className="h-4 w-24" />
          </div>
        ))}
      </div>

      <section>
        <SectionHeader titleWidth="w-32" metaWidth="w-56" />

        <div className="flex h-[29px] items-start justify-between px-5">
          <Skeleton className="h-2.5 w-5" />
          <Skeleton className="h-2.5 w-10" />
        </div>

        <ul className="flex flex-col border-t">
          {Array.from({ length: 8 }, (_, index) => (
            <li key={index} className="flex flex-col gap-2 px-5 py-3">
              <div className="flex h-5 items-center justify-between gap-4">
                <Skeleton className="h-3 w-40" />
                <span className="flex items-center gap-3">
                  <Skeleton className="h-3 w-20" />
                  <Skeleton className="h-3 w-10" />
                </span>
              </div>
              <Skeleton className="h-2.5 w-full rounded-none" />
            </li>
          ))}
        </ul>

        <div className="flex h-[17px] items-center border-t px-5 py-3 box-content">
          <Skeleton className="h-2.5 w-80" />
        </div>
      </section>

      <section className="border-t">
        <SectionHeader titleWidth="w-16" metaWidth="w-64" />

        <div className="flex h-9 items-center gap-6 border-b px-5">
          <Skeleton className="h-2.5 w-20" />
          <span className="flex-1" />
          {[16, 14, 16, 12, 12].map((width, index) => (
            <Skeleton key={index} className="h-2.5" style={{ width: `${width * 4}px` }} />
          ))}
        </div>

        {Array.from({ length: 8 }, (_, index) => (
          <div
            key={index}
            className="flex h-[37px] items-center gap-6 border-b px-5 last:border-b-0"
          >
            <Skeleton className="h-3 w-44" />
            <span className="flex-1" />
            {[16, 14, 16, 12, 12].map((width, cell) => (
              <Skeleton key={cell} className="h-3" style={{ width: `${width * 4}px` }} />
            ))}
          </div>
        ))}
      </section>
    </>
  );
}

function SectionHeader({ titleWidth, metaWidth }: { titleWidth: string; metaWidth: string }) {
  return (
    <header className="flex flex-col gap-1 px-5 py-4">
      <Skeleton className={`h-5 ${titleWidth}`} />
      <Skeleton className={`h-4 ${metaWidth}`} />
    </header>
  );
}
