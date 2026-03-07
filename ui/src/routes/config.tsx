import { useEffect, useState, useCallback } from "react";
import { api } from "../lib/api";
import type { components } from "../lib/api.generated";

type PinQuakeConfig = components["schemas"]["PinQuakeConfig"];
import type { SSEStatus } from "../lib/api";
import type { FieldMeta } from "../lib/schema";
import { extractAllFieldMeta, extractSectionSchema } from "../lib/schema";
import { getErrorMessage } from "../lib/errors";
import { useAutoSave } from "../lib/useAutoSave";
import SimpleNavbar from "../components/SimpleNavbar";
import Container from "../components/Container";
import { Card } from "../components/Card";
import BLEControl from "../components/BLEControl";
import ConnectionBanner from "../components/ConnectionBanner";
import Collapsible from "../components/Collapsible";
import { ErrorAlert } from "../components/ErrorAlert";
import { SchemaSection } from "../components/SchemaSection";
import { InputField } from "../components/InputField";

type PreviewTab = "canvas" | "crosshair";

type ConfigSection = "waveform" | "crosshair" | "viz";

interface SectionSchema {
  waveform: FieldMeta[];
  crosshair: FieldMeta[];
}

function SaveStatus({ status, error }: Readonly<{ status: string; error: string | null }>) {
  if (status === "saving") {
    return <span className="text-xs text-slate-400 animate-pulse">Saving...</span>;
  }
  if (status === "saved") {
    return <span className="text-xs text-green-400">Saved</span>;
  }
  if (status === "error") {
    return <span className="text-xs text-red-400">{error ?? "Save failed"}</span>;
  }
  return null;
}

export default function ConfigRoute() {
  const [config, setConfig] = useState<PinQuakeConfig | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [previewTab, setPreviewTab] = useState<PreviewTab>("canvas");
  const [sseStatus, setSSEStatus] = useState<SSEStatus>("connecting");
  const [sectionSchema, setSectionSchema] = useState<SectionSchema | null>(null);
  const [schemaError, setSchemaError] = useState<string | null>(null);

  const { status: saveStatus, error: saveError } = useAutoSave(config);

  useEffect(() => {
    document.documentElement.classList.add("dark");

    api.GET("/api/config").then(({ data, error }) => {
      if (error) { setLoadError(error.detail ?? "Failed to load config"); return; }
      setConfig(data);
    });

    fetch("/openapi.json")
      .then((r) => r.json() as Promise<Record<string, unknown>>)
      .then((schema) => {
        const waveformSchema = extractSectionSchema(schema, "waveform");
        const crosshairSchema = extractSectionSchema(schema, "crosshair");
        if (!waveformSchema || !crosshairSchema) {
          setSchemaError("Schema missing waveform or crosshair section");
          return;
        }
        setSectionSchema({
          waveform: extractAllFieldMeta(waveformSchema),
          crosshair: extractAllFieldMeta(crosshairSchema),
        });
      })
      .catch((error: unknown) => {
        console.error("Failed to fetch OpenAPI schema:", error);
        setSchemaError(getErrorMessage(error));
      });
  }, []);

  const updateSection = useCallback(
    (section: ConfigSection, key: string, value: unknown) => {
      setConfig((prev) => {
        if (!prev) return prev;
        return {
          ...prev,
          [section]: { ...prev[section], [key]: value },
        };
      });
    },
    [],
  );

  if (!config) {
    return (
      <div className="flex h-screen items-center justify-center bg-slate-900 text-white">
        {loadError ? (
          <p className="text-red-400">{loadError}</p>
        ) : (
          <p className="text-slate-400">Loading config...</p>
        )}
      </div>
    );
  }

  const canvasUrl = `${window.location.origin}/canvas?width=${config.viz.width}&height=${config.viz.height}`;
  const crosshairUrl = `${window.location.origin}/crosshair?width=${config.viz.width}&height=${config.viz.height}`;
  const previewUrl = previewTab === "canvas" ? canvasUrl : crosshairUrl;

  return (
    <div className="min-h-screen bg-slate-900 overflow-auto">
      <ConnectionBanner status={sseStatus} />
      <SimpleNavbar logoText="PinQuake" />

      <Container>
        <div className="flex gap-8 pb-8">
          <div className="w-full max-w-md space-y-4 shrink-0">
            {(loadError ?? saveError ?? schemaError) && (
              <ErrorAlert message={(loadError ?? saveError ?? schemaError)!} />
            )}

            <BLEControl
              onSSEStatus={setSSEStatus}
            />
            <div className="flex items-center justify-between">
              <h2 className="text-xs font-medium text-slate-500 uppercase tracking-wider">
                Visualization Settings
              </h2>
              <SaveStatus status={saveStatus} error={saveError} />
            </div>

            <SchemaSection
              id="waveform"
              title="Waveform"
              fields={sectionSchema?.waveform}
              values={config.waveform as unknown as Record<string, unknown>}
              onChange={(key, value) => updateSection("waveform", key, value)}
            />

            <SchemaSection
              id="crosshair"
              title="Crosshair"
              fields={sectionSchema?.crosshair}
              values={config.crosshair as unknown as Record<string, unknown>}
              onChange={(key, value) => updateSection("crosshair", key, value)}
            />

            <Collapsible id="dimensions" title="Dimensions">
              <div className="grid grid-cols-2 gap-4">
                <InputField
                  label="Width"
                  type="number"
                  value={config.viz.width}
                  onChange={(e) => updateSection("viz", "width", Number(e.target.value))}
                />
                <InputField
                  label="Height"
                  type="number"
                  value={config.viz.height}
                  onChange={(e) => updateSection("viz", "height", Number(e.target.value))}
                />
              </div>
            </Collapsible>
          </div>

          <div className="flex-1 min-w-0">
            <div className="sticky top-4">
              <Card padding="none" className="overflow-hidden">
                <div className="px-4 py-3 border-b border-slate-200 dark:border-slate-700 flex items-center gap-2">
                  <div className="flex rounded overflow-hidden border border-slate-600 shrink-0">
                    {(["canvas", "crosshair"] as const).map((tab) => (
                      <button
                        key={tab}
                        onClick={() => setPreviewTab(tab)}
                        className={`px-2.5 py-1 text-xs font-medium transition-colors ${
                          previewTab === tab
                            ? "bg-blue-600 text-white"
                            : "bg-slate-800 text-slate-400 hover:text-slate-200"
                        }`}
                      >
                        {tab === "canvas" ? "Canvas" : "Crosshair"}
                      </button>
                    ))}
                  </div>
                  <input
                    type="text"
                    readOnly
                    value={previewUrl}
                    onFocus={(e) => e.target.select()}
                    className="flex-1 min-w-0 rounded bg-slate-800 border border-slate-600 px-2 py-1 text-xs text-slate-300 font-mono select-all"
                  />
                </div>
                <div className="bg-black/50 flex items-center justify-center p-4">
                  <div
                    style={{
                      width: "100%",
                      aspectRatio: "9 / 16",
                    }}
                  >
                    <iframe
                      src={previewUrl}
                      className="w-full h-full border-0 rounded"
                      style={{ background: "transparent" }}
                      title={`${previewTab === "canvas" ? "Canvas" : "Crosshair"} Preview`}
                    />
                  </div>
                </div>
              </Card>
            </div>
          </div>
        </div>
      </Container>
    </div>
  );
}
