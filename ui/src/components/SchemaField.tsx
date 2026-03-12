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

  if (meta.type === "select" && meta.options) {
    const numValue = Number(value ?? meta.default ?? meta.options[0]);
    return (
      <div className="space-y-1">
        <label className="block text-sm font-medium text-gray-300">{meta.description}</label>
        <select
          value={numValue}
          onChange={(e) => onChange(Number(e.target.value))}
          className="block w-full rounded-sm border border-slate-300/20 bg-slate-800 px-3 py-2 text-sm text-white focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
        >
          {meta.options.map((opt) => (
            <option key={opt} value={opt}>
              {opt}
            </option>
          ))}
        </select>
      </div>
    );
  }

  if (meta.type === "slider" && meta.sentinel && meta.min !== undefined && meta.max !== undefined) {
    const numValue = Number(value ?? meta.default ?? meta.min);
    const isSentinel = numValue === meta.sentinel.value;
    const step = meta.step ?? 1;
    const rangeSteps = Math.round((meta.max - meta.min) / step);
    // Position 0 = sentinel, positions 1..rangeSteps+1 = min..max
    const sliderMax = rangeSteps + 1;
    const sliderPos = isSentinel ? 0 : Math.round((numValue - meta.min) / step) + 1;
    const displayLabel = isSentinel ? meta.sentinel.label : formatValue(numValue, step);

    return (
      <div className="space-y-1">
        <div className="flex items-center justify-between">
          <label className="text-sm font-medium text-gray-300">{meta.description}</label>
          <span className="text-xs font-mono text-slate-400 tabular-nums">{displayLabel}</span>
        </div>
        <input
          type="range"
          min={0}
          max={sliderMax}
          step={1}
          value={sliderPos}
          onChange={(e) => {
            const pos = Number(e.target.value);
            onChange(pos === 0 ? meta.sentinel!.value : meta.min! + (pos - 1) * step);
          }}
          className="w-full"
        />
      </div>
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
