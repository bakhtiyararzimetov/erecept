-- Английские названия и вещества для английской версии сайта и приложения.
ALTER TABLE medicines ADD COLUMN name_en TEXT NOT NULL DEFAULT '';
ALTER TABLE medicines ADD COLUMN substance_en TEXT NOT NULL DEFAULT '';
CREATE INDEX medicines_name_en_idx ON medicines (lower(name_en));

-- Справочник уже в базе (демо на Render): заполняем по русскому названию.
UPDATE medicines m SET name_en = v.name_en, substance_en = v.substance_en
FROM (VALUES
    ('Парацетамол', 'Paracetamol', 'paracetamol'),
    ('Панадол', 'Panadol', 'paracetamol'),
    ('Ибупрофен', 'Ibuprofen', 'ibuprofen'),
    ('Нурофен', 'Nurofen', 'ibuprofen'),
    ('Амоксициллин', 'Amoxicillin', 'amoxicillin'),
    ('Флемоксин Солютаб', 'Flemoxin Solutab', 'amoxicillin'),
    ('Азитромицин', 'Azithromycin', 'azithromycin'),
    ('Сумамед', 'Sumamed', 'azithromycin'),
    ('Омепразол', 'Omeprazole', 'omeprazole'),
    ('Омез', 'Omez', 'omeprazole'),
    ('Метформин', 'Metformin', 'metformin'),
    ('Глюкофаж', 'Glucophage', 'metformin'),
    ('Лизиноприл', 'Lisinopril', 'lisinopril'),
    ('Амлодипин', 'Amlodipine', 'amlodipine'),
    ('Аторвастатин', 'Atorvastatin', 'atorvastatin'),
    ('Цетиризин', 'Cetirizine', 'cetirizine'),
    ('Зодак', 'Zodac', 'cetirizine'),
    ('Лоратадин', 'Loratadine', 'loratadine'),
    ('Амброксол', 'Ambroxol', 'ambroxol'),
    ('Ацетилцистеин', 'Acetylcysteine', 'acetylcysteine'),
    ('Метронидазол', 'Metronidazole', 'metronidazole'),
    ('Ципрофлоксацин', 'Ciprofloxacin', 'ciprofloxacin'),
    ('Дротаверин', 'Drotaverine', 'drotaverine'),
    ('Но-шпа', 'No-Spa', 'drotaverine'),
    ('Панкреатин', 'Pancreatin', 'pancreatin'),
    ('Лоперамид', 'Loperamide', 'loperamide'),
    ('Нимесулид', 'Nimesulide', 'nimesulide'),
    ('Диклофенак', 'Diclofenac', 'diclofenac'),
    ('Валсартан', 'Valsartan', 'valsartan'),
    ('Бисопролол', 'Bisoprolol', 'bisoprolol')
) AS v(name, name_en, substance_en)
WHERE m.name = v.name;
