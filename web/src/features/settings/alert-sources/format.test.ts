import { describe, expect, it } from "vitest";

import {
  emptyAlertSourceForm,
  formStateToWriteRequest,
  labelsToText,
  parseLabelsText
} from "./format";

describe("alert source settings formatting", () => {
  it("parses labels from key-value lines", () => {
    const result = parseLabelsText("owner=platform\nenv=prod\n");

    expect(result).toEqual({
      ok: true,
      value: {
        owner: "platform",
        env: "prod"
      }
    });
  });

  it("rejects malformed and duplicate labels", () => {
    expect(parseLabelsText("owner").ok).toBe(false);
    expect(parseLabelsText("owner=platform\nowner=sre").ok).toBe(false);
  });

  it("formats labels in stable key order", () => {
    expect(labelsToText({ owner: "platform", env: "prod" })).toBe("env=prod\nowner=platform");
  });

  it("builds a profile write request without secret values", () => {
    const form = {
      ...emptyAlertSourceForm(),
      name: "Primary Prometheus",
      baseURL: "https://prometheus.example.test",
      authMode: "bearer" as const,
      secretRef: "secret/openclarion/prometheus-bearer",
      enabled: true,
      labelsText: "env=prod"
    };

    expect(formStateToWriteRequest(form)).toEqual({
      ok: true,
      value: {
        name: "Primary Prometheus",
        kind: "prometheus",
        base_url: "https://prometheus.example.test",
        auth_mode: "bearer",
        secret_ref: "secret/openclarion/prometheus-bearer",
        enabled: true,
        labels: { env: "prod" }
      }
    });
  });

  it("enforces auth and URL boundaries before submit", () => {
    expect(
      formStateToWriteRequest({
        ...emptyAlertSourceForm(),
        name: "Bad URL",
        baseURL: "https://user@example.test"
      }).ok
    ).toBe(false);
    expect(
      formStateToWriteRequest({
        ...emptyAlertSourceForm(),
        name: "Bearer without secret",
        baseURL: "https://prometheus.example.test",
        authMode: "bearer"
      }).ok
    ).toBe(false);
    expect(
      formStateToWriteRequest({
        ...emptyAlertSourceForm(),
        name: "None with secret",
        baseURL: "https://prometheus.example.test",
        secretRef: "secret/openclarion/prometheus-bearer"
      }).ok
    ).toBe(false);
  });
});
