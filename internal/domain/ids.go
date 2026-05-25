package domain

// Typed identifiers for the five M1 entities. Using distinct named
// types prevents accidental cross-entity ID confusion at compile time
// (e.g. passing an AlertEventID where a DiagnosisTaskID is expected)
// and documents the FK target right at the field type.
//
// The underlying type is int64 to match the Ent default `bigserial`
// primary key (see schema-catalog.md). A zero value (0) means
// "not yet persisted"; repository Insert paths fill it in.

// AlertEventID is the surrogate identifier of an AlertEvent row.
type AlertEventID int64

// AlertGroupID is the surrogate identifier of an AlertGroup row.
type AlertGroupID int64

// EvidenceSnapshotID is the surrogate identifier of an
// EvidenceSnapshot row.
type EvidenceSnapshotID int64

// DiagnosisTaskID is the surrogate identifier of a DiagnosisTask row.
type DiagnosisTaskID int64

// DiagnosisTaskEventID is the surrogate identifier of a
// DiagnosisTaskEvent row.
type DiagnosisTaskEventID int64
