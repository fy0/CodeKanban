import { describe, expect, it } from 'vitest';

import {
  filterWebSessionQuickInputItems,
  paginateWebSessionQuickInputItems,
  WEB_SESSION_QUICK_INPUT_PAGE_SIZE,
} from '@/components/web-session/webSessionQuickInput';

describe('web session quick input list helpers', () => {
  it('filters the complete list case-insensitively before pagination', () => {
    const items = Array.from({ length: 30 }, (_, index) =>
      index % 2 === 0 ? `Build step ${index}` : `Review step ${index}`
    );

    const filtered = filterWebSessionQuickInputItems(items, 'BUILD');

    expect(filtered).toHaveLength(15);
    expect(paginateWebSessionQuickInputItems(filtered, 1)).toHaveLength(
      WEB_SESSION_QUICK_INPUT_PAGE_SIZE
    );
    expect(paginateWebSessionQuickInputItems(filtered, 3)).toEqual([
      'Build step 24',
      'Build step 26',
      'Build step 28',
    ]);
  });

  it('returns six items per page and clamps invalid page values', () => {
    const items = Array.from({ length: 13 }, (_, index) => `item ${index + 1}`);

    expect(paginateWebSessionQuickInputItems(items, 1)).toEqual([
      'item 1',
      'item 2',
      'item 3',
      'item 4',
      'item 5',
      'item 6',
    ]);
    expect(paginateWebSessionQuickInputItems(items, 3)).toEqual(['item 13']);
    expect(paginateWebSessionQuickInputItems(items, 0)).toEqual([
      'item 1',
      'item 2',
      'item 3',
      'item 4',
      'item 5',
      'item 6',
    ]);
  });
});
