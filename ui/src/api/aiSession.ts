import { http } from './http';
import type { ConversationResponse, ConversationWindowResponse } from '@/types/models';

type ItemResponse<T> = {
  item?: T;
};

type ConversationWindowOptions = {
  beforeCursor?: string;
  limit?: number;
};

function buildConversationWindowQuery(options?: ConversationWindowOptions) {
  const params = new URLSearchParams();
  if (options?.beforeCursor) {
    params.set('beforeCursor', options.beforeCursor);
  }
  if (typeof options?.limit === 'number' && Number.isFinite(options.limit)) {
    params.set('limit', String(Math.max(1, Math.trunc(options.limit))));
  }
  const query = params.toString();
  return query ? `?${query}` : '';
}

export const aiSessionApi = {
  async conversationByID(id: string): Promise<ConversationResponse> {
    const body =
      (await http.Get<ItemResponse<ConversationResponse>>(`/ai-sessions/${id}/conversation`, {
        cacheFor: 0,
      }).send()) ?? {};
    if (!body.item) {
      throw new Error('failed to load conversation');
    }
    return body.item;
  },

  async conversationBySessionID(sessionId: string): Promise<ConversationResponse> {
    const body =
      (await http.Get<ItemResponse<ConversationResponse>>(
        `/ai-sessions/by-session-id/${encodeURIComponent(sessionId)}/conversation`,
        {
          cacheFor: 0,
        }
      ).send()) ?? {};
    if (!body.item) {
      throw new Error('failed to load conversation');
    }
    return body.item;
  },

  async conversationWindowByID(
    id: string,
    options?: ConversationWindowOptions
  ): Promise<ConversationWindowResponse> {
    const body =
      (await http.Get<ItemResponse<ConversationWindowResponse>>(
        `/ai-sessions/${id}/conversation/window${buildConversationWindowQuery(options)}`,
        {
          cacheFor: 0,
        }
      ).send()) ?? {};
    if (!body.item) {
      throw new Error('failed to load conversation window');
    }
    return body.item;
  },

  async conversationWindowBySessionID(
    sessionId: string,
    options?: ConversationWindowOptions
  ): Promise<ConversationWindowResponse> {
    const body =
      (await http.Get<ItemResponse<ConversationWindowResponse>>(
        `/ai-sessions/by-session-id/${encodeURIComponent(sessionId)}/conversation/window${buildConversationWindowQuery(options)}`,
        {
          cacheFor: 0,
        }
      ).send()) ?? {};
    if (!body.item) {
      throw new Error('failed to load conversation window');
    }
    return body.item;
  },

  async refreshConversationWindow(id: string, limit?: number): Promise<ConversationWindowResponse> {
    const suffix =
      typeof limit === 'number' && Number.isFinite(limit)
        ? `?limit=${Math.max(1, Math.trunc(limit))}`
        : '';
    const body =
      (await http.Post<ItemResponse<ConversationWindowResponse>>(
        `/ai-sessions/${id}/refresh-window${suffix}`
      ).send()) ?? {};
    if (!body.item) {
      throw new Error('failed to refresh conversation window');
    }
    return body.item;
  },

  async toolResultByID(id: string, toolUseId: string): Promise<string | null> {
    const body =
      (await http.Get<ItemResponse<{ toolUseId: string; content: string }>>(
        `/ai-sessions/${id}/conversation/tool-results/${encodeURIComponent(toolUseId)}`,
        {
          cacheFor: 0,
        }
      ).send()) ?? {};
    return body.item?.content || null;
  },

  async toolResultBySessionID(sessionId: string, toolUseId: string): Promise<string | null> {
    const body =
      (await http.Get<ItemResponse<{ toolUseId: string; content: string }>>(
        `/ai-sessions/by-session-id/${encodeURIComponent(sessionId)}/conversation/tool-results/${encodeURIComponent(toolUseId)}`,
        {
          cacheFor: 0,
        }
      ).send()) ?? {};
    return body.item?.content || null;
  },
};
