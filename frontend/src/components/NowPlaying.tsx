import type { Track } from '../types'

interface NowPlayingProps {
  track: Track | null | undefined
}

export function NowPlaying({ track }: NowPlayingProps) {
  if (!track) {
    return (
      <section className="panel">
        <h2>🎵 Now Playing</h2>
        <p className="muted">Nothing playing — queue a track!</p>
      </section>
    )
  }

  return (
    <section className="panel now-playing">
      <h2>🎵 Now Playing</h2>
      <div className="track-card highlight">
        <div className="track-title">{track.title}</div>
        <div className="track-artist">{track.artist}</div>
        <div className="track-meta">
          <span className="badge">{track.source}</span>
          {track.on_chain && <span className="badge badge-sol">✦ On-Chain</span>}
          <span className="badge">🎶 {track.play_count} plays</span>
          <span className="badge">💧 {track.tip_total} tipped</span>
          {track.sol_sig && (
            <a
              className="badge badge-tx"
              href={`https://solscan.io/tx/${track.sol_sig}${track.sol_sig.length > 32 ? '?cluster=devnet' : ''}`}
              target="_blank"
              rel="noopener noreferrer"
            >
              🔗 Tx
            </a>
          )}
        </div>
      </div>
    </section>
  )
}