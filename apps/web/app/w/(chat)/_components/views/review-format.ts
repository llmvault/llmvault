export function formatPatchCount(count: number) {
  return `${new Intl.NumberFormat().format(count)} ${
    count === 1 ? "file" : "files"
  }`
}
