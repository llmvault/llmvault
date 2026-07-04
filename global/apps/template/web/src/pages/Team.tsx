import { ErrorNotice, Loading } from "../components/Feedback"
import { usePages, useRows } from "../hooks/queries"
import { cell } from "../lib/cell"

// "/team" — lists the bound sheet's pages with row/field counts, plus a
// preview of the first page's rows. That preview is the canonical pattern
// for rendering row data: field headers come from the typed SheetField[]
// (pages.data), and every cell value — typed `unknown` on Row.data — goes
// through cell() before landing in JSX. Copy this shape for any screen that
// renders rows; don't put row.data[fieldId] directly into an element.
export default function Team() {
  const pages = usePages()
  const firstPage = pages.data?.pages[0]
  // useRows is safe to call with "" (disabled) before pages has loaded — see
  // its `enabled` guard in src/hooks/queries.ts.
  const rows = useRows(firstPage?.id ?? "")

  if (pages.isPending) return <Loading label="Loading sheet…" />
  if (pages.isError) return <ErrorNotice error={pages.error} />

  const { sheet, pages: sheetPages } = pages.data

  return (
    <section>
      <h2>{sheet.name}</h2>
      {sheet.description && <p className="muted">{sheet.description}</p>}
      <ul className="pages">
        {sheetPages.map((page) => (
          <li key={page.id}>
            <span>{page.name}</span>
            <span className="muted">
              {page.row_count} rows · {page.fields.length} fields
            </span>
          </li>
        ))}
      </ul>

      {firstPage && (
        <>
          <h2>{firstPage.name} preview</h2>
          {rows.isPending && <Loading label="Loading rows…" />}
          {rows.isError && <ErrorNotice error={rows.error} />}
          {rows.data && (
            <table className="rows">
              <thead>
                <tr>
                  {firstPage.fields.map((field) => (
                    <th key={field.id}>{field.name}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {rows.data.rows.map((row) => (
                  <tr key={row.id}>
                    {firstPage.fields.map((field) => (
                      <td key={field.id}>{cell(row.data[field.id])}</td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </>
      )}
    </section>
  )
}
