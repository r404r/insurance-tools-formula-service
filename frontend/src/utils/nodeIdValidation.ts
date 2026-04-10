/**
 * Node ID format validation.
 *
 * Rules:
 * - Non-empty
 * - First character: letter or underscore
 * - Subsequent characters: letters, digits, underscores
 * - Max length 64
 *
 * Historical IDs (e.g. UUID-like ones with hyphens) are NOT force-migrated;
 * this regex is only applied when a user actively edits a node ID.
 */
export const NODE_ID_REGEX = /^[A-Za-z_][A-Za-z0-9_]{0,63}$/

export type NodeIdFormatError = 'empty' | 'invalid'

export function validateNodeIdFormat(id: string): NodeIdFormatError | null {
  if (id.length === 0) return 'empty'
  if (!NODE_ID_REGEX.test(id)) return 'invalid'
  return null
}
