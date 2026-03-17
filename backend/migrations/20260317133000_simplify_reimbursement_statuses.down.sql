ALTER TABLE reimbursements
DROP CONSTRAINT IF EXISTS reimbursements_status_check;

ALTER TABLE reimbursements
ADD CONSTRAINT reimbursements_status_check
CHECK (status IN ('submitted', 'manager_review', 'finance_approval', 'approved', 'rejected', 'paid'));
