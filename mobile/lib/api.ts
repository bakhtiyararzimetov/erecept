import { getDeviceSecret } from "./storage";
import type {
  Course,
  Mark,
  OffersParams,
  OffersResponse,
  RelativeOverview,
} from "./types";

export const API_URL = process.env.EXPO_PUBLIC_API_URL ?? "https://erecept.onrender.com";

export class ApiError extends Error {
  status: number;
  constructor(message: string, status: number) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

async function request<T>(method: string, path: string, body?: unknown, form?: FormData): Promise<T> {
  const headers: Record<string, string> = { "X-Device-Secret": await getDeviceSecret() };
  let payload: BodyInit | undefined;
  if (form) {
    payload = form; // Content-Type с boundary выставит сам fetch
  } else if (body !== undefined) {
    headers["Content-Type"] = "application/json";
    payload = JSON.stringify(body);
  }
  let res: Response;
  try {
    res = await fetch(API_URL + path, { method, headers, body: payload });
  } catch (e) {
    console.warn("api: сеть недоступна", method, path, e);
    throw new ApiError("Нет связи с сервером", 0);
  }
  let data: any = null;
  try {
    data = await res.json();
  } catch {
    // тело не JSON — обработаем ниже
  }
  if (!res.ok) {
    throw new ApiError(
      (data && typeof data.error === "string" && data.error) || "Нет связи с сервером",
      res.status,
    );
  }
  return data as T;
}

function query(params: Record<string, string | number | undefined>): string {
  const parts = Object.entries(params)
    .filter(([, v]) => v !== undefined && v !== "")
    .map(([k, v]) => `${encodeURIComponent(k)}=${encodeURIComponent(String(v))}`);
  return parts.length ? "?" + parts.join("&") : "";
}

export const api = {
  claim: (code: string) =>
    request<Course>("POST", `/api/prescriptions/${encodeURIComponent(code)}/claim`).then(
      (r: any) => ({ prescription: r.prescription, doses: r.doses }) as Course,
    ),
  myPrescriptions: () => request<Course[]>("GET", "/api/me/prescriptions"),
  sync: (marks: Mark[]) => request<{ updated: number }>("POST", "/api/doses/sync", { marks }),
  pushToken: (token: string) => request<{ ok: boolean }>("POST", "/api/push-token", { token }),
  sos: () => request<{ notified: number; throttled?: boolean }>("POST", "/api/sos"),
  createInvite: () => request<{ code: string; expires_at: string }>("POST", "/api/invites"),
  joinRelative: (code: string, name: string) =>
    request<{ patient_name: string }>("POST", "/api/relatives/join", { code, name }),
  relativeOverview: () => request<RelativeOverview>("GET", "/api/relative/overview"),
  offers: (p: OffersParams) =>
    request<OffersResponse>(
      "GET",
      "/api/offers" +
        query({ name: p.name, medicine_id: p.medicine_id, sort: p.sort, lat: p.lat, lng: p.lng }),
    ),
  uploadPhoto: (itemId: number, uri: string) => {
    const form = new FormData();
    const ext = uri.toLowerCase().endsWith(".png") ? "png" : "jpg";
    // React Native принимает объект {uri, name, type} вместо Blob
    form.append("photo", { uri, name: `photo.${ext}`, type: ext === "png" ? "image/png" : "image/jpeg" } as any);
    return request<{ photo_url: string }>("POST", `/api/items/${itemId}/photo`, undefined, form);
  },
};
