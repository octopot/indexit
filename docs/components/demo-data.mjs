// Public, fictional records used only by the documentation illustrations.
export const messages = [
  { id: 1042, date: '2026-08-15T12:30:00Z', text: 'Weekend trip: the lake trail is open. Bring a camera!', from: { display: 'Alex', username: 'example_alex' }, media: { type: 'photo' } },
  { id: 1041, date: '2026-08-14T18:10:00Z', text: 'Train tickets booked. See you on Saturday.', from: { display: 'Sam', username: 'example_sam' } },
  { id: 1040, date: '2026-08-13T09:00:00Z', text: 'A packing list for the next trip: boots, snacks, a map.', from: { display: 'Alex', username: 'example_alex' } },
]
export const dialogs = [
  { title: 'Weekend trips', uid: 'channel:101', username: 'example_channel' },
  { title: 'Trail notes', uid: 'channel:102', username: 'example_trails' },
  { title: 'Design bookmarks', uid: 'channel:103', username: 'example_design' },
  { title: 'Reading club', uid: 'channel:104', username: 'example_reading' },
  { title: 'Release notes', uid: 'channel:105', username: 'example_releases' },
  { title: 'Project ideas', uid: 'channel:106', username: 'example_projects' },
]

// Illustrates subsequence matching, not fzf's full ranking or query language.
export function fuzzyMatch(value, query) {
  const text = value.toLocaleLowerCase()
  return query.toLocaleLowerCase().trim().split(/\s+/).every(term => {
    let offset = 0
    for (const char of term) {
      const found = text.indexOf(char, offset)
      if (found === -1) return false
      offset = found + char.length
    }
    return true
  })
}
export function leaves(value, path = '') {
  if (value !== null && typeof value === 'object') {
    return Object.entries(value).flatMap(([key, child]) => leaves(child,
      Array.isArray(value) ? `${path}[${key}]` : `${path}.${key}`))
  }
  return [{ path, value }]
}
