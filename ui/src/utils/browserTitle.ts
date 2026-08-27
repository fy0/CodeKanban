export interface BrowserTitleSummary {
  working: number;
  blocking: number;
  unreadCompleted: number;
}

export interface BrowserTabTitleOptions {
  summary: BrowserTitleSummary;
  appName: string;
  projectName?: string | null;
}

function normalizeProjectName(projectName?: string | null) {
  return typeof projectName === 'string' ? projectName.trim() : '';
}

function formatSummaryBadge(summary: BrowserTitleSummary) {
  return `[${summary.working}/${summary.blocking}/${summary.unreadCompleted}]`;
}

export function formatBrowserTabTitle({ summary, appName, projectName }: BrowserTabTitleOptions) {
  const normalizedProjectName = normalizeProjectName(projectName);
  const hasSummary = summary.working + summary.blocking + summary.unreadCompleted > 0;

  if (!hasSummary) {
    return normalizedProjectName ? `${normalizedProjectName} - ${appName}` : appName;
  }

  const summaryBadge = formatSummaryBadge(summary);
  if (!normalizedProjectName) {
    return `${summaryBadge} ${appName}`;
  }

  return `${summaryBadge} - ${normalizedProjectName} - ${appName}`;
}
