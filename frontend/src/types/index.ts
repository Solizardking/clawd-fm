/** Track in the CLAWD FM queue. */
export interface Track {
  cid: string
  title: string
  artist: string
  duration: number // nanoseconds
  source: string
  solana_wallet?: string
  on_chain: boolean
  sol_sig?: string
  tip_total: number
  play_count: number
  added_at: string // ISO time
}

/** Live station stats snapshot. */
export interface StationStats {
  name: string
  listeners: number
  queue_len: number
  block_height: number
  balance: number
  agents: number
  peers: number
  streaming: boolean
  now_playing?: Track
  sol_wallet?: string
  has_solana: boolean
}

/** Station event from SSE stream. */
export interface StationEvent {
  type: string
  payload: unknown
  time: string
}