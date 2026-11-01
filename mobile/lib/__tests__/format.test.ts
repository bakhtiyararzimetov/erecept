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
