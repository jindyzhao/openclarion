"use client";

import { FormEvent, useMemo, useState } from "react";

import type { ApiResult } from "@/lib/api/client";

import { refreshAlertSourceProfiles, submitAlertSourceProfile } from "./client-api";
import {
  emptyAlertSourceForm,
  formStateToWriteRequest,
  formatDateTime,
  labelsToText,
  profileToFormState
} from "./format";
import type {
  AlertSourceAuthMode,
  AlertSourceFormState,
  AlertSourceKind,
  AlertSourceProfile,
  AlertSourceProfileListResponse
} from "./types";

type AlertSourceSettingsManagerProps = {
  result: ApiResult<AlertSourceProfileListResponse>;
};

type NoticeState = {
  kind: "info" | "error";
  message: string;
};

export function AlertSourceSettingsManager({ result }: AlertSourceSettingsManagerProps) {
  const [profiles, setProfiles] = useState<AlertSourceProfile[]>(result.ok ? result.data.items : []);
  const [form, setForm] = useState<AlertSourceFormState>(emptyAlertSourceForm());
  const [editingID, setEditingID] = useState<number | null>(null);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState<NoticeState | null>(
    result.ok ? null : { kind: "error", message: result.error.message }
  );

  const summary = useMemo(() => {
    const enabled = profiles.filter((profile) => profile.enabled).length;
    const prometheus = profiles.filter((profile) => profile.kind === "prometheus").length;
    const alertmanager = profiles.filter((profile) => profile.kind === "alertmanager").length;
    return { enabled, prometheus, alertmanager };
  }, [profiles]);

  async function handleRefresh() {
    setBusy(true);
    const refreshed = await refreshAlertSourceProfiles();
    setBusy(false);
    if (!refreshed.ok) {
      setNotice({ kind: "error", message: refreshed.error.message });
      return;
    }
    setProfiles(refreshed.data.items);
    setNotice({ kind: "info", message: "Profiles refreshed." });
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const parsed = formStateToWriteRequest(form);
    if (!parsed.ok) {
      setNotice({ kind: "error", message: parsed.message });
      return;
    }

    setBusy(true);
    const saved = await submitAlertSourceProfile(editingID, parsed.value);
    setBusy(false);
    if (!saved.ok) {
      setNotice({ kind: "error", message: saved.error.message });
      return;
    }

    setProfiles((current) => upsertProfile(current, saved.data));
    setForm(emptyAlertSourceForm());
    setEditingID(null);
    setNotice({ kind: "info", message: "Profile saved." });
  }

  function editProfile(profile: AlertSourceProfile) {
    setEditingID(profile.id);
    setForm(profileToFormState(profile));
    setNotice(null);
  }

  function resetForm() {
    setEditingID(null);
    setForm(emptyAlertSourceForm());
    setNotice(null);
  }

  return (
    <div className="stack">
      <section className="metric-grid settings-metrics" aria-label="Alert source metrics">
        <MetricCard label="Profiles" value={profiles.length} />
        <MetricCard label="Enabled" value={summary.enabled} />
        <MetricCard label="Prometheus" value={summary.prometheus} />
        <MetricCard label="Alertmanager" value={summary.alertmanager} />
      </section>

      {notice ? <Notice notice={notice} /> : null}

      <div className="settings-grid">
        <section className="panel">
          <div className="panel-header settings-panel-header">
            <h2>{editingID === null ? "New Alert Source" : `Edit Source #${editingID}`}</h2>
            {editingID === null ? null : (
              <button className="button-secondary button-compact" onClick={resetForm} type="button">
                New
              </button>
            )}
          </div>
          <div className="panel-body">
            <form className="settings-form" onSubmit={handleSubmit}>
              <label>
                <span>Name</span>
                <input
                  autoComplete="off"
                  name="name"
                  onChange={(event) => setForm((current) => ({ ...current, name: event.target.value }))}
                  value={form.name}
                />
              </label>

              <div className="settings-form-row">
                <label>
                  <span>Kind</span>
                  <select
                    name="kind"
                    onChange={(event) =>
                      setForm((current) => ({ ...current, kind: event.target.value as AlertSourceKind }))
                    }
                    value={form.kind}
                  >
                    <option value="prometheus">Prometheus</option>
                    <option value="alertmanager">Alertmanager</option>
                  </select>
                </label>
                <label>
                  <span>Auth</span>
                  <select
                    name="authMode"
                    onChange={(event) =>
                      setForm((current) => ({
                        ...current,
                        authMode: event.target.value as AlertSourceAuthMode,
                        secretRef: event.target.value === "none" ? "" : current.secretRef
                      }))
                    }
                    value={form.authMode}
                  >
                    <option value="none">None</option>
                    <option value="bearer">Bearer</option>
                  </select>
                </label>
              </div>

              <label>
                <span>Base URL</span>
                <input
                  autoComplete="url"
                  name="baseURL"
                  onChange={(event) => setForm((current) => ({ ...current, baseURL: event.target.value }))}
                  type="url"
                  value={form.baseURL}
                />
              </label>

              <label>
                <span>Secret reference</span>
                <input
                  autoComplete="off"
                  disabled={form.authMode === "none"}
                  name="secretRef"
                  onChange={(event) => setForm((current) => ({ ...current, secretRef: event.target.value }))}
                  value={form.secretRef}
                />
              </label>

              <label>
                <span>Labels</span>
                <textarea
                  name="labels"
                  onChange={(event) => setForm((current) => ({ ...current, labelsText: event.target.value }))}
                  placeholder={"env=prod\nowner=platform"}
                  rows={4}
                  value={form.labelsText}
                />
              </label>

              <label className="settings-checkbox">
                <input
                  checked={form.enabled}
                  name="enabled"
                  onChange={(event) => setForm((current) => ({ ...current, enabled: event.target.checked }))}
                  type="checkbox"
                />
                <span>Enabled</span>
              </label>

              <div className="button-row">
                <button className="button-primary" disabled={busy} type="submit">
                  Save Profile
                </button>
                <button className="button-secondary" disabled={busy} onClick={resetForm} type="button">
                  Reset
                </button>
              </div>
            </form>
          </div>
        </section>

        <section className="panel">
          <div className="panel-header settings-panel-header">
            <h2>Configured Sources</h2>
            <button className="button-secondary button-compact" disabled={busy} onClick={handleRefresh} type="button">
              Refresh
            </button>
          </div>
          <div className="panel-body panel-body-flush">
            {profiles.length === 0 ? (
              <div className="notice settings-empty">
                <strong>No alert sources configured.</strong>
              </div>
            ) : (
              <AlertSourceTable onEdit={editProfile} profiles={profiles} />
            )}
          </div>
        </section>
      </div>
    </div>
  );
}

function MetricCard({ label, value }: { label: string; value: number }) {
  return (
    <section className="metric-card">
      <div className="metric-value">{new Intl.NumberFormat("en-US").format(value)}</div>
      <div className="metric-label">{label}</div>
    </section>
  );
}

function Notice({ notice }: { notice: NoticeState }) {
  return (
    <div className={notice.kind === "error" ? "notice notice-error" : "notice"} role={notice.kind === "error" ? "alert" : "status"}>
      <strong>{notice.kind === "error" ? "Request failed" : "Settings"}</strong>
      <div>{notice.message}</div>
    </div>
  );
}

function AlertSourceTable({
  profiles,
  onEdit
}: {
  profiles: AlertSourceProfile[];
  onEdit: (profile: AlertSourceProfile) => void;
}) {
  return (
    <div className="table-wrap settings-table-wrap">
      <table className="settings-table">
        <thead>
          <tr>
            <th>Name</th>
            <th>Kind</th>
            <th>Endpoint</th>
            <th>Auth</th>
            <th>State</th>
            <th>Labels</th>
            <th>Updated</th>
            <th>Action</th>
          </tr>
        </thead>
        <tbody>
          {profiles.map((profile) => (
            <tr key={profile.id}>
              <td className="title-cell">
                {profile.name}
                <div className="muted">#{profile.id}</div>
              </td>
              <td>
                <span className={kindClass(profile.kind)}>{profile.kind}</span>
              </td>
              <td className="settings-url-cell">{profile.base_url}</td>
              <td>
                <span className={profile.auth_mode === "bearer" ? "pill pill-warning" : "pill pill-ok"}>
                  {profile.auth_mode}
                </span>
                {profile.secret_ref === "" ? null : <div className="muted settings-secret-ref">{profile.secret_ref}</div>}
              </td>
              <td>
                <span className={profile.enabled ? "pill pill-ok" : "pill pill-info"}>
                  {profile.enabled ? "enabled" : "disabled"}
                </span>
              </td>
              <td>
                <Labels labels={profile.labels} />
              </td>
              <td>{formatDateTime(profile.updated_at)}</td>
              <td>
                <button className="link-button" onClick={() => onEdit(profile)} type="button">
                  Edit
                </button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function Labels({ labels }: { labels: AlertSourceProfile["labels"] }) {
  const text = labelsToText(labels);
  if (text === "") {
    return <span className="muted">None</span>;
  }
  return (
    <div className="label-stack">
      {Object.entries(labels)
        .sort(([left], [right]) => left.localeCompare(right))
        .map(([key, value]) => (
          <span className="label-chip" key={key}>
            {key}={value}
          </span>
        ))}
    </div>
  );
}

function kindClass(kind: AlertSourceKind): string {
  switch (kind) {
    case "alertmanager":
      return "pill pill-critical";
    case "prometheus":
      return "pill pill-info";
  }
}

function upsertProfile(current: AlertSourceProfile[], saved: AlertSourceProfile): AlertSourceProfile[] {
  const next = [saved, ...current.filter((profile) => profile.id !== saved.id)];
  return next.sort((left, right) => Date.parse(right.updated_at) - Date.parse(left.updated_at));
}
