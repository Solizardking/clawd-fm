import { useState } from 'react'

interface TipProps {
  onTip: (cid: string, lamports: number) => void
}

export function Tip({ onTip }: TipProps) {
  const [cid, setCid] = useState('')
  const [lamports, setLamports] = useState('50000000')
  const [status, setStatus] = useState('')

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!cid.trim() || !lamports.trim()) return
    setStatus('Sending...')
    try {
      await onTip(cid.trim(), parseInt(lamports, 10))
      setStatus('✅ Tip sent!')
      setCid('')
    } catch (err: unknown) {
      setStatus(`❌ ${(err as Error).message}`)
    }
  }

  return (
    <section className="panel">
      <h2>💧 Tip a Track</h2>
      <form className="tip-form" onSubmit={handleSubmit}>
        <input
          type="text"
          placeholder="Track CID"
          value={cid}
          onChange={(e) => setCid(e.target.value)}
          required
        />
        <input
          type="number"
          placeholder="Lamports (1 SOL = 1,000,000,000)"
          value={lamports}
          onChange={(e) => setLamports(e.target.value)}
          min={1}
          required
        />
        <button type="submit">Send Tip</button>
      </form>
      {status && <p className="tip-status">{status}</p>}
    </section>
  )
}