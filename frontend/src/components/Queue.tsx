import { useState } from 'react'
import type { Track } from '../types'

interface QueueProps {
  tracks: Track[]
  onQueue: (title: string, artist?: string) => void
  onTip: (cid: string) => void
}

export function Queue({ tracks, onQueue, onTip }: QueueProps) {
  const [title, setTitle] = useState('')
  const [artist, setArtist] = useState('')
  const [submitting, setSubmitting] = useState(false)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!title.trim()) return
    setSubmitting(true)
    try {
      await onQueue(title.trim(), artist.trim() || undefined)
      setTitle('')
      setArtist('')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <section className="panel">
      <h2>📋 Queue ({tracks.length})</h2>

      <form className="queue-form" onSubmit={handleSubmit}>
        <input
          type="text"
          placeholder="Track title"
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          required
          disabled={submitting}
        />
        <input
          type="text"
          placeholder="Artist (optional)"
          value={artist}
          onChange={(e) => setArtist(e.target.value)}
          disabled={submitting}
        />
        <button type="submit" disabled={submitting || !title.trim()}>
          {submitting ? 'Queueing...' : 'Queue'}
        </button>
      </form>

      {tracks.length === 0 ? (
        <p className="muted">No tracks in queue.</p>
      ) : (
        <ul className="track-list">
          {tracks.map((t, i) => (
            <li key={t.cid + i} className="track-card">
              <div>
                <span className="track-title">{t.title}</span>
                <span className="track-artist"> — {t.artist}</span>
              </div>
              <div className="track-actions">
                <span className="badge">{t.source}</span>
                {t.on_chain && <span className="badge badge-sol">✦ On-Chain</span>}
                <button className="btn-tip" onClick={() => onTip(t.cid)}>
                  💧 Tip
                </button>
              </div>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}