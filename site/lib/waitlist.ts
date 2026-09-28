/** Answers to "What will you deploy?" on the waitlist form. */
export const useCases = [
  { value: "internal-tool", label: "Internal tool" },
  { value: "client-dashboard", label: "Client dashboard" },
  { value: "saas-mvp", label: "SaaS MVP" },
  { value: "other", label: "Other" },
] as const;

export type UseCase = (typeof useCases)[number]["value"];

export function isUseCase(value: string): value is UseCase {
  return useCases.some((c) => c.value === value);
}

export function labelForUseCase(value: string): string | undefined {
  return useCases.find((c) => c.value === value)?.label;
}
