import { describe, expect, it } from 'vitest';

import {
  buildWebSessionComposerPastePlan,
  exposeOfficeImageFallbacks,
  getImageFilesFromTransfer,
  mergeClipboardImageFiles,
  readClipboardImageFiles,
  renderWebSessionComposerPastePlan,
} from '@/components/web-session/webSessionComposerPaste';

const FAILURE_MARKER = '[Image upload failed]';
const PNG_DATA_URL = 'data:image/png;base64,aGVsbG8=';

function text(value: string) {
  return {
    childNodes: [],
    nodeName: '#text',
    nodeType: 3,
    textContent: value,
  } as unknown as Node;
}

function element(name: string, children: Node[] = [], attributes: Record<string, string> = {}) {
  const node = {
    childNodes: children,
    getAttribute: (attribute: string) => attributes[attribute] ?? null,
    localName: name,
    nodeName: name.toUpperCase(),
    nodeType: 1,
    querySelectorAll: () => {
      const descendants: Node[] = [];
      const visit = (child: Node) => {
        if (child.nodeType !== 1) {
          return;
        }
        descendants.push(child);
        Array.from(child.childNodes).forEach(visit);
      };
      children.forEach(visit);
      return descendants;
    },
    textContent: '',
  };
  return node as unknown as Element;
}

function documentWith(...children: Node[]) {
  return { body: element('body', children) } as unknown as Document;
}

function paragraph(...children: Node[]) {
  return element('p', children);
}

function image(source: string, tagName = 'img') {
  return element(tagName, [], { src: source });
}

function buildPlan(options: {
  document?: Document;
  html?: string;
  imageFiles?: File[];
  plainText?: string;
}) {
  return buildWebSessionComposerPastePlan({
    failureMarker: FAILURE_MARKER,
    html: options.html || '',
    imageFiles: options.imageFiles,
    parseHtml: options.document ? () => options.document! : undefined,
    plainText: options.plainText || '',
  });
}

describe('webSessionComposerPaste', () => {
  it('leaves ordinary text paste to the browser', () => {
    expect(buildPlan({ html: '<p>plain text</p>', plainText: 'plain text' })).toBeNull();
  });

  it('pairs a Word file image with its HTML position', () => {
    const imageFile = new File(['word-image'], 'image001.png', { type: 'image/png' });
    const plan = buildPlan({
      document: documentWith(
        paragraph(text('Before the image')),
        paragraph(image('file:///C:/Users/test/AppData/Local/Temp/msohtmlclip1/image001.png')),
        paragraph(text('After the image'))
      ),
      html: '<word-html>',
      imageFiles: [imageFile],
      plainText: 'Before the image\nAfter the image',
    });

    expect(plan?.images).toEqual([imageFile]);
    expect(renderWebSessionComposerPastePlan(plan!, ['[Image #1]'])).toBe(
      'Before the image\n\n[Image #1]\n\nAfter the image'
    );
  });

  it('extracts the non-VML fallback image from Word conditional HTML', () => {
    const officeHtml = [
      '<p>Word image</p>',
      '<!--[if !vml]><!--><img src="',
      PNG_DATA_URL,
      '"><!--<![endif]-->',
    ].join('');
    expect(exposeOfficeImageFallbacks(officeHtml)).toContain(`<img src="${PNG_DATA_URL}">`);

    const plan = buildPlan({
      document: documentWith(paragraph(text('Word image')), image(PNG_DATA_URL)),
      html: officeHtml,
    });

    expect(plan?.images).toHaveLength(1);
    expect(plan?.images[0]?.type).toBe('image/png');
    expect(plan?.images[0]?.name).toBe('pasted-image-1.png');
    expect(plan?.images[0]?.size).toBe(5);
    expect(renderWebSessionComposerPastePlan(plan!, ['[Image #1]'])).toBe('Word image\n[Image #1]');
  });

  it('recognizes Office VML image nodes when no HTML fallback is present', () => {
    const plan = buildPlan({
      document: documentWith(paragraph(text('VML image'), image(PNG_DATA_URL, 'v:imagedata'))),
      html: '<!--[if gte vml 1]><v:imagedata src="data:image/png;base64,aGVsbG8="><![endif]-->',
    });

    expect(plan?.images).toHaveLength(1);
    expect(renderWebSessionComposerPastePlan(plan!, ['[Image #1]'])).toBe('VML image [Image #1]');
  });

  it('decodes URL-encoded data images', async () => {
    const plan = buildPlan({
      document: documentWith(
        paragraph(text('Icon '), image('data:image/svg+xml,%3Csvg%3Eok%3C%2Fsvg%3E'))
      ),
      html: '<word-html>',
    });

    expect(plan?.images[0]?.type).toBe('image/svg+xml');
    expect(await plan?.images[0]?.text()).toBe('<svg>ok</svg>');
    expect(renderWebSessionComposerPastePlan(plan!, ['[Image #1]'])).toBe('Icon [Image #1]');
  });

  it('keeps remote images as URL text without creating upload files', () => {
    const plan = buildPlan({
      document: documentWith(
        paragraph(text('Inspect '), image('https://example.com/report.png'), text(' please'))
      ),
      html: '<word-html>',
    });

    expect(plan?.images).toHaveLength(0);
    expect(plan?.remoteImages).toEqual(['https://example.com/report.png']);
    expect(renderWebSessionComposerPastePlan(plan!)).toBe(
      'Inspect https://example.com/report.png please'
    );
    expect(renderWebSessionComposerPastePlan(plan!, [], ['[Image #1]'])).toBe(
      'Inspect [Image #1] please'
    );
  });

  it('keeps a failure marker for inaccessible Word temporary images', () => {
    const plan = buildPlan({
      document: documentWith(
        paragraph(
          text('Inspect '),
          image('file:///C:/Temp/msohtmlclip1/image001.png'),
          text(' please')
        )
      ),
      html: '<word-html>',
    });

    expect(plan?.images).toHaveLength(0);
    expect(plan?.unavailableImageCount).toBe(1);
    expect(plan?.unavailableImages).toEqual([
      'file:///C:/Temp/msohtmlclip1/image001.png',
    ]);
    expect(renderWebSessionComposerPastePlan(plan!)).toBe(`Inspect ${FAILURE_MARKER} please`);
    expect(renderWebSessionComposerPastePlan(plan!, [], [], ['[Image #1]'])).toBe(
      'Inspect [Image #1] please'
    );
  });

  it('keeps successful images and failure markers in their original order', () => {
    const plan = buildPlan({
      document: documentWith(
        paragraph(text('First '), image(PNG_DATA_URL), text(' then '), image(PNG_DATA_URL))
      ),
      html: '<word-html>',
    });

    expect(renderWebSessionComposerPastePlan(plan!, ['[Image #2]', FAILURE_MARKER])).toBe(
      `First [Image #2] then ${FAILURE_MARKER}`
    );
  });

  it('appends direct images when HTML has no image position', () => {
    const image = new File(['image'], 'clipboard-image.png', { type: 'image/png' });
    const plan = buildPlan({ imageFiles: [image], plainText: 'Copied text' });

    expect(renderWebSessionComposerPastePlan(plan!, ['[Image #1]'])).toBe(
      'Copied text\n[Image #1]'
    );
  });

  it('deduplicates image files exposed through both items and files', () => {
    const image = new File(['image'], 'clipboard-image.png', { type: 'image/png' });
    const transfer = {
      files: [image],
      items: [
        {
          getAsFile: () => image,
          kind: 'file',
          type: 'image/png',
        },
      ],
    } as unknown as DataTransfer;

    expect(getImageFilesFromTransfer(transfer)).toEqual([image]);
  });

  it('reads image blobs from the async Clipboard API', async () => {
    const files = await readClipboardImageFiles(async () => [
      {
        getType: async () => new Blob(['clipboard-image'], { type: 'image/png' }),
        types: ['text/plain', 'image/png'],
      },
    ]);

    expect(files).toHaveLength(1);
    expect(files[0]?.name).toBe('pasted-image-1.png');
    expect(files[0]?.type).toBe('image/png');
  });

  it('silently falls back when async clipboard access is denied', async () => {
    expect(
      await readClipboardImageFiles(async () => {
        throw new DOMException('Permission denied', 'NotAllowedError');
      })
    ).toEqual([]);
  });

  it('merges and deduplicates paste-event and async clipboard images', () => {
    const image = new File(['same-image'], 'image.png', { type: 'image/png' });

    expect(mergeClipboardImageFiles([image], [image])).toEqual([image]);
  });
});
