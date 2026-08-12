import type { ProjectAgentTrustStatus } from '@/types/models';

export function isPiProjectTrusted(
  status: ProjectAgentTrustStatus | null | undefined,
  projectId: string
) {
  return Boolean(
    status?.agent === 'pi' &&
      status.trusted === true &&
      status.projectId.trim() === projectId.trim() &&
      projectId.trim()
  );
}

export function piProjectTrustNeedsRenewal(status: ProjectAgentTrustStatus | null | undefined) {
  return Boolean(status && !status.trusted && status.trustedPath?.trim());
}
