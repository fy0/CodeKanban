import type { CodexSkillSource, CodexSkillSummary } from '@/types/models';

export const CODEX_SKILL_TOKEN_PATTERN = /\$[a-z0-9][a-z0-9._-]*/gi;

type CodexSkillMatch = CodexSkillSummary & {
  score: number;
};

function normalize(value: string) {
  return String(value || '')
    .trim()
    .toLowerCase();
}

function splitSearchTokens(value: string) {
  return normalize(value)
    .split(/[^a-z0-9]+/i)
    .filter(Boolean);
}

function scoreCandidateText(candidate: string, query: string) {
  if (!query) {
    return null;
  }

  if (candidate === query) {
    return 0;
  }
  if (candidate.startsWith(query)) {
    return 10;
  }
  if (candidate.includes(query)) {
    return 40;
  }

  return null;
}

function scoreCandidateGroup(value: string, query: string, baseScore: number) {
  const normalizedValue = normalize(value);
  const scores: number[] = [];

  const directScore = scoreCandidateText(normalizedValue, query);
  if (directScore != null) {
    scores.push(baseScore + directScore);
  }

  splitSearchTokens(value).forEach(token => {
    const tokenScore = scoreCandidateText(token, query);
    if (tokenScore != null) {
      scores.push(baseScore + tokenScore + 5);
    }
  });

  if (scores.length === 0) {
    return null;
  }

  return Math.min(...scores);
}

export function buildCodexSkillToken(name: string) {
  return `$${String(name || '').trim()}`;
}

export function codexSkillSourceOrder(source: CodexSkillSource) {
  switch (source) {
    case 'user':
      return 0;
    case 'bundled':
      return 1;
    default:
      return 2;
  }
}

export function filterCodexSkills(skills: CodexSkillSummary[], query: string) {
  const normalizedQuery = normalize(query);

  const matches: CodexSkillMatch[] = skills
    .map(skill => {
      let score = 300;
      if (normalizedQuery) {
        const scoreCandidates = [
          scoreCandidateGroup(skill.name, normalizedQuery, 0),
          scoreCandidateGroup(skill.displayName, normalizedQuery, 20),
          scoreCandidateGroup(skill.description, normalizedQuery, 120),
        ].filter((value): value is number => value != null);

        if (scoreCandidates.length === 0) {
          return null;
        }
        score = Math.min(...scoreCandidates);
      }

      return {
        ...skill,
        score,
      };
    })
    .filter((skill): skill is CodexSkillMatch => Boolean(skill));

  matches.sort((left, right) => {
    if (left.score !== right.score) {
      return left.score - right.score;
    }

    const leftSource = codexSkillSourceOrder(left.source);
    const rightSource = codexSkillSourceOrder(right.source);
    if (leftSource !== rightSource) {
      return leftSource - rightSource;
    }

    const leftName = normalize(left.displayName || left.name);
    const rightName = normalize(right.displayName || right.name);
    if (leftName !== rightName) {
      return leftName.localeCompare(rightName);
    }

    return normalize(left.name).localeCompare(normalize(right.name));
  });

  return matches.map(({ score: _score, ...skill }) => skill);
}

export function replaceTextSelection(text: string, start: number, end: number, inserted: string) {
  const normalizedText = String(text ?? '');
  const safeStart = Math.max(0, Math.min(start, normalizedText.length));
  const safeEnd = Math.max(safeStart, Math.min(end, normalizedText.length));
  const safeInsert = String(inserted ?? '');

  return {
    text: `${normalizedText.slice(0, safeStart)}${safeInsert}${normalizedText.slice(safeEnd)}`,
    cursor: safeStart + safeInsert.length,
  };
}

export function insertCodexSkillTokenAtCursor(
  text: string,
  selectionStart: number,
  selectionEnd: number,
  skillName: string
) {
  const token = buildCodexSkillToken(skillName);
  const normalizedText = String(text ?? '');
  const safeStart = Math.max(0, Math.min(selectionStart, normalizedText.length));
  const safeEnd = Math.max(safeStart, Math.min(selectionEnd, normalizedText.length));
  const prefix = normalizedText.slice(0, safeStart);
  const suffix = normalizedText.slice(safeEnd);
  const needsLeadingSpace =
    prefix.length > 0 && !/[\s([{'"`]/.test(prefix[prefix.length - 1] || '');
  const needsTrailingSpace = suffix.length > 0 && !/[\s)\]}.,!?;:'"`]/.test(suffix[0] || '');
  const insertedText = `${needsLeadingSpace ? ' ' : ''}${token}${needsTrailingSpace ? ' ' : ''}`;

  return {
    text: `${prefix}${insertedText}${suffix}`,
    cursor: prefix.length + insertedText.length,
  };
}
