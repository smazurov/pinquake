import { useEffect, useRef, useState, useCallback } from "react";
import { api } from "./api";
import type { components } from "./api.generated";

type PinQuakeConfig = components["schemas"]["PinQuakeConfig"];

export type AutoSaveStatus = "idle" | "saving" | "saved" | "error";

interface AutoSaveResult {
  status: AutoSaveStatus;
  error: string | null;
}

export function useAutoSave(
  config: PinQuakeConfig | null,
  { delay = 500 }: { delay?: number } = {},
): AutoSaveResult {
  const [status, setStatus] = useState<AutoSaveStatus>("idle");
  const [error, setError] = useState<string | null>(null);
  const timerRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const lastSavedRef = useRef<string | null>(null);
  const configRef = useRef(config);

  useEffect(() => {
    configRef.current = config;
  }, [config]);

  const save = useCallback(async () => {
    const cfg = configRef.current;
    if (!cfg) return;
    setStatus("saving");
    setError(null);
    const { error: err } = await api.PUT("/api/config", { body: cfg });
    if (err) {
      setError(err.detail ?? "Save failed");
      setStatus("error");
    } else {
      lastSavedRef.current = JSON.stringify(cfg);
      setStatus("saved");
      setTimeout(() => setStatus((s) => (s === "saved" ? "idle" : s)), 1500);
    }
  }, []);

  useEffect(() => {
    if (!config) return;

    const serialized = JSON.stringify(config);

    // On first load, just record the snapshot — don't save
    if (lastSavedRef.current === null) {
      lastSavedRef.current = serialized;
      return;
    }

    // Don't save if config hasn't actually changed
    if (serialized === lastSavedRef.current) return;

    clearTimeout(timerRef.current);
    timerRef.current = setTimeout(() => void save(), delay);

    return () => clearTimeout(timerRef.current);
  }, [config, delay, save]);

  return { status, error };
}
