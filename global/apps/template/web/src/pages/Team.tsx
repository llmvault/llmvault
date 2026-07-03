import { ErrorNotice, Loading } from "../components/Feedback"
import { usePages } from "../hooks/queries"

// "/team" — lists the bound sheet's pages with row/field counts.
export default function Team() {
  const pages = usePages()

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
    </section>
  )
}
