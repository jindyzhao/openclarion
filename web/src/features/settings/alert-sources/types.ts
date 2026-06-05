import type { components } from "@/lib/api/openapi";

export type AlertSourceKind = components["schemas"]["AlertSourceKind"];
export type AlertSourceAuthMode = components["schemas"]["AlertSourceAuthMode"];
export type AlertSourceLabels = components["schemas"]["AlertSourceLabels"];
export type AlertSourceProfile = components["schemas"]["AlertSourceProfile"];
export type AlertSourceProfileListResponse = components["schemas"]["AlertSourceProfileListResponse"];
export type AlertSourceProfileWriteRequest = components["schemas"]["AlertSourceProfileWriteRequest"];

export type AlertSourceFormState = {
  name: string;
  kind: AlertSourceKind;
  baseURL: string;
  authMode: AlertSourceAuthMode;
  secretRef: string;
  enabled: boolean;
  labelsText: string;
};
