import AsyncStorage from "@react-native-async-storage/async-storage";
import type { Course, Mark, Role } from "./types";

const K = {
  secret: "device_secret",
  role: "role",
  courses: "courses",
  pending: "pending_marks",
};

function randomHex(bytes: number): string {
  let s = "";
  for (let i = 0; i < bytes; i++) {
    s += Math.floor(Math.random() * 256).toString(16).padStart(2, "0");
  }
  return s;
}

// Секрет устройства: 32 hex-символа, создаётся один раз.
export async function getDeviceSecret(): Promise<string> {
  const saved = await AsyncStorage.getItem(K.secret);
  if (saved) return saved;
  const fresh = randomHex(16);
  await AsyncStorage.setItem(K.secret, fresh);
  return fresh;
}

export async function getRole(): Promise<Role | null> {
  const r = await AsyncStorage.getItem(K.role);
  return r === "patient" || r === "relative" ? r : null;
}

export async function setRole(role: Role): Promise<void> {
  await AsyncStorage.setItem(K.role, role);
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
