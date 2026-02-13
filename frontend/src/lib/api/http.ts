import axios, {
  type AxiosRequestConfig,
  type AxiosResponse,
} from "axios";

export const http = axios.create({
  headers: {
    "Content-Type": "application/json",
  },
});

async function unwrap<T>(request: Promise<AxiosResponse<T>>): Promise<T> {
  const response = await request;
  return response.data;
}

export const httpClient = {
  get<T>(url: string, config?: AxiosRequestConfig): Promise<T> {
    return unwrap<T>(http.get<T>(url, config));
  },
  post<TResponse, TBody = unknown>(
    url: string,
    body?: TBody,
    config?: AxiosRequestConfig,
  ): Promise<TResponse> {
    return unwrap<TResponse>(http.post<TResponse>(url, body, config));
  },
  patch<TResponse, TBody = unknown>(
    url: string,
    body?: TBody,
    config?: AxiosRequestConfig,
  ): Promise<TResponse> {
    return unwrap<TResponse>(http.patch<TResponse>(url, body, config));
  },
  del<TResponse>(url: string, config?: AxiosRequestConfig): Promise<TResponse> {
    return unwrap<TResponse>(http.delete<TResponse>(url, config));
  },
};

export function getErrorMessage(error: unknown, fallback: string): string {
  if (axios.isAxiosError(error) && error.response?.data) {
    const payload = error.response.data as Record<string, unknown>;
    if (typeof payload.message === "string" && payload.message.length > 0) {
      return payload.message;
    }
    if (typeof payload.error === "string" && payload.error.length > 0) {
      return payload.error;
    }
  }
  return fallback;
}
