import { describe, it, expect } from "vitest";
import { extractFieldMeta, extractAllFieldMeta, extractSectionSchema, type JSONSchemaObject } from "./schema";

describe("extractFieldMeta", () => {
  const schema = {
    properties: {
      decay_s: {
        type: "number",
        description: "Decay time (s)",
        minimum: 0,
        maximum: 2,
        multipleOf: 0.01,
        default: 0.3,
      },
      buffer_size: {
        type: "integer",
        description: "Ring buffer sample count",
        minimum: 32,
        maximum: 512,
        default: 256,
      },
      swap_xy: {
        type: "boolean",
        description: "Swap X and Y axes",
        default: false,
      },
      raw_value: {
        type: "number",
        description: "A plain number field",
        default: 42,
      },
    },
  };

  it("extracts slider field when min+max+step present", () => {
    const meta = extractFieldMeta(schema, "decay_s");
    expect(meta).toEqual({
      key: "decay_s",
      type: "slider",
      description: "Decay time (s)",
      min: 0,
      max: 2,
      step: 0.01,
      default: 0.3,
    });
  });

  it("extracts slider field when min+max present (no step)", () => {
    const meta = extractFieldMeta(schema, "buffer_size");
    expect(meta).toEqual({
      key: "buffer_size",
      type: "slider",
      description: "Ring buffer sample count",
      min: 32,
      max: 512,
      step: undefined,
      default: 256,
    });
  });

  it("extracts checkbox for boolean", () => {
    const meta = extractFieldMeta(schema, "swap_xy");
    expect(meta).toEqual({
      key: "swap_xy",
      type: "checkbox",
      description: "Swap X and Y axes",
      default: false,
    });
  });

  it("extracts number field when no min/max", () => {
    const meta = extractFieldMeta(schema, "raw_value");
    expect(meta).toEqual({
      key: "raw_value",
      type: "number",
      description: "A plain number field",
      min: undefined,
      max: undefined,
      step: undefined,
      default: 42,
    });
  });

  it("extracts select field for integer with enum", () => {
    const enumSchema: JSONSchemaObject = {
      properties: {
        output_rate_hz: {
          type: "integer",
          description: "Output rate (Hz)",
          enum: [10, 20, 50, 100, 200],
          default: 50,
        },
      },
    };
    const meta = extractFieldMeta(enumSchema, "output_rate_hz");
    expect(meta).toEqual({
      key: "output_rate_hz",
      type: "select",
      description: "Output rate (Hz)",
      default: 50,
      options: [10, 20, 50, 100, 200],
    });
  });

  it("returns null for unknown key", () => {
    expect(extractFieldMeta(schema, "nope")).toBeNull();
  });

  const alwaysVisible = "Always visible";

  it("extracts oneOf with const sentinel and range", () => {
    const oneOfSchema: JSONSchemaObject = {
      properties: {
        fade_s: {
          description: "Visible duration after trigger (s)",
          default: 5,
          oneOf: [
            { const: -1, description: alwaysVisible },
            { type: "number", minimum: 1, maximum: 30, multipleOf: 0.1 },
          ],
        },
      },
    };
    const meta = extractFieldMeta(oneOfSchema, "fade_s");
    expect(meta).toEqual({
      key: "fade_s",
      type: "slider",
      description: "Visible duration after trigger (s)",
      min: 1,
      max: 30,
      step: 0.1,
      default: 5,
      sentinel: { value: -1, label: alwaysVisible },
    });
  });

  it("extracts oneOf with enum sentinel and range", () => {
    const oneOfSchema: JSONSchemaObject = {
      properties: {
        fade_s: {
          description: "Fade duration",
          default: 5,
          oneOf: [
            { type: "number", description: alwaysVisible, enum: [-1] },
            { type: "number", minimum: 1, maximum: 30, multipleOf: 0.1 },
          ],
        },
      },
    };
    const meta = extractFieldMeta(oneOfSchema, "fade_s");
    expect(meta).toEqual({
      key: "fade_s",
      type: "slider",
      description: "Fade duration",
      min: 1,
      max: 30,
      step: 0.1,
      default: 5,
      sentinel: { value: -1, label: alwaysVisible },
    });
  });
});

describe("extractAllFieldMeta", () => {
  it("returns all fields in order", () => {
    const schema = {
      properties: {
        a: { type: "number", description: "A", minimum: 0, maximum: 1, multipleOf: 0.1 },
        b: { type: "boolean", description: "B" },
      },
    };
    const metas = extractAllFieldMeta(schema);
    expect(metas).toHaveLength(2);
    expect(metas[0]!.key).toBe("a");
    expect(metas[1]!.key).toBe("b");
  });

  it("returns empty array for missing properties", () => {
    expect(extractAllFieldMeta({})).toEqual([]);
  });
});

describe("extractSectionSchema", () => {
  const openAPI = {
    components: {
      schemas: {
        PinQuakeConfig: {
          properties: {
            waveform: {
              properties: {
                buffer_size: { type: "integer", minimum: 32, maximum: 512 },
              },
            },
          },
        },
        CrosshairConfig: {
          properties: {
            decay_s: { type: "number", minimum: 0, maximum: 2, multipleOf: 0.01 },
          },
        },
      },
    },
  };

  it("extracts inline section schema", () => {
    const section = extractSectionSchema(openAPI, "waveform");
    expect(section?.properties).toHaveProperty("buffer_size");
  });

  it("falls back to type name lookup", () => {
    const section = extractSectionSchema(openAPI, "crosshair");
    expect(section?.properties).toHaveProperty("decay_s");
  });

  it("returns null for unknown section", () => {
    expect(extractSectionSchema(openAPI, "unknown")).toBeNull();
  });
});
