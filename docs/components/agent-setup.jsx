'use client'

import { useState } from 'react'

export function AgentSetup({ url }) {
  const [status, setStatus] = useState('')
  const prompt = `Read ${url} and help me set up indexit, connect my Telegram account, and explore a conversation I choose. Guide me through any steps I need to do myself.`

  async function copy() {
    try {
      await navigator.clipboard.writeText(prompt)
      setStatus('Copied. Paste it into your agent.')
    } catch {
      setStatus('Select and copy the instruction above.')
    }
  }

  return (
    <div className="agent-setup">
      <p className="eyebrow">GIVE THIS TO YOUR AGENT</p>
      <p className="agent-prompt">{prompt}</p>
      <div className="agent-actions">
        <button type="button" className="button-primary" onClick={copy}>Copy instruction</button>
        <a href={url}>Read the agent instructions ↗</a>
      </div>
      <p className="agent-copy-status" role="status">{status}</p>
    </div>
  )
}
