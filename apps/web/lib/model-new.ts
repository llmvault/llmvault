type ModelNewWindow = {
  new_from?: string | null
  new_to?: string | null
}

export function isModelNew(
  model: ModelNewWindow | null | undefined,
  now = new Date()
): boolean {
  if (!model?.new_from || !model.new_to) return false

  const from = Date.parse(model.new_from)
  const to = Date.parse(model.new_to)
  const current = now.getTime()
  return (
    Number.isFinite(from) &&
    Number.isFinite(to) &&
    from < to &&
    current >= from &&
    current < to
  )
}
