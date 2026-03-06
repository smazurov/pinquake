import { useEffect, useState, useCallback } from "react";
import { getConfig, getOpenAPISchema } from "../lib/api";
import type { PinQuakeConfig } from "../lib/api";
import type { SSEStatus } from "../lib/api_sse";
import type { FieldMeta } from "../lib/schema";
import { extractAllFieldMeta, extractSectionSchema } from "../lib/schema";
import { useAutoSave } from "../lib/useAutoSave";
import SimpleNavbar from "../components/SimpleNavbar";
import Container from "../components/Container";
import { Card } from "../components/Card";
import BLEControl from "../components/BLEControl";
import OBSControl from "../components/OBSControl";
import type { OBSState } from "../components/OBSControl";
import ConnectionBanner from "../components/ConnectionBanner";
import Collapsible from "../components/Collapsible";
import SchemaForm from "../components/SchemaForm";
import { InputField } from "../components/InputField";

type PreviewTab = "canvas" | "crosshair";

type ConfigSection = "waveform" | "crosshair" | "viz" | "obs";

interface SectionSchema {
  waveform: FieldMeta[];
  crosshair: FieldMeta[];
}

const FALLBACK_WAVEFORM_FIELDS: FieldMeta[] = [
  { key: "buffer_size", type: "number", description: "Ring buffer sample count", min: 32, max: 512, default: 256 },
  { key: "log_knee", type: "slider", description: "Log compression knee", min: 0.001, max: 0.1, step: 0.001, default: 0.02 },
  { key: "force_yellow_g", type: "slider", description: "Yellow threshold (g)", min: 0.01, max: 0.5, step: 0.01, default: 0.03 },
  { key: "force_red_g", type: "slider", description: "Red threshold (g)", min: 0.01, max: 1.0, step: 0.01, default: 0.1 },
  { key: "amp_scale", type: "slider", description: "Amplitude multiplier", min: 0.1, max: 5.0, step: 0.1, default: 1.0 },
  { key: "swap_xy", type: "checkbox", description: "Swap X and Y axes", default: false },
];

const FALLBACK_CROSSHAIR_FIELDS: FieldMeta[] = [
  { key: "force_yellow_g", type: "slider", description: "Yellow threshold (g)", min: 0.01, max: 0.5, step: 0.01, default: 0.03 },
  { key: "force_red_g", type: "slider", description: "Red threshold (g)", min: 0.01, max: 1.0, step: 0.01, default: 0.1 },
  { key: "smoothing", type: "slider", description: "Exponential smoothing factor", min: 0, max: 1, step: 0.05, default: 0.7 },
  { key: "segment_size", type: "slider", description: "Bar segment size (px)", min: 2, max: 30, step: 1, default: 10 },
  { key: "bar_thickness", type: "slider", description: "Bar thickness (px)", min: 4, max: 30, step: 1, default: 12 },
  { key: "swap_xy", type: "checkbox", description: "Swap X and Y axes", default: false },
];

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
  const [obsState, setOBSState] = useState<OBSState>("disconnected");
  const [sectionSchema, setSectionSchema] = useState<SectionSchema>({
    waveform: FALLBACK_WAVEFORM_FIELDS,
    crosshair: FALLBACK_CROSSHAIR_FIELDS,
  });

  const { status: saveStatus, error: saveError } = useAutoSave(config);

  useEffect(() => {
    document.documentElement.classList.add("dark");

    getConfig()
      .then(setConfig)
      .catch((error: unknown) =>
        setLoadError(error instanceof Error ? error.message : String(error)),
      );

    getOpenAPISchema()
      .then((schema) => {
        const waveformSchema = extractSectionSchema(schema, "waveform");
        const crosshairSchema = extractSectionSchema(schema, "crosshair");
        setSectionSchema({
          waveform: waveformSchema
            ? extractAllFieldMeta(waveformSchema)
            : FALLBACK_WAVEFORM_FIELDS,
          crosshair: crosshairSchema
            ? extractAllFieldMeta(crosshairSchema)
            : FALLBACK_CROSSHAIR_FIELDS,
        });
      })
      .catch(() => {});
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
            {(loadError ?? saveError) && (
              <div className="rounded bg-red-900/50 p-3 text-sm text-red-300">
                {loadError ?? saveError}
              </div>
            )}

            <BLEControl
              onSSEStatus={setSSEStatus}
              onOBSStatus={(status) => setOBSState(status as OBSState)}
            />
            <OBSControl
              obsState={obsState}
              config={config}
              onConnectionSave={(host, port, pw) => {
                setConfig((prev) => {
                  if (!prev) return prev;
                  return { ...prev, obs: { ...prev.obs, host, port, password: pw } };
                });
              }}
              onSelectSource={(sceneName, sourceName) => {
                updateSection("obs" as ConfigSection, "scene_name", sceneName);
                updateSection("obs" as ConfigSection, "source_name", sourceName);
              }}
            />
            <div className="flex items-center justify-between">
              <h2 className="text-xs font-medium text-slate-500 uppercase tracking-wider">
                Visualization Settings
              </h2>
              <SaveStatus status={saveStatus} error={saveError} />
            </div>

            <Collapsible id="waveform" title="Waveform">
              <SchemaForm
                fields={sectionSchema.waveform}
                values={config.waveform as unknown as Record<string, unknown>}
                onChange={(key, value) => updateSection("waveform", key, value)}
              />
            </Collapsible>

            <Collapsible id="crosshair" title="Crosshair">
              <SchemaForm
                fields={sectionSchema.crosshair}
                values={config.crosshair as unknown as Record<string, unknown>}
                onChange={(key, value) => updateSection("crosshair", key, value)}
              />
            </Collapsible>

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
