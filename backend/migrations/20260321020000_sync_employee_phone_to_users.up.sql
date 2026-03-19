-- Sync phone numbers from employees to users where users.phone is NULL
-- This fixes the issue where phones set via HRIS weren't visible to WA broadcast
UPDATE users
SET phone = e.phone, updated_at = NOW()
FROM employees e
WHERE e.user_id = users.id
  AND (users.phone IS NULL OR users.phone = '')
  AND e.phone IS NOT NULL
  AND e.phone != '';
