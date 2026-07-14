import Link from "next/link"
import type { ReactNode } from "react"
import { AppIcon } from "@/components/icon"
import { DocsMediaPlaceholder } from "./docs-media-placeholder"

const WORKSPACE_ROLES = [
  {
    role: "Owner",
    summary: "Has the highest access level.",
    details:
      "Owners can do everything an admin can. Only they can grant or remove the Owner role, transfer ownership, or permanently delete the workspace.",
  },
  {
    role: "Admin",
    summary: "Runs membership and shared setup.",
    details:
      "Admins invite and remove members, assign Admin or Member, manage teams, and configure shared plugins, connections, and knowledge.",
  },
  {
    role: "Member",
    summary: "Works inside their teams.",
    details:
      "Members use their teams' agents and channels. They can create and manage channels there, but they don't get workspace-wide admin controls.",
  },
]

export function AccessControl() {
  return (
    <div className="mt-10 text-base leading-7">
      <p className="max-w-2xl text-muted">
        Give people the access their jobs require, then stop. Workspace roles
        control administration; team and private-channel membership decide where
        someone can work.
      </p>

      <section aria-labelledby="two-layers" className="mt-14">
        <h2
          id="two-layers"
          className="text-xl font-semibold tracking-tight text-foreground"
        >
          Access has two layers
        </h2>
        <div className="mt-6 overflow-hidden rounded-xl border border-border bg-surface">
          <AccessLayer
            number="01"
            title="Workspace role"
            description="Owner, Admin, or Member sets the actions someone can take across the workspace."
          />
          <AccessLayer
            number="02"
            title="Team membership"
            description="Team membership opens that team's agents and public channels. A private channel has its own member list."
          />
        </div>
        <p className="mt-4 max-w-2xl text-sm leading-6 text-muted">
          An admin can fix access across the workspace; a member sees only the
          teams they&apos;ve joined. See how that boundary works in{" "}
          <DocLink href="/docs/workspace-and-access/teams">Teams</DocLink>.
        </p>
      </section>

      <section aria-labelledby="workspace-roles" className="mt-16">
        <h2
          id="workspace-roles"
          className="text-xl font-semibold tracking-tight text-foreground"
        >
          Choose the lowest role that fits
        </h2>
        <p className="mt-3 max-w-2xl text-muted">
          Start most people as Members. Promote someone to Admin or Owner only
          when their job includes managing the workspace itself.
        </p>

        <div className="mt-7 divide-y divide-border border-y border-border">
          {WORKSPACE_ROLES.map((item) => (
            <div
              key={item.role}
              className="grid gap-2 py-6 sm:grid-cols-[9rem_1fr] sm:gap-8"
            >
              <h3 className="font-semibold text-foreground">{item.role}</h3>
              <div>
                <p className="font-medium text-foreground">{item.summary}</p>
                <p className="mt-1 text-sm leading-6 text-muted">
                  {item.details}
                </p>
              </div>
            </div>
          ))}
        </div>
      </section>

      <DocsMediaPlaceholder
        type="image"
        title="Members, roles, and pending invitations"
        description="Take this screenshot from Settings > Teams at 4K and 100% browser zoom. Put Members and Pending invitations in the same frame, with demo entries for each role and one pending invite; hide every real email address."
        className="mt-12"
      />

      <div className="mt-16 space-y-14 border-t border-border pt-14">
        <DocSection title="Invite people into the right scope">
          <p>
            Go to <strong>Settings</strong> &gt; <strong>Teams</strong>, then
            choose <strong>Invite member</strong>. Enter an email address, pick{" "}
            <strong>Admin</strong> or <strong>Member</strong>, and select the
            teams that person needs.
          </p>
          <p className="mt-3">
            Once the recipient accepts, Hivy adds them to those teams. They must
            sign in with the address that received the invitation.
          </p>
          <DocLink href="/w/settings/teams">Open team settings</DocLink>
        </DocSection>

        <DocSection title="Manage pending invitations">
          <p>
            Owners and admins see outstanding invites under{" "}
            <strong>Pending invitations</strong>. Each link expires after seven
            days; resend it to create a new link and expiry, or revoke it to
            stop the person from joining.
          </p>
        </DocSection>

        <DocsMediaPlaceholder
          type="video"
          title="Invite a member with the right access"
          description="Record a 45 to 60 second walkthrough at 4K and 100% browser zoom. Invite a demo user as a Member on two teams, show the pending invite, then use Resend and Revoke without exposing a real email address."
          bleed={false}
        />

        <DocSection title="Change access as responsibilities change">
          <p>
            Owners and admins can change another person&apos;s role or remove
            them from the workspace. Edit team membership separately when the
            person moves between groups; their workspace role can stay the same.
          </p>
          <p className="mt-3">
            Hivy blocks self-service role changes. Only an Owner can grant or
            revoke ownership, and nobody can demote or remove the last Owner.
          </p>
        </DocSection>

        <DocSection title="Transfer ownership deliberately">
          <p>
            When an Owner transfers the workspace, Hivy promotes the chosen
            member to Owner and changes the previous Owner to Admin in the same
            operation. An Owner always remains.
          </p>
        </DocSection>

        <section
          aria-labelledby="access-checklist"
          className="rounded-xl border border-border bg-surface-secondary p-6"
        >
          <h2
            id="access-checklist"
            className="text-lg font-semibold tracking-tight text-foreground"
          >
            A safe starting point
          </h2>
          <ul className="mt-4 space-y-3 text-sm text-muted">
            {[
              "Start most people as Members.",
              "A person only needs the teams they work in.",
              "Reserve Admin for people who manage members, teams, and shared workspace setup.",
              "For continuity, keep a second trusted Owner.",
            ].map((item) => (
              <li key={item} className="flex gap-3">
                <AppIcon
                  icon="check"
                  className="mt-1 h-4 w-4 shrink-0 text-accent"
                />
                <span>{item}</span>
              </li>
            ))}
          </ul>
        </section>
      </div>
    </div>
  )
}

function AccessLayer({
  number,
  title,
  description,
}: {
  number: string
  title: string
  description: string
}) {
  return (
    <div className="grid gap-3 border-b border-border p-5 last:border-b-0 sm:grid-cols-[3rem_10rem_1fr] sm:items-baseline sm:gap-5">
      <span className="text-xs font-semibold tracking-[0.12em] text-muted uppercase">
        {number}
      </span>
      <h3 className="font-semibold text-foreground">{title}</h3>
      <p className="text-sm leading-6 text-muted">{description}</p>
    </div>
  )
}

function DocSection({
  title,
  children,
}: {
  title: string
  children: ReactNode
}) {
  const id = title.toLowerCase().replaceAll(" ", "-")

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

function DocLink({ href, children }: { href: string; children: ReactNode }) {
  return (
    <Link
      href={href}
      className="rounded-sm font-medium text-foreground underline decoration-border underline-offset-4 transition-colors hover:text-accent focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus"
    >
      {children}
    </Link>
  )
}
