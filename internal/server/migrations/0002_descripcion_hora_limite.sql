-- Descripción, hora y fecha límite. Las notas pasan a ser comentarios con
-- fecha ({"date": "AAAA-MM-DD" | null, "text": "…"}); las notas anteriores
-- (texto suelto, venían de las descripciones de Todoist) pasan a la descripción.

ALTER TABLE tasks ADD COLUMN description TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN time TEXT;       -- HH:MM
ALTER TABLE tasks ADD COLUMN deadline TEXT;   -- AAAA-MM-DD

UPDATE tasks
SET description = COALESCE(
      (SELECT group_concat(value, char(10)) FROM json_each(tasks.notes) WHERE json_each.type = 'text'),
      ''),
    notes = '[]'
WHERE EXISTS (SELECT 1 FROM json_each(tasks.notes) WHERE json_each.type = 'text');
