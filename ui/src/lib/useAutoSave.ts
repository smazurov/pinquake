import { useEffect, useRef, useState, useCallback } from "react";

export type AutoSaveStatus = "idle" | "saving" | "saved" | "error";

interface AutoSaveResult {
  status: AutoSaveStatus;
  error: string | null;
}

export function useAutoSave<T>(
  value: T | null,
  saveFn: (val: T) => Promise<{ error?: { detail?: string } }>,
  { delay = 500 }: { delay?: number } = {},
): AutoSaveResult {
  const [status, setStatus] = useState<AutoSaveStatus>("idle");
  const [error, setError] = useState<string | null>(null);
  const timerRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const lastSavedRef = useRef<string | null>(null);
  const valueRef = useRef(value);

  useEffect(() => {
    valueRef.current = value;
  }, [value]);

  const save = useCallback(async () => {
    const val = valueRef.current;
    if (!val) return;
    setStatus("saving");
    setError(null);
    const { error: err } = await saveFn(val);
    if (err) {
      setError(err.detail ?? "Save failed");
      setStatus("error");
    } else {
      lastSavedRef.current = JSON.stringify(val);
      setStatus("saved");
      setTimeout(() => setStatus((s) => (s === "saved" ? "idle" : s)), 1500);
    }
  }, [saveFn]);

  useEffect(() => {
    if (!value) return;

    const serialized = JSON.stringify(value);

    if (lastSavedRef.current === null) {
      lastSavedRef.current = serialized;
      return;
    }

    if (serialized === lastSavedRef.current) return;

    clearTimeout(timerRef.current);
    timerRef.current = setTimeout(() => void save(), delay);

    return () => clearTimeout(timerRef.current);
  }, [value, delay, save]);

  return { status, error };
}
