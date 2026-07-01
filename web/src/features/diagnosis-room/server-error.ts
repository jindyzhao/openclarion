export type DiagnosisServerError = {
  code: string;
  message: string;
};

export type DiagnosisServerErrorDisplay = {
  actionLabel?: string;
  actionTitle?: string;
  description: string;
  message: string;
  type: "error" | "warning";
};

export function diagnosisServerErrorDisplay(
  error: DiagnosisServerError | null,
): DiagnosisServerErrorDisplay | null {
  if (error === null) {
    return null;
  }
  if (error.code === "confirm_rejected") {
    const recovery = confirmRejectedRecovery(error.message);
    return {
      actionLabel: "Review evidence tasks",
      actionTitle:
        "Jump to the review queue that contains the evidence or reassessment task blocking confirmation.",
      description: `${error.message} ${recovery}`,
      message: "Conclusion cannot be confirmed yet",
      type: "warning",
    };
  }
  if (isRecoverableDiagnosisServerError(error.code)) {
    return {
      description: `${error.message} Query the latest room state; if the turn did not progress, retry the operator message or provide narrower evidence.`,
      message: `Diagnosis request failed: ${error.code}`,
      type: "warning",
    };
  }
  return {
    description: error.message,
    message: `Diagnosis request failed: ${error.code}`,
    type: "error",
  };
}

function confirmRejectedRecovery(message: string): string {
  const normalized = message.trim().toLowerCase();
  if (normalized.includes("missing evidence")) {
    return "Open the review queue, add the requested operator evidence, ask AI to reassess submitted evidence when needed, then retry confirmation.";
  }
  if (
    normalized.includes("planned executable evidence") ||
    normalized.includes("collect planned")
  ) {
    return "Open the review queue, run the pending executable evidence collection, let AI reassess the collected evidence, then retry confirmation.";
  }
  if (normalized.includes("evidence collection")) {
    return "Open the review queue and recover the failed, skipped, or unsupported evidence collection before retrying confirmation.";
  }
  if (normalized.includes("reassessment")) {
    return "Open the review queue and ask AI to reassess the submitted evidence before retrying confirmation.";
  }
  return "Open the review queue, resolve the listed evidence or reassessment task, then retry confirmation.";
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
