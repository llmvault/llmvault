import { fetchClient } from "@/api/client";
import type { NangoProvider } from "./utils";

export async function listProviders(): Promise<NangoProvider[]> {
  const { data } = await fetchClient.GET("/v1/integrations/providers");
  return (data ?? []) as NangoProvider[];
}

export async function createIntegration(body: {
  provider: string;
  display_name: string;
  credentials?: Record<string, unknown>;
  meta?: Record<string, unknown>;
}) {
  const { data } = await fetchClient.POST("/v1/integrations", {
    body: body as never,
  });
  return data!;
}

export async function updateIntegration(
  id: string,
  body: {
    display_name?: string;
    credentials?: Record<string, unknown>;
    meta?: Record<string, unknown>;
  },
) {
  const { data } = await fetchClient.PUT("/v1/integrations/{id}", {
    params: { path: { id } },
    body: body as never,
  });
  return data!;
}

export async function deleteIntegration(id: string) {
  await fetchClient.DELETE("/v1/integrations/{id}", {
    params: { path: { id } },
  });
}
