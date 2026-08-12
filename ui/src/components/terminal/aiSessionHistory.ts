import type { AISessionType, ProjectAISessions } from '@/types/models';

type AISessionAvailability = Pick<ProjectAISessions, 'hasClaudeCode' | 'hasCodex' | 'hasPi'>;
type AISessionScanState = Pick<
  ProjectAISessions,
  'claudeScanPhase' | 'codexScanPhase' | 'piScanPhase'
>;

export function resolvePreferredAISessionType(data: AISessionAvailability): AISessionType {
  if (data.hasClaudeCode) return 'claude_code';
  if (data.hasCodex) return 'codex';
  if (data.hasPi) return 'pi';
  return 'claude_code';
}

export function isProjectAISessionScanning(data: AISessionScanState): boolean {
  return (
    (data.claudeScanPhase !== undefined && data.claudeScanPhase !== 'complete') ||
    (data.codexScanPhase !== undefined && data.codexScanPhase !== 'complete') ||
    (data.piScanPhase !== undefined && data.piScanPhase !== 'complete')
  );
}
