-- Esquema inicial de tasktui en D1.
-- Las columnas reflejan tasktui_core::dto::TaskDto.

CREATE TABLE tasks (
  id          TEXT PRIMARY KEY,
  text        TEXT    NOT NULL,
  done        INTEGER NOT NULL DEFAULT 0,
  due         TEXT,                          -- AAAA-MM-DD
  done_date   TEXT,                          -- AAAA-MM-DD
  priority    INTEGER NOT NULL DEFAULT 4,    -- 1 (máxima) … 4 (sin prioridad)
  labels      TEXT    NOT NULL DEFAULT '[]', -- JSON
  recurrence  TEXT,                          -- regla de Obsidian Tasks: «every week on Monday»
  project     TEXT    NOT NULL DEFAULT 'Inbox',
  section     TEXT,
  notes       TEXT    NOT NULL DEFAULT '[]', -- JSON
  position    REAL    NOT NULL,              -- orden dentro del proyecto
  created_at  INTEGER NOT NULL,              -- ms desde 1970
  updated_at  INTEGER NOT NULL
);

CREATE INDEX tasks_by_project ON tasks (project, position);
CREATE INDEX tasks_by_due ON tasks (done, due);

-- Proyectos explícitos (para conservar los que aún no tienen tareas).
CREATE TABLE projects (
  name        TEXT PRIMARY KEY COLLATE NOCASE,
  created_at  INTEGER NOT NULL
);

INSERT INTO projects (name, created_at) VALUES ('Inbox', 0);
