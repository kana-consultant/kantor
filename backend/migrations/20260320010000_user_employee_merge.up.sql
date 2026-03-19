-- Backfill: create employee records for users that don't have one yet.
INSERT INTO employees (user_id, full_name, email, position, date_joined, employment_status)
SELECT u.id, u.full_name, u.email, 'Belum Ditentukan', u.created_at::date, 'active'
FROM users u
WHERE NOT EXISTS (SELECT 1 FROM employees e WHERE e.user_id = u.id)
  AND NOT EXISTS (SELECT 1 FROM employees e WHERE e.email = u.email);
