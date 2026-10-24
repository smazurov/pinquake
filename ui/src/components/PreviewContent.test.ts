import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import PreviewContent from "./PreviewContent";

describe("PreviewContent", () => {
  it("keeps the same viewport-sized area when the preview is off", () => {
    const html = renderToStaticMarkup(createElement(PreviewContent, {
      url: null,
      label: "Off",
      dimensions: null,
    }));

    expect(html).toContain("Preview off");
    expect(html).not.toContain("<iframe");
    expect(html).toContain("height:calc(100dvh - 12rem)");
  });

  it.each([
    { label: "Plumb Bob", width: 400, height: 400 },
    { label: "Crosshair", width: 200, height: 200 },
    { label: "Waveform", width: 608, height: 1080 },
    { label: "Experiment", width: 400, height: 400 },
    { label: "Custom", width: 320, height: 180 },
  ])("fits $label inside a stable full-width frame without changing its dimensions", ({ label, width, height }) => {
    const url = `/canvas?width=${width}&height=${height}`;
    const html = renderToStaticMarkup(createElement(PreviewContent, {
      url,
      label,
      dimensions: { width, height },
    }));

    // Leave room for the navbar, preview toolbar, and page padding.
    expect(html).toContain("height:calc(100dvh - 12rem)");
    expect(html).toContain("w-full");
    expect(html).not.toContain("aspect-ratio:");
    expect(html).toContain(`width="${width}"`);
    expect(html).toContain(`height="${height}"`);
    expect(html).toContain("absolute left-1/2 top-1/2");
    expect(html).toContain(`src="${url.replaceAll("&", "&amp;")}"`);
  });
});
