// Replace the app's axios adapter in tests, so components talk to a fake server.
import type { AxiosAdapter, AxiosResponse, InternalAxiosRequestConfig } from "axios";
import { http } from "../lib/api/http";

export interface RecordedRequest {
  method: string;
  url: string;
  params?: unknown;
  data?: unknown;
}

export type FakeRoute = (request: RecordedRequest) => unknown | Promise<unknown>;

export interface FakeServer {
  requests: RecordedRequest[];
  restore: () => void;
}

export class FakeHttpError extends Error {
  status: number;
  body: unknown;

  constructor(status: number, body: unknown = { error: `HTTP ${status}` }) {
    super(`HTTP ${status}`);
    this.status = status;
    this.body = body;
  }
}

// `routes` maps "METHOD /path" (no query string) to a handler returning the JSON body.
// Unmatched requests fail with 404, so a test notices calls it did not expect.
export function fakeServer(routes: Record<string, FakeRoute>): FakeServer {
  const original = http.defaults.adapter;
  const requests: RecordedRequest[] = [];
  const adapter: AxiosAdapter = async (config: InternalAxiosRequestConfig) => {
    const method = (config.method ?? "get").toUpperCase();
    const url = (config.url ?? "").split("?")[0] ?? "";
    let data: unknown = config.data;
    if (typeof data === "string") {
      try {
        data = JSON.parse(data);
      } catch {
        // leave non-JSON bodies as they are
      }
    }
    const request: RecordedRequest = { method, url: config.url ?? "", params: config.params, data };
    requests.push(request);
    const handler = routes[`${method} ${url}`];
    const respond = (status: number, body: unknown): AxiosResponse => ({
      data: body,
      status,
      statusText: String(status),
      headers: {},
      config,
    });
    if (!handler) {
      const error = Object.assign(new Error(`Unexpected ${method} ${url}`), {
        isAxiosError: true,
        config,
        response: respond(404, { error: `no fake route for ${method} ${url}` }),
      });
      throw error;
    }
    try {
      return respond(200, await handler(request));
    } catch (failure) {
      if (failure instanceof FakeHttpError) {
        throw Object.assign(new Error(failure.message), {
          isAxiosError: true,
          config,
          response: respond(failure.status, failure.body),
        });
      }
      throw failure;
    }
  };
  http.defaults.adapter = adapter;
  return {
    requests,
    restore: () => {
      http.defaults.adapter = original;
    },
  };
}

// Resolves after pending promises and Vue updates have run.
export async function flushPromises(rounds = 5): Promise<void> {
  for (let i = 0; i < rounds; i += 1) {
    await new Promise((resolve) => setTimeout(resolve, 0));
  }
}
