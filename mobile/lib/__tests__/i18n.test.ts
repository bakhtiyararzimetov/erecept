import { en, getLang, setLang, t, tf } from "../i18n";

test("по умолчанию русский, без перевода — исходная строка", () => {
  expect(getLang()).toBe("ru");
  expect(t("Сегодня")).toBe("Сегодня");
  expect(tf("день {0} из {1}", 2, 7)).toBe("день 2 из 7");
});

test("английский: перевод и подстановка", async () => {
  await setLang("en").catch(() => undefined); // в тестах нет AsyncStorage
  expect(t("Сегодня")).toBe("Today");
  expect(tf("день {0} из {1}", 2, 7)).toBe("day 2 of 7");
  expect(t("нет такой строки")).toBe("нет такой строки");
});

test("у каждой строки перевода нет русских букв", () => {
  for (const [ru, value] of Object.entries(en)) {
    expect([ru, /[а-яё]/i.test(value)]).toEqual([ru, false]);
  }
});
