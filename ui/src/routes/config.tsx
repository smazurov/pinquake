import { useEffect, useState } from "react";
import { getConfig, updateConfig } from "../lib/api";
import type { PinQuakeConfig } from "../lib/api";
import SimpleNavbar from "../components/SimpleNavbar";
import Container from "../components/Container";
import { Card, CardHeader, CardContent } from "../components/Card";
import { InputField } from "../components/InputField";
import { Button } from "../components/Button";
import BLEControl from "../components/BLEControl";

export default function ConfigRoute() {
  const [config, setConfig] = useState<PinQuakeConfig | null>(null);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);

  useEffect(() => {
    document.documentElement.classList.add("dark");
    getConfig()
      .then(setConfig)
      .catch((err: unknown) =>
        setError(err instanceof Error ? err.message : String(err)),
      );
  }, []);

  const handleSave = async () => {
    if (!config) return;
    setSaving(true);
    setError(null);
    setSaved(false);
    try {
      const updated = await updateConfig(config);
      setConfig(updated);
      setSaved(true);
      setTimeout(() => setSaved(false), 2000);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSaving(false);
    }
  };

  if (!config) {
    return (
      <div className="flex h-screen items-center justify-center bg-slate-900 text-white">
        {error ? (
          <p className="text-red-400">{error}</p>
        ) : (
          <p className="text-slate-400">Loading config...</p>
        )}
      </div>
    );
  }

  const canvasUrl = `${window.location.origin}/canvas?width=${config.viz.width}&height=${config.viz.height}`;

  return (
    <div className="min-h-screen bg-slate-900 overflow-auto">
      <SimpleNavbar logoText="PinQuake" />

      <Container>
        <div className="flex gap-8 pb-8">
          <div className="w-full max-w-md space-y-6 shrink-0">
            {error && (
              <div className="rounded bg-red-900/50 p-3 text-sm text-red-300">{error}</div>
            )}

            <BLEControl />

            <Card padding="lg">
              <CardHeader>
                <h2 className="text-sm font-semibold text-slate-300">Waveform</h2>
              </CardHeader>
              <CardContent className="space-y-4">
                <InputField
                  label="Buffer Size"
                  type="number"
                  value={config.waveform.buffer_size}
                  onChange={(e) =>
                    setConfig({
                      ...config,
                      waveform: { ...config.waveform, buffer_size: Number(e.target.value) },
                    })
                  }
                />
                <InputField
                  label="Log Knee"
                  type="number"
                  step={0.005}
                  value={config.waveform.log_knee}
                  onChange={(e) =>
                    setConfig({
                      ...config,
                      waveform: { ...config.waveform, log_knee: Number(e.target.value) },
                    })
                  }
                />
                <InputField
                  label="Force Yellow (g)"
                  type="number"
                  step={0.01}
                  value={config.waveform.force_yellow_g}
                  onChange={(e) =>
                    setConfig({
                      ...config,
                      waveform: { ...config.waveform, force_yellow_g: Number(e.target.value) },
                    })
                  }
                />
                <InputField
                  label="Force Red (g)"
                  type="number"
                  step={0.01}
                  value={config.waveform.force_red_g}
                  onChange={(e) =>
                    setConfig({
                      ...config,
                      waveform: { ...config.waveform, force_red_g: Number(e.target.value) },
                    })
                  }
                />
                <InputField
                  label="Amp Scale"
                  type="number"
                  step={0.1}
                  value={config.waveform.amp_scale}
                  onChange={(e) =>
                    setConfig({
                      ...config,
                      waveform: { ...config.waveform, amp_scale: Number(e.target.value) },
                    })
                  }
                />
                <label className="flex items-center gap-2 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={config.waveform.swap_xy}
                    onChange={(e) =>
                      setConfig({
                        ...config,
                        waveform: { ...config.waveform, swap_xy: e.target.checked },
                      })
                    }
                    className="rounded border-slate-300/20 bg-slate-800 text-blue-600 focus:ring-blue-500"
                  />
                  <span className="text-sm font-medium text-gray-300">Swap X/Y</span>
                </label>
              </CardContent>
            </Card>

            <Card padding="lg">
              <CardHeader>
                <h2 className="text-sm font-semibold text-slate-300">Visualization</h2>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="grid grid-cols-2 gap-4">
                  <InputField
                    label="Width"
                    type="number"
                    value={config.viz.width}
                    onChange={(e) =>
                      setConfig({ ...config, viz: { ...config.viz, width: Number(e.target.value) } })
                    }
                  />
                  <InputField
                    label="Height"
                    type="number"
                    value={config.viz.height}
                    onChange={(e) =>
                      setConfig({ ...config, viz: { ...config.viz, height: Number(e.target.value) } })
                    }
                  />
                </div>
              </CardContent>
            </Card>

            <div className="flex items-center gap-3">
              <Button
                size="MD"
                theme="primary"
                text={saving ? "Saving..." : "Save"}
                disabled={saving}
                loading={saving}
                fullWidth
                onClick={() => void handleSave()}
              />
              {saved && (
                <span className="text-sm text-green-400 shrink-0">Saved</span>
              )}
            </div>
          </div>

          <div className="flex-1 min-w-0">
            <div className="sticky top-4">
              <Card padding="none" className="overflow-hidden">
                <div className="px-4 py-3 border-b border-slate-200 dark:border-slate-700 flex items-center justify-between gap-2">
                  <h2 className="text-sm font-semibold text-slate-300">Canvas Preview</h2>
                  <input
                    type="text"
                    readOnly
                    value={canvasUrl}
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
                      src={canvasUrl}
                      className="w-full h-full border-0 rounded"
                      style={{ background: "transparent" }}
                      title="Canvas Preview"
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
