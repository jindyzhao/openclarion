export type DiagnosisServerError = {
  code: string;
  message: string;
};

export type DiagnosisServerErrorPresentation = {
  action: "review_evidence_tasks" | null;
  code: string;
  detail: string;
  kind: "confirmation_rejected" | "fatal" | "recoverable";
  type: "error" | "warning";
};

export function diagnosisServerErrorPresentation(
  error: DiagnosisServerError | null,
): DiagnosisServerErrorPresentation | null {
  if (error === null) {
    return null;
  }
  if (error.code === "confirm_rejected") {
    return {
      action: "review_evidence_tasks",
      code: error.code,
      detail: error.message,
      kind: "confirmation_rejected",
      type: "warning",
    };
  }
  if (isRecoverableDiagnosisServerError(error.code)) {
    return {
      action: null,
      code: error.code,
      detail: error.message,
      kind: "recoverable",
      type: "warning",
    };
  }
  return {
    action: null,
    code: error.code,
    detail: error.message,
    kind: "fatal",
    type: "error",
  };
}

function isRecoverableDiagnosisServerError(code: string): boolean {
  switch (code) {
    case "llm_timeout":
    case "llm_failed":
    case "turn_failed":
      return true;
    default:
      return false;
  }
}
