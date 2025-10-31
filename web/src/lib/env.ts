import z from "zod/v3";

const isDev = z.boolean().parse(import.meta.env.DEV);

const env = z.object({
  VITE_API_URL: z.string(),
});

export const Env = env.parse({
  VITE_API_URL: isDev ? "http://localhost:8080" : "",
});
