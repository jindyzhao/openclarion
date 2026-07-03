const diagnosisReviewReturnParam = "diagnosis_review";
const reportDeliveryProofHash = "#report-delivery-proof";
const reportDiagnosisReadinessHash = "#diagnosis-readiness";

export type DiagnosisReviewReturnState = "confirmed" | "none" | "reviewed";

export function diagnosisReportReturnHref(
  backHref: string | undefined,
  state: Exclude<DiagnosisReviewReturnState, "none"> = "reviewed",
) {
  if (backHref === undefined || backHref.trim() === "") {
    return undefined;
  }
  const url = new URL(backHref, "http://openclarion.local");
  url.searchParams.set(
    diagnosisReviewReturnParam,
    state === "confirmed" ? "confirmed" : "1",
  );
  const targetHash =
    state === "confirmed" ? reportDeliveryProofHash : reportDiagnosisReadinessHash;
  if (
    url.hash === "" ||
    (state === "confirmed" && url.hash === reportDiagnosisReadinessHash)
  ) {
    url.hash = targetHash;
  }
  return `${url.pathname}${url.search}${url.hash}`;
}

export function diagnosisReviewReturnState(
  value: string | string[] | undefined,
): DiagnosisReviewReturnState {
  const raw = Array.isArray(value) ? value[0] : value;
  switch (raw) {
    case "confirmed":
      return "confirmed";
    case "1":
    case "true":
    case "reviewed":
      return "reviewed";
    default:
      return "none";
  }
}

export function diagnosisReviewReturnSearchParam(
  value: string | string[] | undefined,
) {
  return diagnosisReviewReturnState(value) !== "none";
}
