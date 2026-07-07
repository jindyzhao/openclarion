-- Modify "report_workflow_schedules" table
ALTER TABLE "report_workflow_schedules" ADD COLUMN "cadence" character varying NOT NULL DEFAULT 'interval', ADD COLUMN "calendar_hour" bigint NOT NULL DEFAULT 0, ADD COLUMN "calendar_minute" bigint NOT NULL DEFAULT 0, ADD COLUMN "calendar_day_of_week" bigint NOT NULL DEFAULT 0, ADD COLUMN "calendar_day_of_month" bigint NOT NULL DEFAULT 0;
