/**
 * Scores how well query matches text: prefix > word start > substring >
 * in-order subsequence. Returns 0 for no match. Case-insensitive.
 */
export function score(query: string, text: string): number {
  const q = query.toLowerCase()
  const t = text.toLowerCase()
  if (!q) return 1
  if (t.startsWith(q)) return 100 - Math.min(t.length - q.length, 50) / 10
  const at = t.indexOf(q)
  if (at > 0) return /[\s\-_.@:/]/.test(t[at - 1]) ? 80 : 60

  let ti = 0
  let gaps = 0
  for (const ch of q) {
    const found = t.indexOf(ch, ti)
    if (found < 0) return 0
    gaps += found - ti
    ti = found + 1
  }
  return Math.max(1, 40 - gaps)
}

/** Best score of query against any of the fields. */
export function best(query: string, fields: string[]): number {
  let top = 0
  for (const f of fields) if (f) top = Math.max(top, score(query, f))
  return top
}
