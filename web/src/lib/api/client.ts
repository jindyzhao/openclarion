import type { components } from "./openapi";

type ErrorResponse = components["schemas"]["ErrorResponse"];

type ApiError = {
  message: string;
  status?: number;
};

export type ApiResult<T> =
  | { ok: true; data: T }
  | { ok: false; error: ApiError };

export type RequestJSONOptions = {
  method?: "DELETE" | "GET" | "PATCH" | "POST" | "PUT";
  body?: unknown;
  headers?: HeadersInit;
};

const defaultAPIBaseURL = "http://localhost:8080";

export async function requestJSON<T>(path: string, options: RequestJSONOptions = {}): Promise<ApiResult<T>> {
  const baseURL = process.env.OPENCLARION_API_BASE_URL ?? defaultAPIBaseURL;
  let response: Response;
  try {
    const headers = new Headers(options.headers);
    if (!headers.has("accept")) {
      headers.set("accept", "application/json");
    }
    if (options.body !== undefined && !headers.has("content-type")) {
      headers.set("content-type", "application/json");
    }

    response = await fetch(new URL(path, baseURL), {
      cache: "no-store",
      method: options.method ?? "GET",
      headers,
      body: options.body === undefined ? undefined : JSON.stringify(options.body)
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

  if (response.status === httpNoContent) {
    return { ok: true, data: undefined as T };
  }

  return parseJSONResponse<T>(response);
}

const httpNoContent = 204;
const invalidJSONMessage = "Response body must be valid JSON.";

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

async function parseJSONResponse<T>(response: Response): Promise<ApiResult<T>> {
  try {
    return { ok: true, data: (await response.json()) as T };
  } catch {
    return {
      ok: false,
      error: { message: invalidJSONMessage, status: 502 }
    };
  }
}
