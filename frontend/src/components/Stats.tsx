import type { StationStats } from '../types'

interface StatsProps {
  stats: StationStats | null
}

export function Stats({ stats }: StatsProps) {
  if (!stats) {
    return (
      <section className="panel">
        <h2>📡 Station Status</h2>
        <p className="muted">Waiting for station data...</p>
      </section>
    )
  }

  return (
    <section className="panel">
      <h2>📡 {stats.name}</h2>
      <div className="stat-grid">
        <Stat label="Block Height" value={String(stats.block_height)} />
        <Stat label="Peers" value={String(stats.peers)} />
        <Stat label="Listeners" value={String(stats.listeners)} />
        <Stat label="Agents" value={String(stats.agents)} />
        <Stat label="Queue" value={String(stats.queue_len)} />
        <Stat label="Streaming" value={stats.streaming ? '🔊 Live' : '⏸️ Paused'} />
        <Stat label="SOL Wallet" value={stats.has_solana ? stats.sol_wallet || '—' : 'Local Only'} />
        <Stat label="Balance" value={`${stats.balance} tokens`} />
      </div>
    </section>
  )
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="stat">
      <span className="stat-label">{label}</span>
      <span className="stat-value">{value}</span>
    </div>
  )
}