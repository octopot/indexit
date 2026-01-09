'use client'

import { useId, useRef, useState } from 'react'
import { dialogs, fuzzyMatch, leaves, messages } from './demo-data.mjs'

const nodes = leaves(messages)

function Frame({ name, command, children, footer }) {
  return <div className="workflow-terminal">
    <div className="workflow-title"><span><span aria-hidden="true">● ● ●</span> {name}</span><span>TRY IT HERE</span></div>
    <pre className="workflow-command"><span aria-hidden="true">$ </span>{command}</pre>
    {children}
    <div className="workflow-footer">{footer}</div>
    <div className="workflow-disclaimer">Simplified browser illustration · fictional data · no Telegram connection</div>
  </div>
}

function TreeNode({ label, value, path, onSelect, depth = 0 }) {
  if (value !== null && typeof value === 'object') {
    return <details className="json-branch" open={depth < 1}>
      <summary><span className="json-key">{label}</span> <span className="json-hint">{Array.isArray(value) ? `[${value.length}]` : `{${Object.keys(value).length}}`}</span></summary>
      <div className="json-children">{Object.entries(value).map(([key, child]) =>
        <TreeNode key={key} label={key} value={child} path={`${path}.${key}`} depth={depth + 1} onSelect={onSelect} />
      )}</div>
    </details>
  }
  return <button type="button" className="json-leaf" onClick={() => onSelect({ path, value })}>
    <span className="json-key">{label}:</span> <span className={typeof value === 'number' ? 'json-number' : 'json-value'}>{JSON.stringify(value)}</span>
  </button>
}

export function FxDemo() {
  const id = useId()
  const input = useRef(null)
  const [mode, setMode] = useState('text')
  const [query, setQuery] = useState('')
  const [selected, setSelected] = useState(null)
  const matches = nodes.filter(node => mode === 'path'
    ? fuzzyMatch(node.path, query)
    : String(node.value).toLocaleLowerCase().includes(query.toLocaleLowerCase()))

  function search(nextMode, value = '') {
    setMode(nextMode); setQuery(value); setSelected(null); input.current?.focus()
  }

  return <Frame name="fx / explore messages" command={'indexit -q telegram fetch messages \\\n  --dialog @example_channel --limit 200 | fx'} footer={<><kbd>/</kbd> values <kbd>@</kbd> paths <span>Click a field to inspect it</span></>}>
    <div className="workflow-body" onKeyDown={event => {
      if (event.target.tagName === 'INPUT') return
      if (event.key === '/' || event.key === '@') { event.preventDefault(); search(event.key === '@' ? 'path' : 'text') }
    }}>
      <div className="demo-presets"><span>Try a search</span><button type="button" onClick={() => search('text', 'trip')}>trip</button><button type="button" onClick={() => search('path', 'from.display')}>@ from.display</button><button type="button" onClick={() => search('text')}>Reset</button></div>
      <div className="demo-search">
        <div className="demo-search-mode" aria-label="Search mode"><button type="button" aria-pressed={mode === 'text'} onClick={() => search('text')}>/</button><button type="button" aria-pressed={mode === 'path'} onClick={() => search('path')}>@</button></div>
        <label className="demo-sr-only" htmlFor={id}>{mode === 'path' ? 'Fuzzy search JSON paths' : 'Search sample message values'}</label>
        <input id={id} ref={input} value={query} onChange={event => { setQuery(event.target.value); setSelected(null) }} placeholder={mode === 'path' ? 'Find a path…' : 'Find a word in the messages…'} autoComplete="off" spellCheck="false" />
      </div>
      <div className="demo-scroll json-tree" aria-label="Sample JSON explorer">
        {query ? <>
          <div className="demo-count" role="status">{matches.length} matching fields</div>
          {matches.map(node => <button type="button" className="json-result" key={node.path} onClick={() => setSelected(node)}><span className="json-path">{node.path}</span><span className="json-value">{JSON.stringify(node.value)}</span></button>)}
          {!matches.length && <p className="demo-empty">No matching nodes. Try “trip” or switch to path search.</p>}
        </> : messages.map((message, index) => <TreeNode key={message.id} label={`[${index}]`} path={`[${index}]`} value={message} onSelect={setSelected} />)}
      </div>
      <div className="demo-result" aria-live="polite">{selected ? <><span className="json-path">{selected.path}</span><span>{JSON.stringify(selected.value)}</span></> : <span>Explore a record, or search across its fields.</span>}</div>
    </div>
  </Frame>
}

export function FzfDemo() {
  const id = useId()
  const [query, setQuery] = useState('')
  const [active, setActive] = useState(0)
  const [selected, setSelected] = useState(null)
  const matches = dialogs.filter(dialog => fuzzyMatch(dialog.title, query))
  const cursor = Math.min(active, Math.max(0, matches.length - 1))
  function change(value) { setQuery(value); setActive(0); setSelected(null) }
  function choose(dialog) { if (dialog) setSelected(dialog) }
  return <Frame name="fzf / find a conversation" command={'indexit -q telegram fetch dialogs \\\n  | jq --unbuffered -r \'[.uid, .title] | @tsv\' \\\n  | fzf --delimiter \'\\t\' --with-nth 2..'} footer={<><kbd>↑</kbd><kbd>↓</kbd> navigate <kbd>enter</kbd> choose <span>Search while you type</span></>}>
    <div className="workflow-body">
      <div className="demo-presets"><span>Try a search</span><button type="button" onClick={() => change('wknd')}>wknd</button><button type="button" onClick={() => change('rdng')}>rdng</button><button type="button" onClick={() => change('')}>Reset</button></div>
      <div className="demo-search"><span className="demo-prompt" aria-hidden="true">❯</span><label className="demo-sr-only" htmlFor={id}>Find a sample conversation</label><input id={id} role="combobox" aria-autocomplete="list" aria-expanded="true" aria-controls={`${id}-list`} aria-activedescendant={matches.length ? `${id}-option-${cursor}` : undefined} value={query} onChange={event => change(event.target.value)} placeholder="Find a conversation…" autoComplete="off" spellCheck="false" onKeyDown={event => {
        if (event.key === 'ArrowDown' || event.key === 'ArrowUp') { event.preventDefault(); setActive(Math.max(0, Math.min(matches.length - 1, cursor + (event.key === 'ArrowDown' ? 1 : -1)))) }
        if (event.key === 'Enter') { event.preventDefault(); choose(matches[cursor]) }
        if (event.key === 'Escape') change('')
      }} /></div>
      <div className="demo-count" role="status">{matches.length} / {dialogs.length} conversations</div>
      <div className="demo-scroll finder-list" id={`${id}-list`} role="listbox" aria-label="Matching conversations">
        {matches.map((dialog, index) => <button type="button" role="option" aria-selected={cursor === index} tabIndex={-1} id={`${id}-option-${index}`} key={dialog.uid} className={`finder-option ${cursor === index ? 'is-active' : ''}`} onClick={() => { setActive(index); choose(dialog) }}><span aria-hidden="true">{cursor === index ? '›' : ' '}</span><span>{dialog.title}</span></button>)}
        {!matches.length && <p className="demo-empty">No conversations match. Try a shorter query.</p>}
      </div>
      <div className="demo-result" aria-live="polite">{selected ? <><span className="json-path">Selected: {selected.title}</span><span>The terminal returns this title together with its UID.</span></> : <span>Type a few letters, then choose a conversation.</span>}</div>
    </div>
  </Frame>
}

export function WorkflowDemo() {
  const [mode, setMode] = useState('fx')
  return <div className="workflow-demo">
    <div className="workflow-switch" role="group" aria-label="Choose an interactive example"><button type="button" aria-pressed={mode === 'fx'} onClick={() => setMode('fx')}>01 <b>Explore with fx</b></button><button type="button" aria-pressed={mode === 'fzf'} onClick={() => setMode('fzf')}>02 <b>Find with fzf</b></button></div>
    {mode === 'fx' ? <FxDemo /> : <FzfDemo />}
  </div>
}
