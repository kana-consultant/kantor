DROP INDEX IF EXISTS idx_project_members_user_id;
DROP INDEX IF EXISTS idx_projects_created_by;
DROP INDEX IF EXISTS idx_projects_deadline;
DROP INDEX IF EXISTS idx_projects_priority;
DROP INDEX IF EXISTS idx_projects_status;

DROP TABLE IF EXISTS project_members;
DROP TABLE IF EXISTS projects;
