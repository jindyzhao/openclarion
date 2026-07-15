import { getTranslations } from "next-intl/server";

import { fetchNotificationChannelProfiles } from "@/features/settings/notification-channels/api";
import {
  notificationChannelEditIDFromSearchParams,
  notificationChannelLaunchIntentFromSearchParams,
  notificationChannelLaunchIntentKey,
  notificationChannelWorkflowReturnFromSearchParams
} from "@/features/settings/notification-channels/format";
import { NotificationChannelSettingsManager } from "@/features/settings/notification-channels/notification-channel-settings-view";
import { diagnosisBackendRequestOptionsFromIncomingHeaders } from "@/lib/api/server-authorization";

export const dynamic = "force-dynamic";

type NotificationChannelSettingsPageProps = {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
};

export default async function NotificationChannelSettingsPage({
  searchParams
}: NotificationChannelSettingsPageProps) {
  const t = await getTranslations("SettingsPages");
  const backendRequestOptions =
    await diagnosisBackendRequestOptionsFromIncomingHeaders();
  const result = await fetchNotificationChannelProfiles(backendRequestOptions);
  const resolvedSearchParams = await searchParams;
  const launchIntent = notificationChannelLaunchIntentFromSearchParams(resolvedSearchParams);
  const launchEditChannelID = notificationChannelEditIDFromSearchParams(resolvedSearchParams);
  const workflowReturn = notificationChannelWorkflowReturnFromSearchParams(resolvedSearchParams);
  const count = result.ok ? result.data.items.length : 0;

  return (
    <>
      <section className="page-heading">
        <div>
          <h1>{t("notificationChannelsTitle")}</h1>
          <p>{t("notificationChannelsSubtitle")}</p>
        </div>
        <div className="status-line">{t("channels", { count })}</div>
      </section>

      <NotificationChannelSettingsManager
        key={`${notificationChannelLaunchIntentKey(launchIntent)}:${launchEditChannelID ?? "none"}`}
        launchEditChannelID={launchEditChannelID}
        launchIntent={launchIntent}
        result={result}
        workflowReturn={workflowReturn}
      />
    </>
  );
}
