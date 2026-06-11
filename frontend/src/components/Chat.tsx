import { useState, useEffect, useRef } from 'react'

interface ChatMessage {
  name: string
  text: string
  time: string
}

interface ChatProps {
  onSend: (name: string, text: string) => void
  /** Incoming messages from SSE events (type === "chat_message"). */
  incoming: ChatMessage[]
}

export function Chat({ onSend, incoming }: ChatProps) {
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [name, setName] = useState('Listener')
  const [text, setText] = useState('')
  const listRef = useRef<HTMLUListElement>(null)

  // Prepend incoming SSE chat messages.
  useEffect(() => {
    if (incoming.length > 0) {
      setMessages((prev) => [...prev, ...incoming].slice(-100))
    }
  }, [incoming])

  // Auto-scroll on new messages.
  useEffect(() => {
    if (listRef.current) {
      listRef.current.scrollTop = listRef.current.scrollHeight
    }
  }, [messages])

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (!text.trim()) return
    const msg: ChatMessage = {
      name: name.trim() || 'Anonymous',
      text: text.trim(),
      time: new Date().toISOString(),
    }
    setMessages((prev) => [...prev, msg])
    onSend(msg.name, msg.text)
    setText('')
  }

  return (
    <section className="panel">
      <h2>💬 Chat</h2>

      <form className="chat-form" onSubmit={handleSubmit}>
        <input
          type="text"
          placeholder="Your name"
          value={name}
          onChange={(e) => setName(e.target.value)}
          className="chat-name"
        />
        <div className="chat-input-row">
          <input
            type="text"
            placeholder="Type a message..."
            value={text}
            onChange={(e) => setText(e.target.value)}
            required
          />
          <button type="submit">Send</button>
        </div>
      </form>

      <ul className="chat-messages" ref={listRef}>
        {messages.length === 0 && (
          <li className="muted">No messages yet. Say something!</li>
        )}
        {messages.map((m, i) => (
          <li key={i}>
            <strong>{m.name}:</strong> {m.text}
          </li>
        ))}
      </ul>
    </section>
  )
}