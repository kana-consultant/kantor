-- Enforce referential integrity between employees.department and departments(name).
-- Until now the column was a free-form TEXT, so typos created ghost departments and
-- renaming a department in the departments table left employees rows pointing at a
-- stale string. The FK below catches both: rejects unknown names, and cascades on
-- rename. Deletes set the employee's department to NULL so we never lose an employee
-- record when a department is removed.

-- Backfill: every (tenant_id, department) pair that an employee currently references
-- must exist in departments before the FK can be added.
INSERT INTO departments (tenant_id, name, created_at)
SELECT DISTINCT e.tenant_id, e.department, NOW()
FROM employees e
WHERE e.department IS NOT NULL
  AND TRIM(e.department) <> ''
  AND NOT EXISTS (
    SELECT 1
    FROM departments d
    WHERE d.tenant_id = e.tenant_id
      AND d.name = e.department
  );

-- Normalize blank strings to NULL so the FK doesn't try to validate '' against a row
-- that will never exist.
UPDATE employees SET department = NULL WHERE department IS NOT NULL AND TRIM(department) = '';

ALTER TABLE employees
    ADD CONSTRAINT fk_employees_department
    FOREIGN KEY (tenant_id, department)
    REFERENCES departments (tenant_id, name)
    ON UPDATE CASCADE
    ON DELETE SET NULL;
