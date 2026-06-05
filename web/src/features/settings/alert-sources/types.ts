import type { components } from "@/lib/api/openapi";

type AlertSourceKind = components["schemas"]["AlertSourceKind"];
type AlertSourceAuthMode = components["schemas"]["AlertSourceAuthMode"];
export type AlertSourceLabels = components["schemas"]["AlertSourceLabels"];
export type AlertSourceConnectionTestResult = components["schemas"]["AlertSourceConnectionTestResult"];
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
