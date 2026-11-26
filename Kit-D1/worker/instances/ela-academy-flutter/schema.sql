-- ELA Academy — esquema D1 para el worker de sincronización.
-- La identidad cloud es sync_id (UUID estable entre dispositivos); id es
-- local y lo asigna D1. settings se identifica por key.

CREATE TABLE IF NOT EXISTS teacher (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  sync_id TEXT UNIQUE,
  name TEXT NOT NULL,
  email TEXT,
  phone TEXT,
  address TEXT,
  level TEXT,
  modality TEXT,
  permissions TEXT,
  created_at TEXT,
  updated_at TEXT,
  deleted_at TEXT,
  device_id TEXT,
  sk TEXT
);

CREATE TABLE IF NOT EXISTS curso (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  sync_id TEXT UNIQUE,
  teacher_id INTEGER,
  name TEXT NOT NULL,
  level TEXT,
  modality TEXT,
  start_year INTEGER,
  end_year INTEGER,
  status TEXT DEFAULT 'active',
  created_at TEXT,
  updated_at TEXT,
  device_id TEXT,
  sk TEXT
);

CREATE TABLE IF NOT EXISTS student (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  sync_id TEXT UNIQUE,
  name TEXT NOT NULL,
  email TEXT,
  phone TEXT,
  birth_date TEXT,
  enrollment_date TEXT,
  teacher_id INTEGER,
  level TEXT,
  modality TEXT,
  gender TEXT,
  status TEXT DEFAULT 'active',
  notes TEXT,
  curso_id INTEGER,
  final_grade REAL,
  created_at TEXT,
  updated_at TEXT,
  deleted_at TEXT,
  device_id TEXT,
  sk TEXT
);

CREATE TABLE IF NOT EXISTS student_curso_history (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  sync_id TEXT UNIQUE,
  student_id INTEGER NOT NULL,
  curso_id INTEGER,
  curso_name TEXT,
  started_at TEXT,
  ended_at TEXT,
  created_at TEXT,
  updated_at TEXT,
  device_id TEXT,
  sk TEXT
);

CREATE TABLE IF NOT EXISTS attendance (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  sync_id TEXT UNIQUE,
  student_id INTEGER NOT NULL,
  date TEXT NOT NULL,
  status TEXT NOT NULL,
  notes TEXT,
  created_at TEXT,
  updated_at TEXT,
  deleted_at TEXT,
  device_id TEXT,
  sk TEXT
);

CREATE TABLE IF NOT EXISTS payment (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  sync_id TEXT UNIQUE,
  student_id INTEGER NOT NULL,
  amount REAL NOT NULL,
  payment_date TEXT NOT NULL,
  payment_method TEXT,
  concept TEXT,
  status TEXT DEFAULT 'completed',
  month TEXT,
  year INTEGER,
  discount REAL DEFAULT 0,
  fine REAL DEFAULT 0,
  created_at TEXT,
  updated_at TEXT,
  deleted_at TEXT,
  whatsapp_pending INTEGER DEFAULT 0,
  device_id TEXT,
  sk TEXT
);

CREATE TABLE IF NOT EXISTS level (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  sync_id TEXT UNIQUE,
  name TEXT NOT NULL,
  display_order INTEGER NOT NULL,
  created_at TEXT,
  updated_at TEXT,
  deleted_at TEXT,
  device_id TEXT,
  sk TEXT
);

CREATE TABLE IF NOT EXISTS modality (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  sync_id TEXT UNIQUE,
  name TEXT NOT NULL,
  created_at TEXT,
  updated_at TEXT,
  device_id TEXT,
  sk TEXT
);

CREATE TABLE IF NOT EXISTS grade (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  sync_id TEXT UNIQUE,
  student_id INTEGER NOT NULL,
  level TEXT NOT NULL,
  score REAL NOT NULL,
  max_score REAL DEFAULT 100,
  grading_period TEXT,
  comment TEXT,
  created_at TEXT,
  updated_at TEXT,
  device_id TEXT,
  sk TEXT
);

CREATE TABLE IF NOT EXISTS payment_exemption (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  sync_id TEXT UNIQUE,
  student_id INTEGER NOT NULL,
  month TEXT NOT NULL,
  year INTEGER NOT NULL,
  created_at TEXT,
  updated_at TEXT,
  device_id TEXT,
  sk TEXT
);

CREATE TABLE IF NOT EXISTS users (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  sync_id TEXT UNIQUE,
  name TEXT NOT NULL,
  username TEXT,
  role TEXT NOT NULL,
  password_hash TEXT,
  permissions TEXT,
  teacher_id INTEGER,
  created_at TEXT,
  updated_at TEXT,
  deleted_at TEXT,
  device_id TEXT,
  sk TEXT
);

CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL,
  updated_at TEXT,
  device_id TEXT,
  sk TEXT
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_teacher_sync_id ON teacher(sync_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_curso_sync_id ON curso(sync_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_student_sync_id ON student(sync_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_attendance_sync_id ON attendance(sync_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_payment_sync_id ON payment(sync_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_level_sync_id ON level(sync_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_modality_sync_id ON modality(sync_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_grade_sync_id ON grade(sync_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_payment_exemption_sync_id ON payment_exemption(sync_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_sync_id ON users(sync_id);
