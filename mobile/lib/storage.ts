import AsyncStorage from "@react-native-async-storage/async-storage";
import { getRandomBytes } from "expo-crypto";
import type { Course, Mark, Role } from "./types";

const K = {
  secret: "device_secret",
  role: "role",
  courses: "courses",
  pending: "pending_marks",
};

function randomHex(bytes: number): string {
  return Array.from(getRandomBytes(bytes), (b) => b.toString(16).padStart(2, "0")).join("");
}

// Секрет устройства: 32 hex-символа, создаётся один раз.
// Промис запоминается, чтобы параллельные первые вызовы получили один и тот же секрет.
let secretPromise: Promise<string> | null = null;
export function getDeviceSecret(): Promise<string> {
  if (!secretPromise) {
    secretPromise = (async () => {
      const saved = await AsyncStorage.getItem(K.secret);
      if (saved) return saved;
      const fresh = randomHex(16);
      await AsyncStorage.setItem(K.secret, fresh);
      return fresh;
    })();
    secretPromise.catch(() => {
      secretPromise = null; // ошибку не кэшируем
    });
  }
  return secretPromise;
}

export async function getRole(): Promise<Role | null> {
  const r = await AsyncStorage.getItem(K.role);
  return r === "patient" || r === "relative" ? r : null;
}

const roleListeners = new Set<(r: Role) => void>();

export function onRoleChange(fn: (r: Role) => void): () => void {
  roleListeners.add(fn);
  return () => {
    roleListeners.delete(fn);
  };
}

export async function setRole(role: Role): Promise<void> {
  await AsyncStorage.setItem(K.role, role);
  roleListeners.forEach((fn) => fn(role));
}

async function readJson<T>(key: string, fallback: T): Promise<T> {
  const raw = await AsyncStorage.getItem(key);
  if (!raw) return fallback;
  try {
    return JSON.parse(raw) as T;
  } catch (e) {
    console.warn("storage: повреждённые данные", key, e);
    return fallback;
  }
}

export function loadCourses(): Promise<Course[]> {
  return readJson<Course[]>(K.courses, []);
}

export async function saveCourses(courses: Course[]): Promise<void> {
  await AsyncStorage.setItem(K.courses, JSON.stringify(courses));
}

export function getPendingMarks(): Promise<Mark[]> {
  return readJson<Mark[]>(K.pending, []);
}

export async function setPendingMarks(marks: Mark[]): Promise<void> {
  await AsyncStorage.setItem(K.pending, JSON.stringify(marks));
}
