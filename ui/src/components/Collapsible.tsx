import { useState, type ReactNode } from "react";
import { ChevronRightIcon } from "@heroicons/react/20/solid";

interface CollapsibleProps {
  id: string;
  title?: string;
  header?: ReactNode;
  defaultOpen?: boolean;
  forceOpen?: boolean;
  children: ReactNode;
}

export default function Collapsible({
  id,
  title,
  header,
  defaultOpen = true,
  forceOpen = false,
  children,
}: Readonly<CollapsibleProps>) {
  const key = `collapsible:${id}`;
  const [open, setOpen] = useState(() => {
    const stored = localStorage.getItem(key);
    return stored !== null ? JSON.parse(stored) as boolean : defaultOpen;
  });

  return (
    <div className="rounded-lg border border-slate-700 bg-slate-800 overflow-hidden">
      <div
        role="button"
        tabIndex={0}
        onClick={() => setOpen((o) => {
          const next = !o;
          localStorage.setItem(key, JSON.stringify(next));
          return next;
        })}
        onKeyDown={(e) => {
          if (e.key === "Enter" || e.key === " ") {
            e.preventDefault();
            setOpen((o) => {
              const next = !o;
              localStorage.setItem(key, JSON.stringify(next));
              return next;
            });
          }
        }}
        className="flex w-full items-center gap-2 px-4 py-3 text-sm font-semibold text-slate-300 hover:bg-slate-700/50 transition-colors cursor-pointer select-none"
      >
        <ChevronRightIcon
          className={`h-4 w-4 shrink-0 transition-transform duration-200 ${open || forceOpen ? "rotate-90" : ""}`}
        />
        {header ?? title}
      </div>
      {(open || forceOpen) && <div className="px-4 pb-4 space-y-4">{children}</div>}
    </div>
  );
}
