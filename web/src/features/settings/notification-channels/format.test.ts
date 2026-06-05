import { describe, expect, it } from "vitest";

import {
  channelToFormState,
  emptyNotificationChannelForm,
  formStateToWriteRequest
} from "./format";
import type { NotificationChannelProfile } from "./types";

describe("notification channel form formatting", () => {
  it("builds write requests with secret references only", () => {
    const parsed = formStateToWriteRequest({
      ...emptyNotificationChannelForm(),
      name: " Operations webhook ",
      secretRef: " secret/example/ops-webhook ",
      deliveryScopes: ["report", "diagnosis_close", "report"],
      enabled: true,
      labelsText: "team=ops\nenv=test"
    });

    expect(parsed).toEqual({
      ok: true,
      value: {
        name: "Operations webhook",
        kind: "webhook",
        secret_ref: "secret/example/ops-webhook",
        delivery_scopes: ["diagnosis_close", "report"],
        enabled: true,
        labels: {
          env: "test",
          team: "ops"
        }
      }
    });
  });

  it("rejects missing secret references", () => {
    const parsed = formStateToWriteRequest({
      ...emptyNotificationChannelForm(),
      name: "Operations webhook",
      secretRef: ""
    });

    expect(parsed).toEqual({
      ok: false,
      message: "Secret reference is required."
    });
  });

  it("rejects malformed labels", () => {
    const parsed = formStateToWriteRequest({
      ...emptyNotificationChannelForm(),
      name: "Operations webhook",
      secretRef: "secret/example/ops-webhook",
      labelsText: "owner"
    });

    expect(parsed).toEqual({
      ok: false,
      message: "Labels must use key=value lines."
    });
  });

  it("maps persisted channels back to edit form state", () => {
    const channel: NotificationChannelProfile = {
      id: 7,
      name: "Operations webhook",
      kind: "webhook",
      secret_ref: "secret/example/ops-webhook",
      delivery_scopes: ["report", "diagnosis_close"],
      enabled: false,
      labels: {
        team: "ops"
      },
      created_at: "2026-06-05T09:00:00Z",
      updated_at: "2026-06-05T09:00:00Z"
    };

    expect(channelToFormState(channel)).toEqual({
      name: "Operations webhook",
      kind: "webhook",
      secretRef: "secret/example/ops-webhook",
      deliveryScopes: ["report", "diagnosis_close"],
      enabled: false,
      labelsText: "team=ops"
    });
  });
});
