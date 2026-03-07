import type { FieldMeta } from "../lib/schema";
import Collapsible from "./Collapsible";
import SchemaForm from "./SchemaForm";

export function SchemaSection({
  id,
  title,
  fields,
  values,
  onChange,
}: Readonly<{
  id: string;
  title: string;
  fields: FieldMeta[] | undefined;
  values: Record<string, unknown>;
  onChange: (key: string, value: unknown) => void;
}>) {
  return (
    <Collapsible id={id} title={title}>
      {fields ? (
        <SchemaForm fields={fields} values={values} onChange={onChange} />
      ) : (
        <p className="text-xs text-slate-500">Loading schema...</p>
      )}
    </Collapsible>
  );
}
