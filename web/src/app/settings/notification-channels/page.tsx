import { ReportShell } from "@/features/reports/report-shell";
import { fetchNotificationChannelProfiles } from "@/features/settings/notification-channels/api";
import { NotificationChannelSettingsManager } from "@/features/settings/notification-channels/notification-channel-settings-view";

export const dynamic = "force-dynamic";

export default async function NotificationChannelSettingsPage() {
  const result = await fetchNotificationChannelProfiles();
  const count = result.ok ? result.data.items.length : 0;

  return (
    <ReportShell current="channels">
      <section className="page-heading">
        <div>
          <h1>Notification Channels</h1>
          <p>Secret-backed operator notification targets.</p>
        </div>
        <div className="status-line">{count} channels</div>
      </section>

      <NotificationChannelSettingsManager result={result} />
    </ReportShell>
  );
}
