-- Modify "report_workflow_policies" table
ALTER TABLE "report_workflow_policies" ADD COLUMN "report_notification_channel_profile_id" bigint NULL;
-- Create index "reportworkflowpolicy_report_notification_channel_profile_id" to table: "report_workflow_policies"
CREATE INDEX "reportworkflowpolicy_report_notification_channel_profile_id" ON "report_workflow_policies" ("report_notification_channel_profile_id");
