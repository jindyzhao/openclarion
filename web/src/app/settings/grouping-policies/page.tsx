import { getTranslations } from "next-intl/server";

import { fetchGroupingPolicies } from "@/features/settings/grouping-policies/api";
import {
  groupingPolicyLaunchIntentFromSearchParams,
  groupingPolicyLaunchIntentKey
} from "@/features/settings/grouping-policies/format";
import { GroupingPolicySettingsManager } from "@/features/settings/grouping-policies/grouping-policy-settings-view";
import { diagnosisBackendRequestOptionsFromIncomingHeaders } from "@/lib/api/server-authorization";

export const dynamic = "force-dynamic";

type GroupingPolicySettingsPageProps = {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
};

export default async function GroupingPolicySettingsPage({
  searchParams
}: GroupingPolicySettingsPageProps) {
  const t = await getTranslations("SettingsPages");
  const backendRequestOptions =
    await diagnosisBackendRequestOptionsFromIncomingHeaders();
  const result = await fetchGroupingPolicies(backendRequestOptions);
  const launchIntent = groupingPolicyLaunchIntentFromSearchParams(await searchParams);
  const count = result.ok ? result.data.items.length : 0;

  return (
    <>
      <section className="page-heading">
        <div>
          <h1>{t("groupingPoliciesTitle")}</h1>
          <p>{t("groupingPoliciesSubtitle")}</p>
        </div>
        <div className="status-line">{t("policies", { count })}</div>
      </section>

      <GroupingPolicySettingsManager
        key={groupingPolicyLaunchIntentKey(launchIntent)}
        launchIntent={launchIntent}
        result={result}
      />
    </>
  );
}
