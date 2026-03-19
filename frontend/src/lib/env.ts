import { z } from "zod";

const apiBaseUrlSchema = z
  .string()
  .trim()
  .min(1, "VITE_API_BASE_URL is required")
  .refine(
    (value) => {
      if (value.startsWith("/")) {
        return true;
      }

      try {
        new URL(value);
        return true;
      } catch {
        return false;
      }
    },
    {
      message: "VITE_API_BASE_URL must be an absolute URL or a relative path starting with /",
    },
  );

const envSchema = z.object({
  VITE_API_BASE_URL: apiBaseUrlSchema,
});

const parsed = envSchema.safeParse(import.meta.env);

if (!parsed.success) {
  console.error("Invalid environment variables:", parsed.error.flatten());
  throw new Error("Missing or invalid environment variables");
}

export const env = parsed.data;
