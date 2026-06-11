import type { StationStats, Track } from '../types'

const BASE = import.meta.env.VITE_API_BASE || ''

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...init?.headers,
    },
  })
  if (!res.ok) {
    const body = await res.text()
    throw new Error(`${res.status}: ${body}`)
  }
  return res.json()
}

export const api = {
  /** GET /health */
  health(): Promise<string> {
    return fetch(`${BASE}/health`).then((r) => r.text())
  },

  /** GET /api/stats */
  stats(): Promise<StationStats> {
    return request<StationStats>('/api/stats')
  },

  /** GET /api/queue */
  queue(): Promise<{ queue: Track[] }> {
    return request<{ queue: Track[] }>('/api/queue')
  },

  /** POST /api/queue */
  queueTrack(title: string, artist?: string, source?: string, cid?: string): Promise<{ queue: Track[] }> {
    return request<{ queue: Track[] }>('/api/queue', {
      method: 'POST',
      body: JSON.stringify({ title, artist, source, cid }),
    })
  },

  /** POST /api/chat */
  sendChat(name: string, text: string): Promise<{ status: string }> {
    return request<{ status: string }>('/api/chat', {
      method: 'POST',
      body: JSON.stringify({ name, text }),
    })
  },

  /** POST /api/tip */
  tipTrack(cid: string, lamports: number): Promise<{ status: string }> {
    return request<{ status: string }>('/api/tip', {
      method: 'POST',
      body: JSON.stringify({ cid, lamports }),
    })
  },
}