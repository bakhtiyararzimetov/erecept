import { openStatus } from "../format";
import { formatKm, formatPrice } from "../format";

describe("formatPrice", () => {
  it("разделяет тысячи узким пробелом и добавляет ₸", () => {
    expect(formatPrice(1250)).toBe("1 250 ₸");
    expect(formatPrice(1234567)).toBe("1 234 567 ₸");
  });
  it("не делит числа до 1000", () => {
    expect(formatPrice(0)).toBe("0 ₸");
    expect(formatPrice(999)).toBe("999 ₸");
  });
  it("округляет дробные", () => {
    expect(formatPrice(1249.6)).toBe("1 250 ₸");
  });
});

describe("formatKm", () => {
  it("использует запятую и один знак", () => {
    expect(formatKm(2.4)).toBe("2,4 км");
    expect(formatKm(3)).toBe("3,0 км");
    expect(formatKm(0.04)).toBe("0,0 км");
    expect(formatKm(12.36)).toBe("12,4 км");
  });
});

describe("openStatus", () => {
  const at = (h: number, m = 0) => new Date(2026, 10, 14, h, m);
  test("в часы работы — open, вне — closed", () => {
    expect(openStatus("08:00–22:00", at(9))).toBe("open");
    expect(openStatus("08:00–22:00", at(22, 30))).toBe("closed");
    expect(openStatus("08:30-22:00", at(8, 15))).toBe("closed");
  });
  test("круглосуточно и через полночь", () => {
    expect(openStatus("Круглосуточно", at(3))).toBe("open");
    expect(openStatus("24 hours", at(3))).toBe("open");
    expect(openStatus("20:00–02:00", at(1))).toBe("open");
    expect(openStatus("20:00–02:00", at(12))).toBe("closed");
  });
  test("непонятный формат — null", () => {
    expect(openStatus("по записи", at(9))).toBeNull();
  });
});
