import { ErrorNotice, Loading } from "../components/Feedback"
import { useMe } from "../hooks/queries"

// "/" — greets the signed-in user. The canonical read pattern: call the
// hook, render Loading for isPending, ErrorNotice for isError, data last.
export default function Welcome() {
  const me = useMe()

  if (me.isPending) return <Loading />
  if (me.isError) return <ErrorNotice error={me.error} />

  return (
    <header>
      <h1>Welcome to Hivy, {me.data.user_name}</h1>
      <p className="muted">
        {me.data.user_email} · {me.data.org_name} · {me.data.role}
      </p>
    </header>
  )
}
