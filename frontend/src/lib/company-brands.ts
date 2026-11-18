import { escapeHTML as esc } from "./logic";

// Local copies of the companies' published marks. See public/logos/SOURCES.md.
export const companyBrands: Record<string, { file: string; onInk?: boolean }> = {
  mako: { file: "mako.svg" },
  ctc: { file: "ctc.svg" },
  maven: { file: "maven.png", onInk: true },
  "da-vinci": { file: "da-vinci.svg" },
  sig: { file: "sig.svg", onInk: true },
  drw: { file: "drw.svg", onInk: true },
  akuna: { file: "akuna.svg", onInk: true },
  citadel: { file: "citadel.png" },
  wincent: { file: "wincent.svg", onInk: true },
  flow: { file: "flow.png" },
  "all-options": { file: "all-options.svg" },
};

export function companyLogo(companyID: string, companyName: string): string {
  const brand = companyBrands[companyID];
  if (!brand) return "";
  return `<span class="company-logo company-logo--${esc(companyID)}${brand.onInk ? " company-logo--on-ink" : ""}"><img src="/logos/${esc(brand.file)}" alt="${esc(companyName)} logo" width="220" height="72" loading="lazy" decoding="async" /></span>`;
}
