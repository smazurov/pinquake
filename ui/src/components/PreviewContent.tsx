import { useEffect, useRef, useState } from "react";

interface PreviewContentProps {
  url: string | null;
  label: string;
  dimensions: { width: number; height: number } | null;
}

export default function PreviewContent({ url, label, dimensions }: Readonly<PreviewContentProps>) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [availableSize, setAvailableSize] = useState({ width: 0, height: 0 });

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    const observer = new ResizeObserver(([entry]) => {
      setAvailableSize({ width: entry.contentRect.width, height: entry.contentRect.height });
    });
    observer.observe(container);
    return () => observer.disconnect();
  }, []);

  const scale = dimensions && dimensions.width > 0 && dimensions.height > 0
    ? Math.min(1, availableSize.width / dimensions.width, availableSize.height / dimensions.height)
    : 0;

  return (
    <div className="bg-black/50 p-4">
      <div
        ref={containerRef}
        className="relative w-full overflow-hidden"
        style={{ height: "calc(100dvh - 12rem)" }}
      >
        {url && dimensions ? (
          <iframe
            src={url}
            width={dimensions.width}
            height={dimensions.height}
            className="absolute left-1/2 top-1/2 border-0 rounded"
            style={{ background: "transparent", transform: `translate(-50%, -50%) scale(${scale})` }}
            title={`${label} Preview`}
          />
        ) : (
          <div className="flex h-full items-center justify-center text-slate-500 text-sm">
            Preview off
          </div>
        )}
      </div>
    </div>
  );
}
