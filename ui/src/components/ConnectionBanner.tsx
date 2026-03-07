import type { SSEStatus } from "../lib/api";

export default function ConnectionBanner({ status }: Readonly<{ status: SSEStatus }>) {
  const visible = status === "reconnecting";

  return (
    <div
      className={`fixed top-0 left-0 right-0 z-50 flex items-center justify-center bg-amber-600 text-amber-50 text-xs font-medium transition-all duration-300 ${
        visible ? "h-7 opacity-100" : "h-0 opacity-0"
      }`}
      style={{ overflow: "hidden" }}
    >
      Reconnecting&hellip;
    </div>
  );
}
