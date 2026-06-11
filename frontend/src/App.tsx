import { useEffect, useState, useCallback } from 'react'
import { api } from './api/client'
import { connectSSE } from './api/sse'
import { Stats } from './components/Stats'
import { NowPlaying } from './components/NowPlaying'
import { Queue } from './components/Queue'
import { Chat } from './components/Chat'
import { Tip } from './components/Tip'
import type { StationStats, StationEvent, Track } from './types'

export default function App() {
  const [stats, setStats] = useState<StationStats | null>(null)
  const [queue, setQueue] = useState<Track[]>([])
  const [nowPlaying, setNowPlaying] = useState<Track | null | undefined>(stats?.now_playing)
  const [incomingChat, setIncomingChat] = useState<Array<{ name: string; text: string; time: string }>>([])
  const [connected, setConnected] = useState(false)

  // Fetch initial data.
  useEffect(() => {
    const load = async () => {
      try {
        const h = await api.health()
        setConnected(true)
        console.log('[app]', h)
      } catch {
        setConnected(false)
      }
      try {
        const s = await api.stats()
        setStats(s)
        setNowPlaying(s.now_playing)
      } catch { /* ok */ }
      try {
        const q = await api.queue()
        setQueue(q.queue)
      } catch { /* ok */ }
    }
    load()
    const id = setInterval(load, 15_000)
    return () => clearInterval(id)
  }, [])

  // Live SSE events.
  useEffect(() => {
    const ctrl = connectSSE((ev: StationEvent) => {
      // Update now-playing on track start.
      if (ev.type === 'track_start' && ev.payload) {
        setNowPlaying(ev.payload as Track)
      }

      // Update chat on new messages.
      if (ev.type === 'chat_message' && ev.payload) {
        const msg = ev.payload as { from?: { name?: string }; text?: string; timestamp?: string }
        setIncomingChat([
          {
            name: msg.from?.name || 'Anonymous',
            text: msg.text || '',
            time: msg.timestamp || ev.time,
          },
        ])
      }

      // Refresh queue on any queue-related event.
      if (['track_start', 'track_end', 'new_block'].includes(ev.type)) {
        api.stats().then(setStats).catch(() => {})
        api.queue().then((q) => setQueue(q.queue)).catch(() => {})
      }
    })
    return () => ctrl.abort()
  }, [])

  const handleQueue = useCallback(async (title: string, artist?: string) => {
    await api.queueTrack(title, artist)
    const q = await api.queue()
    setQueue(q.queue)
  }, [])

  const handleTip = useCallback(
    async (cid: string, lamports?: number) => {
      await api.tipTrack(cid, lamports ?? 50_000_000)
    },
    [],
  )

  const handleChat = useCallback(async (name: string, text: string) => {
    await api.sendChat(name, text)
  }, [])

  return (
    <div className="app">
      <header className="header">
        <h1>🎧 CLAWD FM</h1>
        <p className="subtitle">Solana Onchain Terminal Radio</p>
        <span className={`status-dot ${connected ? 'online' : 'offline'}`}>
          {connected ? '● Connected' : '○ Disconnected'}
        </span>
      </header>

      <main className="dashboard">
        <div className="col col-left">
          <Stats stats={stats} />
          <NowPlaying track={nowPlaying} />
          <Tip onTip={(cid, lamports) => handleTip(cid, lamports)} />
        </div>
        <div className="col col-right">
          <Queue tracks={queue} onQueue={handleQueue} onTip={(cid) => handleTip(cid)} />
          <Chat onSend={handleChat} incoming={incomingChat} />
        </div>
      </main>

      <footer className="footer">
        <p>
          <a href="https://github.com/Solizardking/clawd-fm" target="_blank" rel="noopener noreferrer">
            GitHub
          </a>
          {' · '}
          <a href="https://clawd-fm.fly.dev/health" target="_blank" rel="noopener noreferrer">
            API Health
          </a>
          {' · '}
          <a href="https://clawd-fm.fly.dev/events" target="_blank" rel="noopener noreferrer">
            SSE Events
          </a>
        </p>
      </footer>
    </div>
  )
}