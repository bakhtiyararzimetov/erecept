// Типы повторяют JSON контракта REST API (snake_case).
export type Role = "patient" | "relative";

export type Meal = "before" | "after" | "with" | "any" | string;

export type Explanation = {
  purpose: string;
  how_to_take: string;
  do_not_take_if: string[];
  missed_dose: string;
  if_stopped_early: string;
  urgent_signs: string[];
  high_risk: boolean;
  source: { title: string; reg_number: string; url: string };
  model: string;
  generated_at: string;
};

export type Item = {
  id: number;
  medicine_name: string;
  medicine_id: number | null;
  dosage: string;
  times_per_day: number;
  days: number;
  meal: Meal;
  meal_label: string;
  times: string[]; // "HH:MM"
  photo_url: string;
  image?: string; // картинка упаковки (иллюстрация), путь на сервере
  explanation?: Explanation | null; // в старых сохранённых курсах поля нет
};

export type Prescription = {
  id: number;
  code: string;
  patient_name: string;
  patient_birth: string; // YYYY-MM-DD
  doctor_name: string;
  comment: string;
  water_every_hours: number;
  water_from: string; // HH:MM
  water_to: string; // HH:MM
  start_date: string | null; // YYYY-MM-DD
  cancelled?: boolean; // врач отменил рецепт: приёмов и напоминаний нет
  items: Item[];
};

export type Dose = {
  id: number;
  item_id: number;
  scheduled_at: string; // RFC3339, +05:00
  day_number: number;
  total_days: number;
  taken_at: string | null;
};

export type Course = { prescription: Prescription; doses: Dose[] };

export type Mark = { dose_id: number; taken_at: string };

export type MedicineRef = {
  id: number;
  name: string;
  active_substance: string;
  form: string;
  image?: string; // картинка упаковки (иллюстрация)
};

export type Pharmacy = {
  id: number;
  name: string;
  address: string;
  phone: string;
  work_hours: string;
  lat: number;
  lng: number;
};

export type Offer = {
  pharmacy: Pharmacy;
  price: number;
  in_stock: boolean;
  distance_km: number | null;
  updated_at: string;
};

export type Analog = {
  medicine: MedicineRef;
  min_price: number;
  pharmacies_count: number;
};

export type OffersResponse = {
  medicine: MedicineRef | null;
  offers: Offer[];
  analogs: Analog[];
  analog_note: string;
};

export type RecentMissed = { medicine_name: string; scheduled_at: string };

export type RelativePatient = {
  patient_name: string;
  taken: number;
  missed: number;
  upcoming: number;
  recent_missed: RecentMissed[];
};

export type RelativeOverview = { patients: RelativePatient[] };

export type OffersParams = {
  name?: string;
  medicine_id?: number;
  sort?: "price" | "distance";
  lat?: number;
  lng?: number;
};
