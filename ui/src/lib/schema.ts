export interface SentinelMeta {
  value: number;
  label: string;
}

export interface FieldMeta {
  key: string;
  type: "slider" | "number" | "checkbox" | "select";
  description: string;
  min?: number;
  max?: number;
  step?: number;
  default?: unknown;
  sentinel?: SentinelMeta;
  options?: number[];
}

export interface JSONSchemaProperty {
  type?: string;
  description?: string;
  minimum?: number;
  maximum?: number;
  multipleOf?: number;
  default?: unknown;
  oneOf?: JSONSchemaProperty[];
  const?: number;
  enum?: number[];
}

export interface JSONSchemaObject {
  properties?: Record<string, JSONSchemaProperty>;
}

export function extractFieldMeta(
  schema: JSONSchemaObject,
  key: string,
): FieldMeta | null {
  const prop = schema.properties?.[key];
  if (!prop) return null;

  // Handle oneOf pattern: const sentinel + range
  if (prop.oneOf && prop.oneOf.length >= 2) {
    const constBranch = prop.oneOf.find(
      (b) => b.const !== undefined || (b.enum && b.enum.length === 1),
    );
    const rangeBranch = prop.oneOf.find(
      (b) => b.type === "number" && b.minimum !== undefined && b.maximum !== undefined,
    );
    if (constBranch && rangeBranch) {
      const sentinelValue = constBranch.const ?? constBranch.enum![0]!;
      return {
        key,
        type: "slider",
        description: prop.description ?? key,
        min: rangeBranch.minimum,
        max: rangeBranch.maximum,
        step: rangeBranch.multipleOf,
        default: prop.default,
        sentinel: {
          value: sentinelValue,
          label: constBranch.description ?? String(sentinelValue),
        },
      };
    }
  }

  if (prop.type !== "boolean" && prop.type !== "number" && prop.type !== "integer") {
    return null;
  }

  if ((prop.type === "integer" || prop.type === "number") && prop.enum && prop.enum.length > 1) {
    return {
      key,
      type: "select",
      description: prop.description ?? key,
      default: prop.default,
      options: prop.enum,
    };
  }

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

export function extractNamedSchema(
  openAPISchema: Record<string, unknown>,
  schemaName: string,
): JSONSchemaObject | null {
  const schemas = (
    openAPISchema as {
      components?: { schemas?: Record<string, JSONSchemaObject> };
    }
  ).components?.schemas;
  if (!schemas) return null;
  return schemas[schemaName] ?? null;
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
