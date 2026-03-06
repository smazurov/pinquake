import type { FieldMeta } from "../lib/schema";
import SchemaField from "./SchemaField";

interface SchemaFormProps {
  fields: FieldMeta[];
  values: Record<string, unknown>;
  onChange: (key: string, value: unknown) => void;
}

export default function SchemaForm({ fields, values, onChange }: Readonly<SchemaFormProps>) {
  return (
    <div className="space-y-4">
      {fields.map((meta) => (
        <SchemaField
          key={meta.key}
          meta={meta}
          value={values[meta.key]}
          onChange={(v) => onChange(meta.key, v)}
        />
      ))}
    </div>
  );
}
