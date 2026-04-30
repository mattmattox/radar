/**
 * Computes a 1- or 2-character avatar label for a username.
 *
 * Rules (in order):
 *   1. Strip the @-domain from the local-part — domains never carry
 *      useful identity for an in-app avatar.
 *   2. Drop everything that isn't a letter (separators like `.`,
 *      `_`, `-`, digits, punctuation). The avatar circle can only
 *      render a meaningful glyph for letters; rendering `.U` or
 *      `12` looks broken.
 *   3. If the cleaned local-part contains separator-bounded
 *      segments (`.`, `_`, `-`), use the first letter of each
 *      segment (max 2). e.g. `"mary.kohli"` → `"MK"`.
 *   4. Otherwise use the first 1-2 letters of the cleaned
 *      local-part. e.g. `"mkohli"` → `"MK"`.
 *   5. Always uppercase.
 *   6. Returns `''` when no letters survive — the caller falls back
 *      to a silhouette / `?` icon.
 */
export function computeUserInitials(username: string | null | undefined): string {
  if (!username) return ''
  const localPart = username.split('@')[0]
  if (!localPart) return ''
  // Split on the canonical separators first so segment-based
  // initials still work, then drop any non-letter characters per
  // segment so leading punctuation can't leak into the result.
  const segments = localPart
    .split(/[._-]/)
    .map(s => s.replace(/[^a-zA-Z]/g, ''))
    .filter(Boolean)
  if (segments.length === 0) return ''
  if (segments.length >= 2) {
    return segments
      .slice(0, 2)
      .map(s => s[0].toUpperCase())
      .join('')
  }
  // Single segment (no usable separators) — surface up to two
  // leading letters so e.g. `mkohli` produces `MK` instead of `M`.
  return segments[0].slice(0, 2).toUpperCase()
}
