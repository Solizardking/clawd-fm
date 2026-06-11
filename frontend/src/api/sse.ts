import type { StationEvent } from '../types'

const BASE = import.meta.env.VITE_API_BASE || ''

/**
 * Connects to the SSE /events endpoint and calls `onEvent` for each
 * station event received. Returns an AbortController to stop the stream.
 */
export function connectSSE(onEvent: (ev: StationEvent) => void): AbortController {
  const ctrl = new AbortController()

  void (async () => {
    try {
      const res = await fetch(`${BASE}/events`, {
        signal: ctrl.signal,
        headers: { Accept: 'text/event-stream' },
      })

      if (!res.ok || !res.body) {
        console.warn('[sse] connection failed, will retry in 5s')
        scheduleRetry(onEvent, ctrl)
        return
      }

      const reader = res.body.getReader()
      const decoder = new TextDecoder()
      let buf = ''

      while (true) {
        const { done, value } = await reader.read()
        if (done) break

        buf += decoder.decode(value, { stream: true })
        const lines = buf.split('\n')
        buf = lines.pop() || ''

        for (const line of lines) {
          if (line.startsWith('data: ')) {
            try {
              const data = JSON.parse(line.slice(6)) as StationEvent
              onEvent(data)
            } catch {
              // skip malformed
            }
          }
        }
      }
    } catch (err: unknown) {
      if ((err as Error).name !== 'AbortError') {
        console.warn('[sse] error, retrying in 5s:', err)
        scheduleRetry(onEvent, ctrl)
      }
    }
  })()

  return ctrl
}

let retryTimer: ReturnType<typeof setTimeout> | null = null

function scheduleRetry(onEvent: (ev: StationEvent) => void, ctrl: AbortController) {
  if (retryTimer) clearTimeout(retryTimer)
  retryTimer = setTimeout(() => {
    if (!ctrl.signal.aborted) {
      connectSSE(onEvent)
    }
  }, 5000)
}