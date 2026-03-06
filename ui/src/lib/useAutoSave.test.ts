import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

describe("useAutoSave debounce logic", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("debounces multiple calls into one", () => {
    const fn = vi.fn();
    let timer: number | undefined;

    const debouncedSave = (delay: number) => {
      clearTimeout(timer);
      timer = globalThis.setTimeout(() => { fn(); }, delay) as unknown as number;
    };

    debouncedSave(500);
    debouncedSave(500);
    debouncedSave(500);

    expect(fn).not.toHaveBeenCalled();
    vi.advanceTimersByTime(500);
    expect(fn).toHaveBeenCalledTimes(1);
  });

  it("resets timer on each call", () => {
    const fn = vi.fn();
    let timer: number | undefined;

    const debouncedSave = (delay: number) => {
      clearTimeout(timer);
      timer = globalThis.setTimeout(() => { fn(); }, delay) as unknown as number;
    };

    debouncedSave(500);
    vi.advanceTimersByTime(300);
    debouncedSave(500);
    vi.advanceTimersByTime(300);
    expect(fn).not.toHaveBeenCalled();
    vi.advanceTimersByTime(200);
    expect(fn).toHaveBeenCalledTimes(1);
  });

  it("fires immediately after delay expires", () => {
    const fn = vi.fn();
    let timer: number | undefined;

    const debouncedSave = (delay: number) => {
      clearTimeout(timer);
      timer = globalThis.setTimeout(() => { fn(); }, delay) as unknown as number;
    };

    debouncedSave(500);
    vi.advanceTimersByTime(499);
    expect(fn).not.toHaveBeenCalled();
    vi.advanceTimersByTime(1);
    expect(fn).toHaveBeenCalledTimes(1);
  });
});
