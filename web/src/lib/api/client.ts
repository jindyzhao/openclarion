import type { components } from "./openapi";

type ErrorResponse = components["schemas"]["ErrorResponse"];

type ApiError = {
  message: string;
  status?: number;
};

export type ApiResult<T> =
  | { ok: true; data: T }
  | { ok: false; error: ApiError };

const defaultAPIBaseURL = "http://localhost:8080";

export async function requestJSON<T>(path: string): Promise<ApiResult<T>> {
  const baseURL = process.env.OPENCLARION_API_BASE_URL ?? defaultAPIBaseURL;
  let response: Response;
  try {
    response = await fetch(new URL(path, baseURL), {
      cache: "no-store",
      headers: { accept: "application/json" }
    });
  } catch (error) {
    return {
      ok: false,
      error: { message: error instanceof Error ? error.message : "Request failed." }
    };
  }

  if (!response.ok) {
    return {
      ok: false,
      error: {
        message: await errorMessage(response),
        status: response.status
      }
    };
  }

  return { ok: true, data: (await response.json()) as T };
}

async function errorMessage(response: Response): Promise<string> {
  try {
    const body = (await response.json()) as Partial<ErrorResponse>;
    if (typeof body.error === "string" && body.error.trim() !== "") {
      return body.error;
    }
  } catch {
    // Fall through to the HTTP status line.
  }
  return response.statusText || `HTTP ${response.status}`;
}
