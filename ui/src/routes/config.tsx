import { useEffect, useState, useCallback } from "react";
import { api } from "../lib/api";
import type { components } from "../lib/api.generated";

type PinQuakeConfig = components["schemas"]["PinQuakeConfig"];
type WaveformConfig = components["schemas"]["WaveformConfig"];
type CrosshairConfig = components["schemas"]["CrosshairConfig"];
type ExperimentConfig = components["schemas"]["ExperimentConfig"];
type DisplayConfig = components["schemas"]["DisplayConfig"];
type AutoLockConfig = components["schemas"]["AutoLockConfig"];
import type { SSEStatus } from "../lib/api";
import type { FieldMeta } from "../lib/schema";
import { extractAllFieldMeta, extractSectionSchema, extractNamedSchema } from "../lib/schema";
import { getErrorMessage } from "../lib/errors";
import { useAutoSave } from "../lib/useAutoSave";
import SimpleNavbar from "../components/SimpleNavbar";
import Container from "../components/Container";
import { Card } from "../components/Card";
import BLEControl from "../components/BLEControl";
import ConnectionBanner from "../components/ConnectionBanner";
import Collapsible from "../components/Collapsible";
import { ErrorAlert } from "../components/ErrorAlert";
import SchemaForm from "../components/SchemaForm";
import { InputField } from "../components/InputField";
import experimentRegistry from "../lib/experimentRegistry";

type PreviewTab = "off" | "canvas" | "crosshair" | "experiment";


const TAB_LABELS: Record<PreviewTab, string> = {
  off: "Off",
  canvas: "Canvas",
  crosshair: "Crosshair",
  experiment: "Experiment",
};

type WT901Config = components["schemas"]["WT901Config"];

interface SectionSchema {
  waveform: FieldMeta[];
  crosshair: FieldMeta[];
  experiment: FieldMeta[];
  display: FieldMeta[];
  autoLock: FieldMeta[];
}

const HIDDEN_FIELDS = new Set(["enabled", "width", "height"]);

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

function combineSaveStatus(
  ...saves: { status: string; error: string | null }[]
): { status: string; error: string | null } {
  for (const s of saves) {
    if (s.status === "error") return s;
  }
  for (const s of saves) {
    if (s.status === "saving") return s;
  }
  for (const s of saves) {
    if (s.status === "saved") return s;
  }
  return { status: "idle", error: null };
}

const saveWaveform = async (val: WaveformConfig) => {
  const { error } = await api.PUT("/api/config/waveform", { body: val });
  return { error };
};

const saveCrosshair = async (val: CrosshairConfig) => {
  const { error } = await api.PUT("/api/config/crosshair", { body: val });
  return { error };
};

const saveExperiment = async (val: ExperimentConfig) => {
  const { error } = await api.PUT("/api/config/experiment", { body: val });
  return { error };
};

const saveDisplay = async (val: DisplayConfig) => {
  const { error } = await api.PUT("/api/config/display", { body: val });
  return { error };
};

const saveAutoLock = async (val: AutoLockConfig) => {
  const { error } = await api.PUT("/api/config/auto_lock", { body: val });
  return { error };
};

const saveSensorConfig = async (val: WT901Config) => {
  const { error } = await api.PUT("/api/config/sensor/WT901", { body: val });
  return { error };
};

function PreviewContent({ url, label }: Readonly<{ url: string | null; label: string }>) {
  if (!url) {
    return (
      <div className="bg-black/50 flex items-center justify-center p-4 text-slate-500 text-sm">
        Preview off
      </div>
    );
  }
  return (
    <div className="bg-black/50 flex items-center justify-center p-4">
      <div style={{ width: "100%", aspectRatio: "9 / 16" }}>
        <iframe
          src={url}
          className="w-full h-full border-0 rounded"
          style={{ background: "transparent" }}
          title={`${label} Preview`}
        />
      </div>
    </div>
  );
}

export default function ConfigRoute() {
  const [config, setConfig] = useState<PinQuakeConfig | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [previewTab, setPreviewTab] = useState<PreviewTab>("crosshair");
  const [activeExperiments, setActiveExperiments] = useState<Set<string>>(
    () => new Set(experimentRegistry.map((e) => e.id)),
  );
  const [sseStatus, setSSEStatus] = useState<SSEStatus>("connecting");
  const [sectionSchema, setSectionSchema] = useState<SectionSchema | null>(null);
  const [schemaError, setSchemaError] = useState<string | null>(null);
  const [connectedSensor, setConnectedSensor] = useState<string | null>(null);
  const [sensorConfig, setSensorConfig] = useState<WT901Config | null>(null);
  const [sensorFields, setSensorFields] = useState<FieldMeta[] | null>(null);

  const waveformSave = useAutoSave(config?.waveform ?? null, saveWaveform);
  const crosshairSave = useAutoSave(config?.crosshair ?? null, saveCrosshair);
  const experimentSave = useAutoSave(config?.experiment ?? null, saveExperiment);
  const displaySave = useAutoSave(config?.display ?? null, saveDisplay);
  const autoLockSave = useAutoSave(config?.auto_lock ?? null, saveAutoLock);
  const sensorSave = useAutoSave(sensorConfig, saveSensorConfig);
  const { status: saveStatus, error: saveError } = combineSaveStatus(waveformSave, crosshairSave, experimentSave, displaySave, autoLockSave, sensorSave);

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
        const experimentSchema = extractSectionSchema(schema, "experiment");
        const displaySchema = extractSectionSchema(schema, "display");
        const autoLockSchema = extractSectionSchema(schema, "auto_lock");
        if (!waveformSchema || !crosshairSchema || !experimentSchema || !displaySchema) {
          setSchemaError("Schema missing waveform, crosshair, experiment, or display section");
          return;
        }
        setSectionSchema({
          waveform: extractAllFieldMeta(waveformSchema).filter((f) => !HIDDEN_FIELDS.has(f.key)),
          crosshair: extractAllFieldMeta(crosshairSchema).filter((f) => !HIDDEN_FIELDS.has(f.key)),
          experiment: extractAllFieldMeta(experimentSchema).filter((f) => !HIDDEN_FIELDS.has(f.key)),
          display: extractAllFieldMeta(displaySchema, ["swap_xy"]),
          autoLock: autoLockSchema ? extractAllFieldMeta(autoLockSchema) : [],
        });

        const sensorSchema = extractNamedSchema(schema, "WT901Config");
        if (sensorSchema) {
          setSensorFields(
            extractAllFieldMeta(sensorSchema).filter((f) => f.key !== "$schema"),
          );
        }
      })
      .catch((error: unknown) => {
        console.error("Failed to fetch OpenAPI schema:", error);
        setSchemaError(getErrorMessage(error));
      });

  }, []);

  const handleSensorChange = useCallback((sensorName: string | null) => {
    setConnectedSensor(sensorName);
    if (!sensorName) {
      setSensorConfig(null);
      return;
    }
    if (sensorName === "WT901") {
      api.GET("/api/config/sensor/WT901").then(({ data }) => {
        if (data) setSensorConfig(data);
      });
    }
  }, []);

  const updateWaveform = useCallback(
    (key: string, value: unknown) => {
      setConfig((prev) => {
        if (!prev) return prev;
        return { ...prev, waveform: { ...prev.waveform, [key]: value } };
      });
    },
    [],
  );

  const updateCrosshair = useCallback(
    (key: string, value: unknown) => {
      setConfig((prev) => {
        if (!prev) return prev;
        return { ...prev, crosshair: { ...prev.crosshair, [key]: value } };
      });
    },
    [],
  );

  const updateExperiment = useCallback(
    (key: string, value: unknown) => {
      setConfig((prev) => {
        if (!prev) return prev;
        return { ...prev, experiment: { ...prev.experiment, [key]: value } };
      });
    },
    [],
  );

  const updateDisplay = useCallback(
    (key: string, value: unknown) => {
      setConfig((prev) => {
        if (!prev) return prev;
        return { ...prev, display: { ...prev.display, [key]: value } };
      });
    },
    [],
  );

  const updateAutoLock = useCallback(
    (key: string, value: unknown) => {
      setConfig((prev) => {
        if (!prev) return prev;
        return { ...prev, auto_lock: { ...prev.auto_lock, [key]: value } };
      });
    },
    [],
  );

  const updateSensor = useCallback(
    (key: string, value: unknown) => {
      setSensorConfig((prev) => {
        if (!prev) return prev;
        return { ...prev, [key]: value };
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

  const enabledVizzes = [
    "off" as PreviewTab,
    config.waveform.enabled && "canvas",
    config.crosshair.enabled && "crosshair",
    config.experiment.enabled && "experiment",
  ].filter(Boolean) as PreviewTab[];

  const activeTab = enabledVizzes.includes(previewTab) ? previewTab : enabledVizzes[0];

  const canvasUrl = `${window.location.origin}/canvas?width=${config.waveform.width}&height=${config.waveform.height}`;
  const crosshairUrl = `${window.location.origin}/crosshair?width=${config.crosshair.width}&height=${config.crosshair.height}`;
  const showParam = activeExperiments.size === experimentRegistry.length
    ? ""
    : `&show=${[...activeExperiments].join(",")}`;
  const experimentUrl = `${window.location.origin}/experiment?width=${config.experiment.width}&height=${config.experiment.height}${showParam}`;
  const previewUrls: Record<string, string> = { canvas: canvasUrl, crosshair: crosshairUrl, experiment: experimentUrl };
  const previewUrl = activeTab !== "off" ? previewUrls[activeTab] : null;

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
              onSensorChange={handleSensorChange}
            />

            {connectedSensor && sensorConfig && sensorFields && (
              <Collapsible id="sensor" title="Sensor" defaultOpen={false}>
                <SchemaForm
                  fields={sensorFields}
                  values={sensorConfig as unknown as Record<string, unknown>}
                  onChange={updateSensor}
                />
                {sectionSchema?.autoLock && sectionSchema.autoLock.length > 0 && (
                  <>
                    <div className="border-t border-slate-700 mt-4 pt-4">
                      <p className="text-xs font-medium text-slate-400 mb-3">Auto Lock</p>
                      <SchemaForm
                        fields={sectionSchema.autoLock}
                        values={config.auto_lock as unknown as Record<string, unknown>}
                        onChange={updateAutoLock}
                      />
                    </div>
                  </>
                )}
              </Collapsible>
            )}

            <div className="flex items-center justify-between">
              <h2 className="text-xs font-medium text-slate-500 uppercase tracking-wider">
                Visualization Settings
              </h2>
              <SaveStatus status={saveStatus} error={saveError} />
            </div>

            {config.waveform.enabled && (
              <Collapsible id="waveform" title="Waveform">
                <div className="grid grid-cols-2 gap-4 mb-4">
                  <InputField
                    label="Width"
                    type="number"
                    value={config.waveform.width}
                    onChange={(e) => updateWaveform("width", Number(e.target.value))}
                  />
                  <InputField
                    label="Height"
                    type="number"
                    value={config.waveform.height}
                    onChange={(e) => updateWaveform("height", Number(e.target.value))}
                  />
                </div>
                {sectionSchema?.waveform ? (
                  <SchemaForm
                    fields={sectionSchema.waveform}
                    values={config.waveform as unknown as Record<string, unknown>}
                    onChange={updateWaveform}
                  />
                ) : (
                  <p className="text-xs text-slate-500">Loading schema...</p>
                )}
              </Collapsible>
            )}

            {config.crosshair.enabled && (
              <Collapsible id="crosshair" title="Crosshair">
                <div className="grid grid-cols-2 gap-4 mb-4">
                  <InputField
                    label="Width"
                    type="number"
                    value={config.crosshair.width}
                    onChange={(e) => updateCrosshair("width", Number(e.target.value))}
                  />
                  <InputField
                    label="Height"
                    type="number"
                    value={config.crosshair.height}
                    onChange={(e) => updateCrosshair("height", Number(e.target.value))}
                  />
                </div>
                {sectionSchema?.crosshair ? (
                  <SchemaForm
                    fields={sectionSchema.crosshair}
                    values={config.crosshair as unknown as Record<string, unknown>}
                    onChange={updateCrosshair}
                  />
                ) : (
                  <p className="text-xs text-slate-500">Loading schema...</p>
                )}
              </Collapsible>
            )}

            {config.experiment.enabled && (
              <Collapsible id="experiment" title="Experiment">
                <div className="mb-4">
                  <p className="text-xs font-medium text-slate-400 mb-2">Active experiments</p>
                  <div className="flex flex-wrap gap-3">
                    {experimentRegistry.map((entry) => (
                      <label key={entry.id} className="flex items-center gap-1.5 text-sm text-slate-300 cursor-pointer">
                        <input
                          type="checkbox"
                          checked={activeExperiments.has(entry.id)}
                          onChange={() => {
                            setActiveExperiments((prev) => {
                              const next = new Set(prev);
                              if (next.has(entry.id)) next.delete(entry.id);
                              else next.add(entry.id);
                              return next;
                            });
                          }}
                        />
                        {entry.label}
                      </label>
                    ))}
                  </div>
                </div>
                <div className="grid grid-cols-2 gap-4 mb-4">
                  <InputField
                    label="Width"
                    type="number"
                    value={config.experiment.width}
                    onChange={(e) => updateExperiment("width", Number(e.target.value))}
                  />
                  <InputField
                    label="Height"
                    type="number"
                    value={config.experiment.height}
                    onChange={(e) => updateExperiment("height", Number(e.target.value))}
                  />
                </div>
                {sectionSchema?.experiment ? (
                  <SchemaForm
                    fields={sectionSchema.experiment}
                    values={config.experiment as unknown as Record<string, unknown>}
                    onChange={updateExperiment}
                  />
                ) : (
                  <p className="text-xs text-slate-500">Loading schema...</p>
                )}
              </Collapsible>
            )}

            <Collapsible id="display" title="Display">
              {sectionSchema?.display ? (
                <SchemaForm
                  fields={sectionSchema.display}
                  values={config.display as unknown as Record<string, unknown>}
                  onChange={updateDisplay}
                />
              ) : (
                <p className="text-xs text-slate-500">Loading schema...</p>
              )}
            </Collapsible>

          </div>

          <div className="flex-1 min-w-0">
            <div className="sticky top-4">
              <Card padding="none" className="overflow-hidden">
                <div className="px-4 py-3 border-b border-slate-200 dark:border-slate-700 flex items-center gap-2">
                  <div className="flex rounded overflow-hidden border border-slate-600 shrink-0">
                    {enabledVizzes.map((tab) => (
                      <button
                        key={tab}
                        onClick={() => setPreviewTab(tab)}
                        className={`px-2.5 py-1 text-xs font-medium transition-colors ${
                          activeTab === tab
                            ? "bg-blue-600 text-white"
                            : "bg-slate-800 text-slate-400 hover:text-slate-200"
                        }`}
                      >
                        {TAB_LABELS[tab]}
                      </button>
                    ))}
                  </div>
                  {previewUrl && (
                    <input
                      type="text"
                      readOnly
                      value={previewUrl}
                      onFocus={(e) => e.target.select()}
                      className="flex-1 min-w-0 rounded bg-slate-800 border border-slate-600 px-2 py-1 text-xs text-slate-300 font-mono select-all"
                    />
                  )}
                </div>
                <PreviewContent url={previewUrl} label={TAB_LABELS[activeTab]} />
              </Card>
            </div>
          </div>
        </div>
      </Container>
    </div>
  );
}
