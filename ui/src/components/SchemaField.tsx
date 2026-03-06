import type { FieldMeta } from "../lib/schema";

function formatValue(value: number, step?: number): string {
  if (step === undefined || step === 0) return String(value);
  const decimals = Math.max(0, -Math.floor(Math.log10(step)));
  return value.toFixed(decimals);
}

interface SchemaFieldProps {
  meta: FieldMeta;
  value: unknown;
  onChange: (value: unknown) => void;
}

export default function SchemaField({ meta, value, onChange }: Readonly<SchemaFieldProps>) {
  if (meta.type === "checkbox") {
    return (
      <label className="flex items-center gap-2 cursor-pointer">
        <input
          type="checkbox"
          checked={Boolean(value)}
          onChange={(e) => onChange(e.target.checked)}
          className="rounded border-slate-300/20 bg-slate-800 text-blue-600 focus:ring-blue-500"
        />
        <span className="text-sm font-medium text-gray-300">{meta.description}</span>
      </label>
    );
  }

  if (meta.type === "slider") {
    const numValue = Number(value ?? meta.default ?? meta.min ?? 0);
    return (
      <div className="space-y-1">
        <div className="flex items-center justify-between">
          <label className="text-sm font-medium text-gray-300">{meta.description}</label>
          <span className="text-xs font-mono text-slate-400 tabular-nums">{formatValue(numValue, meta.step)}</span>
        </div>
        <input
          type="range"
          min={meta.min}
          max={meta.max}
          step={meta.step}
          value={numValue}
          onChange={(e) => onChange(Number(e.target.value))}
          className="w-full"
        />
      </div>
    );
  }

  // number input
  const numValue = Number(value ?? meta.default ?? 0);
  return (
    <div className="space-y-1">
      <label className="block text-sm font-medium text-gray-300">{meta.description}</label>
      <input
        type="number"
        min={meta.min}
        max={meta.max}
        step={meta.step}
        value={numValue}
        onChange={(e) => onChange(Number(e.target.value))}
        className="block w-full rounded-sm border border-slate-300/20 bg-slate-800 px-3 py-2 text-sm text-white focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
      />
    </div>
  );
}
