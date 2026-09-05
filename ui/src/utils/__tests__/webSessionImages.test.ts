import { describe, expect, it } from 'vitest';

import {
  buildImageViewPreviewUrl,
  parseImageViewToolOutput,
  resolveImageViewDisplayName,
  stripImagePlaceholdersFromText,
} from '@/utils/webSessionImages';

describe('webSessionImages', () => {
  it('keeps text unchanged when there are no attachments', () => {
    const text = 'Please inspect [Image #1]';

    expect(stripImagePlaceholdersFromText(text, 0)).toBe(text);
  });

  it('removes inline image placeholders when attachments are present', () => {
    expect(stripImagePlaceholdersFromText('hello [Image #1]', 1)).toBe('hello');
    expect(stripImagePlaceholdersFromText('[Image #1] hello [Image #2]', 2)).toBe('hello');
  });

  it('preserves surrounding line structure while removing placeholders', () => {
    const text = 'first line\n[Image #1]\nsecond line';

    expect(stripImagePlaceholdersFromText(text, 1)).toBe('first line\nsecond line');
  });
});

describe('webSessionImages imageView helpers', () => {
  it('parses valid imageView JSON output', () => {
    expect(
      parseImageViewToolOutput(
        '{"id":"call_1","path":"/tmp/browser-manager/demo.png","type":"imageView"}'
      )
    ).toEqual({
      id: 'call_1',
      path: '/tmp/browser-manager/demo.png',
      type: 'imageView',
      cwd: undefined,
    });
  });

  it('rejects invalid or unrelated output', () => {
    expect(parseImageViewToolOutput('plain text')).toBeNull();
    expect(parseImageViewToolOutput('{"type":"other","path":"/tmp/demo.png"}')).toBeNull();
    expect(parseImageViewToolOutput('{"type":"imageView"}')).toBeNull();
  });

  it('builds encoded preview urls with optional cwd', () => {
    expect(
      buildImageViewPreviewUrl('/tmp/browser manager/demo.png', {
        cwd: '/workspace/project',
        baseUrl: 'http://localhost:3000',
      })
    ).toBe(
      'http://localhost:3000/api/v1/web-sessions/image-view?path=%2Ftmp%2Fbrowser+manager%2Fdemo.png&cwd=%2Fworkspace%2Fproject'
    );
  });

  it('derives a readable display name from the file path', () => {
    expect(resolveImageViewDisplayName('/tmp/browser-manager/demo.png')).toBe('demo.png');
    expect(resolveImageViewDisplayName('demo.png')).toBe('demo.png');
  });
});
