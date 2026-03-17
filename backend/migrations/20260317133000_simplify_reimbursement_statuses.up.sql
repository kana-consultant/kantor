UPDATE reimbursements
SET status = 'submitted'
WHERE status = 'manager_review';

UPDATE reimbursements
SET status = 'approved'
WHERE status = 'finance_approval';

ALTER TABLE reimbursements
DROP CONSTRAINT IF EXISTS reimbursements_status_check;

ALTER TABLE reimbursements
ADD CONSTRAINT reimbursements_status_check
CHECK (status IN ('submitted', 'approved', 'rejected', 'paid'));
