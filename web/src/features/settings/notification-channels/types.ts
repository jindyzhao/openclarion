import type { components } from "@/lib/api/openapi";

export type NotificationChannelProfile = components["schemas"]["NotificationChannelProfile"];
export type NotificationChannelProfileListResponse = components["schemas"]["NotificationChannelProfileListResponse"];
export type NotificationChannelTestResult = components["schemas"]["NotificationChannelTestResult"];
export type NotificationChannelTestContentKind = NonNullable<NotificationChannelTestResult["content_kind"]>;
export type NotificationChannelProfileWriteRequest = components["schemas"]["NotificationChannelProfileWriteRequest"];
export type NotificationDeliveryScope = components["schemas"]["NotificationDeliveryScope"];

export type NotificationChannelFormState = {
  name: string;
  kind: components["schemas"]["NotificationChannelKind"];
  secretRef: string;
  deliveryScopes: NotificationDeliveryScope[];
  enabled: boolean;
  labelsText: string;
};
