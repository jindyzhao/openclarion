import Link from "next/link";
import type { ReactNode } from "react";

type ReportShellProps = {
  children: ReactNode;
  current: "dashboard" | "reports" | "diagnosis";
};

export function ReportShell({ children, current }: ReportShellProps) {
  return (
    <div className="app-shell">
      <header className="topbar">
        <Link className="brand" href="/dashboard">
          OpenClarion
        </Link>
        <nav aria-label="Primary" className="nav">
          <Link aria-current={current === "dashboard" ? "page" : undefined} href="/dashboard">
            Dashboard
          </Link>
          <Link aria-current={current === "reports" ? "page" : undefined} href="/reports">
            Reports
          </Link>
          <Link aria-current={current === "diagnosis" ? "page" : undefined} href="/diagnosis-room">
            Diagnosis
          </Link>
        </nav>
      </header>
      <main className="content">{children}</main>
    </div>
  );
}
