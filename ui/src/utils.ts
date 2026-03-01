import { twMerge } from "tailwind-merge";
import { cva, type VariantProps } from "cva";

export function cn(...inputs: (string | undefined)[]) {
  return twMerge(inputs.filter(Boolean).join(" "));
}

export { cva, type VariantProps };
