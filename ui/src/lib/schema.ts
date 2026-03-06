export interface FieldMeta {
  key: string;
  type: "slider" | "number" | "checkbox";
  description: string;
  min?: number;
  max?: number;
  step?: number;
  default?: unknown;
}

interface JSONSchemaProperty {
  type?: string;
  description?: string;
  minimum?: number;
  maximum?: number;
  multipleOf?: number;
  default?: unknown;
}

interface JSONSchemaObject {
  properties?: Record<string, JSONSchemaProperty>;
}

export function extractFieldMeta(
  schema: JSONSchemaObject,
  key: string,
): FieldMeta | null {
  const prop = schema.properties?.[key];
  if (!prop) return null;

  if (prop.type === "boolean") {
    return {
      key,
      type: "checkbox",
      description: prop.description ?? key,
      default: prop.default,
    };
  }

  const hasRange =
    prop.minimum !== undefined &&
    prop.maximum !== undefined;

  return {
    key,
    type: hasRange ? "slider" : "number",
    description: prop.description ?? key,
    min: prop.minimum,
    max: prop.maximum,
    step: prop.multipleOf,
    default: prop.default,
  };
}

export function extractAllFieldMeta(schema: JSONSchemaObject): FieldMeta[] {
  if (!schema.properties) return [];
  return Object.keys(schema.properties)
    .map((key) => extractFieldMeta(schema, key))
    .filter((m): m is FieldMeta => m !== null);
}

export function extractSectionSchema(
  openAPISchema: Record<string, unknown>,
  sectionPath: string,
): JSONSchemaObject | null {
  // Navigate: components.schemas.PinQuakeConfig.properties.<section>
  const schemas = (
    openAPISchema as {
      components?: { schemas?: Record<string, JSONSchemaObject> };
    }
  ).components?.schemas;
  if (!schemas) return null;

  // Find the config schema — try known names
  const configSchema =
    schemas["PinQuakeConfig"] ?? schemas["ConfigRequestBody"];
  if (!configSchema?.properties) return null;

  const section = configSchema.properties[sectionPath] as
    | JSONSchemaObject
    | undefined;
  if (!section?.properties) {
    // Try $ref resolution — look for the section type directly
    const sectionTypeName =
      sectionPath.charAt(0).toUpperCase() +
      sectionPath.slice(1) +
      "Config";
    return schemas[sectionTypeName] ?? null;
  }
  return section;
}
