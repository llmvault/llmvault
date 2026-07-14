import type { ReactNode } from "react"
import { DocsMediaPlaceholder } from "./docs-media-placeholder"

export function HttpWebhooks() {
  return (
    <div className="mt-10 text-base leading-7">
      <p className="max-w-2xl text-muted">
        Any system that can send a POST request can start a Hivy agent through
        an HTTP webhook. The body carries request-specific data; the
        webhook&apos;s saved instructions tell the agent what to produce in its
        channel.
      </p>

      <DocsMediaPlaceholder
        type="video"
        title="Call a protected Hivy webhook"
        description="Create a webhook trigger with a shared secret on camera; copy its URL, send a small JSON POST request with an Authorization bearer header, and open the session that request starts."
        className="mt-12"
      />

      <section aria-labelledby="create-the-webhook" className="mt-16">
        <h2
          id="create-the-webhook"
          className="text-xl font-semibold tracking-tight text-foreground"
        >
          Create the webhook
        </h2>
        <ol className="mt-8 divide-y divide-border overflow-hidden rounded-xl border border-border bg-surface">
          <Step number="1" title="Choose its home">
            Pick the channel for new sessions and an agent from that
            channel&apos;s team.
          </Step>
          <Step number="2" title="Write stable instructions">
            Explain what every request should make the agent do; each body
            supplies the data that changes.
          </Step>
          <Step number="3" title="Create and store a shared secret">
            Make the value long and unique, then save it in the caller&apos;s
            secret manager. Hivy keeps a protected hash and can&apos;t recover
            the original.
          </Step>
          <Step number="4" title="Copy the unique URL">
            Open the new webhook&apos;s detail page and copy its POST URL.
          </Step>
        </ol>
      </section>

      <DocsMediaPlaceholder
        type="image"
        title="Webhook URL, status, and last run"
        description="Keep the webhook detail page readable, including the URL and Copy action, latest run link, channel, agent, instructions, and enabled status. No real shared secret should appear."
        className="mt-12"
      />

      <div className="mt-16 space-y-14 border-t border-border pt-14">
        <DocSection title="Send a small POST request">
          <p>
            Put the shared secret in the Authorization header as a bearer token.
            An agent can usually read a JSON object more reliably than an
            unstructured body, although Hivy accepts either; the body must stay
            below 256 KB.
          </p>
          <RequestExample />
          <p className="mt-4">
            Hivy also reads the secret from X-Api-Key or X-Webhook-Secret.
            Don&apos;t use the supported secret query parameter unless your
            caller can&apos;t set headers, because URL logs may capture it.
          </p>
        </DocSection>

        <DocSection title="Treat the response as acceptance">
          <p>
            A 200 response means Hivy accepted the request for asynchronous
            processing, not that the agent finished. Open the webhook&apos;s
            latest session for the result; the HTTP response won&apos;t contain
            it.
          </p>
        </DocSection>

        <DocSection title="Keep sensitive data out of the task">
          <p>
            Hivy redacts JSON fields whose keys contain password, secret, token,
            API key, credential, or authorization. Redaction can&apos;t make a
            loose data policy safe, so send a reference or the smallest required
            value instead of a credential.
          </p>
          <p className="mt-3">
            The URL identifies the webhook, but every call still needs the
            shared secret. To rotate that secret, create a replacement webhook
            and move the caller before you delete the old one.
          </p>
        </DocSection>

        <DocSection title="Disable requests without deleting the setup">
          <p>
            Turn the webhook off when calls need to stop for a while; Hivy
            rejects requests to it and doesn&apos;t start the agent. Workspace
            owners and admins can edit, disable, or delete an existing webhook.
          </p>
        </DocSection>
      </div>
    </div>
  )
}

function Step({
  number,
  title,
  children,
}: {
  number: string
  title: string
  children: ReactNode
}) {
  return (
    <li className="grid gap-4 px-5 py-5 sm:grid-cols-[2rem_1fr]">
      <span className="flex h-7 w-7 items-center justify-center rounded-full bg-default text-xs font-semibold text-foreground">
        {number}
      </span>
      <div>
        <h3 className="font-semibold text-foreground">{title}</h3>
        <p className="mt-1 text-sm leading-6 text-muted">{children}</p>
      </div>
    </li>
  )
}

function RequestExample() {
  return (
    <pre className="mt-6 overflow-x-auto rounded-xl border border-border bg-surface p-5 text-sm leading-6 text-foreground">
      <code>{`curl -X POST "YOUR_WEBHOOK_URL" \\
  -H "Authorization: Bearer YOUR_SHARED_SECRET" \\
  -H "Content-Type: application/json" \\
  -d '{"customer_id":"customer-123","event":"trial_ended"}'`}</code>
    </pre>
  )
}

function DocSection({
  title,
  children,
}: {
  title: string
  children: ReactNode
}) {
  const id = "http-webhooks-" + title.toLowerCase().replaceAll(" ", "-")
  return (
    <section aria-labelledby={id}>
      <h2
        id={id}
        className="text-xl font-semibold tracking-tight text-foreground"
      >
        {title}
      </h2>
      <div className="mt-3 max-w-2xl text-muted">{children}</div>
    </section>
  )
}
